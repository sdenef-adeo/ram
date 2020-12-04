// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the 'License');
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an 'AS IS' BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package listgroups

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/BrunoReboul/ram/utilities/aut"
	"github.com/BrunoReboul/ram/utilities/cai"
	"github.com/BrunoReboul/ram/utilities/ffo"
	"github.com/BrunoReboul/ram/utilities/gfs"
	"github.com/BrunoReboul/ram/utilities/gps"
	"github.com/BrunoReboul/ram/utilities/solution"
	"google.golang.org/api/option"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/functions/metadata"
	"cloud.google.com/go/pubsub"
	admin "google.golang.org/api/admin/directory/v1"
)

// Global variable to deal with GroupsListCall Pages constraint: no possible to pass variable to the function in pages()
// https://pkg.go.dev/google.golang.org/api/admin/directory/v1?tab=doc#GroupsListCall.Pages
var ctx context.Context
var directoryCustomerID string
var domain string
var emailPrefix string
var logEventEveryXPubSubMsg uint64
var pubSubClient *pubsub.Client
var outputTopicName string
var pubSubErrNumber uint64
var pubSubID string
var pubSubMsgNumber uint64
var timestamp time.Time

// Global structure for global variables to optimize the cloud function performances
type Global struct {
	ctx                     context.Context
	dirAdminService         *admin.Service
	directoryCustomerID     string
	firestoreClient         *firestore.Client
	inputTopicName          string
	logEventEveryXPubSubMsg uint64
	maxResultsPerPage       int64 // API Max = 200
	outputTopicName         string
	pubSubClient            *pubsub.Client
	PubSubID                string
	retryTimeOutSeconds     int64
}

// Settings from PubSub triggering event
type Settings struct {
	Domain      string `json:"domain"`
	EmailPrefix string `json:"emailPrefix"`
}

// Initialize is to be executed in the init() function of the cloud function to optimize the cold start
func Initialize(ctx context.Context, global *Global) (err error) {
	global.ctx = ctx

	var instanceDeployment InstanceDeployment
	var clientOption option.ClientOption
	var ok bool

	log.Println("Function COLD START")
	err = ffo.ReadUnmarshalYAML(solution.PathToFunctionCode+solution.SettingsFileName, &instanceDeployment)
	if err != nil {
		return fmt.Errorf("ReadUnmarshalYAML %s %v", solution.SettingsFileName, err)
	}

	gciAdminUserToImpersonate := instanceDeployment.Settings.Instance.GCI.SuperAdminEmail
	global.directoryCustomerID = instanceDeployment.Settings.Instance.GCI.DirectoryCustomerID
	global.inputTopicName = instanceDeployment.Artifacts.TopicName
	global.logEventEveryXPubSubMsg = instanceDeployment.Settings.Service.LogEventEveryXPubSubMsg
	global.maxResultsPerPage = instanceDeployment.Settings.Service.MaxResultsPerPage
	global.outputTopicName = instanceDeployment.Artifacts.OutputTopicName
	global.retryTimeOutSeconds = instanceDeployment.Settings.Service.GCF.RetryTimeOutSeconds
	projectID := instanceDeployment.Core.SolutionSettings.Hosting.ProjectID
	keyJSONFilePath := solution.PathToFunctionCode + instanceDeployment.Settings.Service.KeyJSONFileName
	serviceAccountEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com",
		instanceDeployment.Core.ServiceName,
		instanceDeployment.Core.SolutionSettings.Hosting.ProjectID)

	global.firestoreClient, err = firestore.NewClient(global.ctx, projectID)
	if err != nil {
		return fmt.Errorf("firestore.NewClient: %v", err)
	}

	serviceAccountKeyNames, err := gfs.ListKeyNames(ctx, global.firestoreClient, instanceDeployment.Core.ServiceName)
	if err != nil {
		return fmt.Errorf("gfs.ListKeyNames %v", err)
	}

	if clientOption, ok = aut.GetClientOptionAndCleanKeys(ctx,
		serviceAccountEmail,
		keyJSONFilePath,
		instanceDeployment.Core.SolutionSettings.Hosting.ProjectID,
		gciAdminUserToImpersonate,
		[]string{"https://www.googleapis.com/auth/apps.groups.settings", "https://www.googleapis.com/auth/admin.directory.group.readonly"},
		serviceAccountKeyNames); !ok {
		return fmt.Errorf("aut.GetClientOptionAndCleanKeys")
	}
	global.dirAdminService, err = admin.NewService(ctx, clientOption)
	if err != nil {
		return fmt.Errorf("admin.NewService: %v", err)
	}
	global.pubSubClient, err = pubsub.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("pubsub.NewClient: %v", err)
	}
	return nil
}

// EntryPoint is the function to be executed for each cloud function occurence
func EntryPoint(ctxEvent context.Context, PubSubMessage gps.PubSubMessage, global *Global) error {
	// log.Println(string(PubSubMessage.Data))
	metadata, err := metadata.FromContext(ctxEvent)
	if err != nil {
		// Assume an error on the function invoker and try again.
		return fmt.Errorf("pubsub_id no available REDO_ON_TRANSIENT metadata.FromContext: %v", err)
	}
	global.PubSubID = metadata.EventID
	expiration := metadata.Timestamp.Add(time.Duration(global.retryTimeOutSeconds) * time.Second)
	if time.Now().After(expiration) {
		log.Printf("pubsub_id %s NORETRY_ERROR pubsub message too old", global.PubSubID)
		return nil
	}

	// Pass data to global variables to deal with func browseGroup
	ctx = global.ctx
	directoryCustomerID = global.directoryCustomerID
	logEventEveryXPubSubMsg = global.logEventEveryXPubSubMsg
	pubSubClient = global.pubSubClient
	outputTopicName = global.outputTopicName
	timestamp = metadata.Timestamp
	pubSubID = global.PubSubID

	if strings.HasPrefix(string(PubSubMessage.Data), "cron schedule") {
		err = initiateQueries(global)
		if err != nil {
			return fmt.Errorf("pubsub_id %s REDO_ON_TRANSIENT initiateQueries: %v", global.PubSubID, err)
		}
	} else {
		var settings Settings
		err = json.Unmarshal(PubSubMessage.Data, &settings)
		if err != nil {
			return fmt.Errorf("pubsub_id %s REDO_ON_TRANSIENT json.Unmarshal(PubSubMessage.Data, &settings) %v", global.PubSubID, err)
		}
		domain = settings.Domain
		emailPrefix = settings.EmailPrefix
		err = queryDirectory(settings.Domain, settings.EmailPrefix, global)
		if err != nil {
			return fmt.Errorf("pubsub_id %s REDO_ON_TRANSIENT queryDirectory: %v", global.PubSubID, err)
		}
	}
	return nil
}

func initiateQueries(global *Global) error {
	figures := getByteSet('0', 10)
	alphabetLower := getByteSet('a', 26)
	// Query on directory group email is NOT case sensitive
	// alphabetUpper := getByteSet('A', 26)

	emailAuthorizedByteSet := append(figures, alphabetLower...)
	// emailAuthorizedByteSet := append(emailAuthorizedByteSet, alphabetUpper...)
	log.Printf("pubsub_id %s initiate multiple queries on emailAuthorizedByteSet: %s", global.PubSubID, string(emailAuthorizedByteSet))

	domains, err := global.dirAdminService.Domains.List(global.directoryCustomerID).Context(global.ctx).Do()
	if err != nil {
		return fmt.Errorf("dirAdminService.Domains.List: %v", err)
	}
	for _, domain := range domains.Domains {
		for _, emailPrefix := range emailAuthorizedByteSet {
			var settings Settings
			settings.Domain = domain.DomainName
			settings.EmailPrefix = string(emailPrefix)
			settingsJSON, err := json.Marshal(settings)
			if err != nil {
				log.Printf("pubsub_id %s NORETRY_ERROR json.Marshal(settings) %v", global.PubSubID, err)
			}
			pubSubMessage := &pubsub.Message{
				Data: settingsJSON,
			}
			topic := global.pubSubClient.Topic(global.inputTopicName)
			id, err := topic.Publish(global.ctx, pubSubMessage).Get(global.ctx)
			if err != nil {
				log.Printf("pubsub_id %s NORETRY_ERROR pubSubClient.Topic initateQuery: %v", global.PubSubID, err)
			}
			log.Printf("pubsub_id %s initiate query domain '%s' emailPrefix '%s' to topic %s msg id: %s", global.PubSubID, settings.Domain, settings.EmailPrefix, global.inputTopicName, id)
		}
	}
	return nil
}

func queryDirectory(domain string, emailPrefix string, global *Global) error {
	log.Printf("pubsub_id %s settings retrieved, launch query on domain '%s' and email prefix '%s'", global.PubSubID, domain, emailPrefix)
	pubSubMsgNumber = 0
	pubSubErrNumber = 0
	query := fmt.Sprintf("email:%s*", emailPrefix)
	log.Printf("query: %s", query)
	// pages function expect just the name of the callback function. Not an invocation of the function
	err := global.dirAdminService.Groups.List().Customer(global.directoryCustomerID).Domain(domain).Query(query).MaxResults(global.maxResultsPerPage).OrderBy("email").Pages(global.ctx, browseGroups)
	if err != nil {
		if strings.Contains(err.Error(), "Domain not found") {
			log.Printf("pubsub_id %s INFO domain not found %s query %s customer ID %s", global.PubSubID, domain, query, global.directoryCustomerID) // NO RETRY
		} else {
			return fmt.Errorf("dirAdminService.Groups.List: %v", err)
		}
	}
	if pubSubMsgNumber > 0 {
		log.Printf("pubsub_id %s finished - Directory %s domain '%s' emailPrefix '%s' Number of groups published %d to topic %s", global.PubSubID, directoryCustomerID, domain, emailPrefix, pubSubMsgNumber, outputTopicName)
	} else {
		log.Printf("pubsub_id %s no group found for directory %s domain '%s' emailPrefix '%s'", global.PubSubID, directoryCustomerID, domain, emailPrefix)
	}
	if pubSubErrNumber > 0 {
		log.Printf("pubsub_id %s NORETRY_ERROR %d messages did not publish successfully", global.PubSubID, pubSubErrNumber)
	}
	return nil
}

// browseGroups is executed for each page returning a set of groups
// A non-nil error returned will halt the iteration
// the only accepted parameter is groups: https://pkg.go.dev/google.golang.org/api/admin/directory/v1?tab=doc#GroupsListCall.Pages
// so, it use global variables to this package
func browseGroups(groups *admin.Groups) error {
	var waitgroup sync.WaitGroup
	topic := pubSubClient.Topic(outputTopicName)
	for _, group := range groups.Groups {
		var feedMessage cai.FeedMessageGroup
		feedMessage.Window.StartTime = timestamp
		feedMessage.Origin = "batch-listgroups"
		feedMessage.Deleted = false
		feedMessage.Asset.Ancestors = []string{fmt.Sprintf("directories/%s", directoryCustomerID)}
		feedMessage.Asset.AncestryPath = fmt.Sprintf("directories/%s", directoryCustomerID)
		feedMessage.Asset.AssetType = "www.googleapis.com/admin/directory/groups"
		feedMessage.Asset.Name = fmt.Sprintf("//directories/%s/groups/%s", directoryCustomerID, group.Id)
		feedMessage.Asset.Resource = group
		feedMessage.Asset.Resource.Etag = ""
		feedMessageJSON, err := json.Marshal(feedMessage)
		if err != nil {
			log.Printf("pubsub_id %s NORETRY_ERROR %s json.Marshal(feedMessage): %v", pubSubID, group.Email, err)
		} else {
			pubSubMessage := &pubsub.Message{
				Data: feedMessageJSON,
			}
			publishResult := topic.Publish(ctx, pubSubMessage)
			waitgroup.Add(1)
			go gps.GetPublishCallResult(ctx, publishResult, &waitgroup, directoryCustomerID+"/"+group.Email, &pubSubErrNumber, &pubSubMsgNumber, logEventEveryXPubSubMsg)
		}
	}
	waitgroup.Wait()
	return nil
}

// getByteSet return a set of lenght contiguous bytes starting at bytes
func getByteSet(start byte, length int) []byte {
	byteSet := make([]byte, length)
	for i := range byteSet {
		byteSet[i] = start + byte(i)
	}
	return byteSet
}
