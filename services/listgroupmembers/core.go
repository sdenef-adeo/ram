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

package listgroupmembers

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
	"github.com/BrunoReboul/ram/utilities/gcf"
	"github.com/BrunoReboul/ram/utilities/gps"
	"github.com/BrunoReboul/ram/utilities/solution"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	admin "google.golang.org/api/admin/directory/v1"
)

// Global variable to deal with GroupsListCall Pages constraint: no possible to pass variable to the function in pages()
// https://pkg.go.dev/google.golang.org/api/admin/directory/v1?tab=doc#GroupsListCall.Pages
var ancestors []string
var ctx context.Context
var groupAssetName string
var groupEmail string
var logEventEveryXPubSubMsg uint64
var pubSubClient *pubsub.Client
var outputTopicName string
var pubSubErrNumber uint64
var pubSubMsgNumber uint64
var timestamp time.Time
var origin string

// Global structure for global variables to optimize the cloud function performances
type Global struct {
	collectionID            string
	ctx                     context.Context
	dirAdminService         *admin.Service
	firestoreClient         *firestore.Client
	logEventEveryXPubSubMsg uint64
	maxResultsPerPage       int64 // API Max = 200
	outputTopicName         string
	projectID               string
	pubSubClient            *pubsub.Client
	retryTimeOutSeconds     int64
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
		return fmt.Errorf("ERROR - ReadUnmarshalYAML %s %v", solution.SettingsFileName, err)
	}

	gciAdminUserToImpersonate := instanceDeployment.Settings.Instance.GCI.SuperAdminEmail
	global.collectionID = instanceDeployment.Core.SolutionSettings.Hosting.FireStore.CollectionIDs.Assets
	global.logEventEveryXPubSubMsg = instanceDeployment.Settings.Service.LogEventEveryXPubSubMsg
	global.outputTopicName = instanceDeployment.Core.SolutionSettings.Hosting.Pubsub.TopicNames.GCIGroupMembers
	global.maxResultsPerPage = instanceDeployment.Settings.Service.MaxResultsPerPage
	global.projectID = instanceDeployment.Core.SolutionSettings.Hosting.ProjectID
	global.retryTimeOutSeconds = instanceDeployment.Settings.Service.GCF.RetryTimeOutSeconds
	projectID := instanceDeployment.Core.SolutionSettings.Hosting.ProjectID
	keyJSONFilePath := solution.PathToFunctionCode + instanceDeployment.Settings.Service.KeyJSONFileName
	serviceAccountEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com",
		instanceDeployment.Core.ServiceName,
		instanceDeployment.Core.SolutionSettings.Hosting.ProjectID)

	if clientOption, ok = aut.GetClientOptionAndCleanKeys(ctx,
		serviceAccountEmail,
		keyJSONFilePath,
		projectID,
		gciAdminUserToImpersonate,
		[]string{admin.AdminDirectoryGroupMemberReadonlyScope}); !ok {
		return fmt.Errorf("aut.GetClientOptionAndCleanKeys")
	}
	global.dirAdminService, err = admin.NewService(ctx, clientOption)
	if err != nil {
		return fmt.Errorf("ERROR - admin.NewService: %v", err)
	}
	global.firestoreClient, err = firestore.NewClient(global.ctx, global.projectID)
	if err != nil {
		return fmt.Errorf("ERROR - firestore.NewClient: %v", err)
	}
	global.pubSubClient, err = pubsub.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("ERROR - pubsub.NewClient: %v", err)
	}
	return nil
}

// EntryPoint is the function to be executed for each cloud function occurence
func EntryPoint(ctxEvent context.Context, PubSubMessage gps.PubSubMessage, global *Global) error {
	// log.Println(string(PubSubMessage.Data))
	ok, metadata, err := gcf.IntialRetryCheck(ctxEvent, global.retryTimeOutSeconds)
	if !ok {
		return err
	}
	// log.Printf("EventType %s EventID %s Resource %s Timestamp %v", metadata.EventType, metadata.EventID, metadata.Resource.Type, metadata.Timestamp)

	// Pass data to global variables to deal with func browseGroup
	ctx = global.ctx
	logEventEveryXPubSubMsg = global.logEventEveryXPubSubMsg
	pubSubClient = global.pubSubClient
	outputTopicName = global.outputTopicName
	timestamp = metadata.Timestamp

	var feedMessageGroup cai.FeedMessageGroup
	err = json.Unmarshal(PubSubMessage.Data, &feedMessageGroup)
	if err != nil {
		log.Println("ERROR - json.Unmarshal(pubSubMessage.Data, &feedMessageGroup)")
		return nil // NO RETRY
	}

	pubSubMsgNumber = 0
	groupAssetName = feedMessageGroup.Asset.Name
	groupEmail = feedMessageGroup.Asset.Resource.Email
	// First ancestor is my parent
	ancestors = []string{fmt.Sprintf("groups/%s", feedMessageGroup.Asset.Resource.Id)}
	// Next ancestors are my parent ancestors
	for _, ancestor := range feedMessageGroup.Asset.Ancestors {
		ancestors = append(ancestors, ancestor)
	}
	origin = feedMessageGroup.Origin
	if feedMessageGroup.Deleted {
		// retreive members from cache
		err = browseFeedMessageGroupMembersFromCache(global)
		if err != nil {
			return fmt.Errorf("browseFeedMessageGroupMembersFromCache(global): %v", err) // RETRY
		}
	} else {
		// retreive members from admin SDK
		// pages function except just the name of the callback function. Not an invocation of the function
		err = global.dirAdminService.Members.List(feedMessageGroup.Asset.Resource.Id).MaxResults(global.maxResultsPerPage).Pages(ctx, browseMembers)
		if err != nil {
			return fmt.Errorf("dirAdminService.Members.List: %v", err) // RETRY
		}
	}
	log.Printf("Completed - Group %s %s isDeleted %v Number of members published to pubsub topic %s: %d",
		feedMessageGroup.Asset.Resource.Email,
		feedMessageGroup.Asset.Resource.Id,
		feedMessageGroup.Deleted,
		outputTopicName,
		pubSubMsgNumber)
	return nil
}

func browseFeedMessageGroupMembersFromCache(global *Global) (err error) {
	var waitgroup sync.WaitGroup
	var documentSnap *firestore.DocumentSnapshot
	var feedMessageMember cai.FeedMessageMember
	topic := pubSubClient.Topic(outputTopicName)
	assets := global.firestoreClient.Collection(global.collectionID)
	query := assets.Where(
		"asset.assetType", "==", "www.googleapis.com/admin/directory/members").Where(
		"asset.resource.groupEmail", "==", strings.ToLower(groupEmail))
	iter := query.Documents(global.ctx)
	defer iter.Stop()
	for {
		documentSnap, err = iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("ERROR - iter.Next() %v", err) // log and move to next
		} else {
			if documentSnap.Exists() {
				err = documentSnap.DataTo(&feedMessageMember)
				if err != nil {
					log.Printf("documentSnap.DataTo %v", err) // log and move to next
				} else {
					feedMessageMember.Deleted = true
					feedMessageMember.Window.StartTime = timestamp
					feedMessageMember.Origin = origin
					feedMessageMemberJSON, err := json.Marshal(feedMessageMember)
					if err != nil {
						log.Printf("ERROR - %s json.Marshal(feedMessageMember): %v", feedMessageMember.Asset.Name, err)
					} else {
						pubSubMessage := &pubsub.Message{
							Data: feedMessageMemberJSON,
						}
						publishResult := topic.Publish(ctx, pubSubMessage)
						waitgroup.Add(1)
						go gps.GetPublishCallResult(ctx, publishResult, &waitgroup, feedMessageMember.Asset.Name, &pubSubErrNumber, &pubSubMsgNumber, logEventEveryXPubSubMsg)
					}
				}
			} else {
				log.Printf("ERROR - document does not exists %s", documentSnap.Ref.Path)
			}
		}
	}
	return nil
}

// browseMembers is executed for each page returning a set of members
// A non-nil error returned will halt the iteration
// the only accepted parameter is groups: https://pkg.go.dev/google.golang.org/api/admin/directory/v1?tab=doc#GroupsListCall.Pages
// so, it use global variables to this package
func browseMembers(members *admin.Members) error {
	var waitgroup sync.WaitGroup
	topic := pubSubClient.Topic(outputTopicName)
	for _, member := range members.Members {
		var feedMessageMember cai.FeedMessageMember
		feedMessageMember.Window.StartTime = timestamp
		feedMessageMember.Origin = origin
		feedMessageMember.Asset.Ancestors = ancestors
		feedMessageMember.Asset.AncestryPath = groupAssetName
		feedMessageMember.Asset.AssetType = "www.googleapis.com/admin/directory/members"
		feedMessageMember.Asset.Name = groupAssetName + "/members/" + member.Id
		feedMessageMember.Asset.Resource.GroupEmail = groupEmail
		feedMessageMember.Asset.Resource.MemberEmail = member.Email
		feedMessageMember.Asset.Resource.ID = member.Id
		feedMessageMember.Asset.Resource.Kind = member.Kind
		feedMessageMember.Asset.Resource.Role = member.Role
		feedMessageMember.Asset.Resource.Type = member.Type
		feedMessageMemberJSON, err := json.Marshal(feedMessageMember)
		if err != nil {
			log.Printf("ERROR - %s json.Marshal(feedMessageMember): %v", member.Email, err)
		} else {
			pubSubMessage := &pubsub.Message{
				Data: feedMessageMemberJSON,
			}
			publishResult := topic.Publish(ctx, pubSubMessage)
			waitgroup.Add(1)
			go gps.GetPublishCallResult(ctx, publishResult, &waitgroup, groupAssetName+"/"+member.Email, &pubSubErrNumber, &pubSubMsgNumber, logEventEveryXPubSubMsg)
		}
	}
	return nil
}
