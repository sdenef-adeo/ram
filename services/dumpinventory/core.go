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

package dumpinventory

import (
	"context"
	"fmt"
	"log"
	"os"

	asset "cloud.google.com/go/asset/apiv1"
	assetpb "google.golang.org/genproto/googleapis/cloud/asset/v1"

	"github.com/BrunoReboul/ram/utilities/ram"
)

// Global structure for global variables to optimize the cloud function performances
type Global struct {
	ctx                 context.Context
	initFailed          bool
	retryTimeOutSeconds int64
	assetClient         *asset.Client
	request             *assetpb.ExportAssetsRequest
}

// Initialize is to be executed in the init() function of the cloud function to optimize the cold start
func Initialize(ctx context.Context, global *Global) {
	global.ctx = ctx
	global.initFailed = false

	// err is pre-declared to avoid shadowing client.
	var err error
	var instanceDeployment InstanceDeployment

	log.Println("Function COLD START")
	err = ram.ReadUnmarshalYAML(fmt.Sprintf("./%s", ram.SettingsFileName), &instanceDeployment)
	if err != nil {
		log.Printf("ERROR - ReadUnmarshalYAML %s %v", ram.SettingsFileName, err)
		global.initFailed = true
		return
	}

	global.retryTimeOutSeconds = instanceDeployment.Settings.Service.GCF.RetryTimeOutSeconds

	var gcsDestinationURI assetpb.GcsDestination_Uri
	gcsDestinationURI.Uri = fmt.Sprintf("gs://%s/%s.dump",
		instanceDeployment.Core.SolutionSettings.Hosting.GCS.Buckets.CAIExport.Name,
		os.Getenv("FUNCTION_NAME"))

	var gcsDestination assetpb.GcsDestination
	gcsDestination.ObjectUri = &gcsDestinationURI

	var outputConfigGCSDestination assetpb.OutputConfig_GcsDestination
	outputConfigGCSDestination.GcsDestination = &gcsDestination

	var outputConfig assetpb.OutputConfig
	outputConfig.Destination = &outputConfigGCSDestination

	log.Println(instanceDeployment.Settings.Instance.CAI.ContentType)
	switch instanceDeployment.Settings.Instance.CAI.ContentType {
	case "RESOURCE":
		log.Println("case RESOURCE")
		log.Println(assetpb.ContentType_RESOURCE)
		global.request.ContentType = assetpb.ContentType_RESOURCE
		log.Println(global.request.ContentType)
	case "IAM_POLICY":
		log.Println("case IAM_Policy")
		log.Println(assetpb.ContentType_IAM_POLICY)
		global.request.ContentType = assetpb.ContentType_IAM_POLICY
		log.Println(global.request.ContentType)
	default:
		log.Printf("ERROR - unsupported content type: %s", instanceDeployment.Settings.Instance.CAI.ContentType)
		global.initFailed = true
		return
	}
	log.Println(global.request.ContentType)

	log.Println("hello4")
	global.request.Parent = instanceDeployment.Settings.Instance.CAI.Parent

	log.Println("hello5")
	global.request.AssetTypes = instanceDeployment.Settings.Instance.CAI.AssetTypes

	log.Println("hello6")
	global.request.OutputConfig = &outputConfig

	ram.YAMLMarshalPrint(global.request)

	global.assetClient, err = asset.NewClient(ctx)
	if err != nil {
		log.Printf("ERROR - asset.NewClient: %v", err)
		global.initFailed = true
		return
	}
}

// EntryPoint is the function to be executed for each cloud function occurence
func EntryPoint(ctxEvent context.Context, PubSubMessage ram.PubSubMessage, global *Global) error {
	// log.Println(string(PubSubMessage.Data))
	if ok, _, err := ram.IntialRetryCheck(ctxEvent, global.initFailed, global.retryTimeOutSeconds); !ok {
		return err
	}
	// log.Printf("EventType %s EventID %s Resource %s Timestamp %v", metadata.EventType, metadata.EventID, metadata.Resource.Type, metadata.Timestamp)

	operation, err := global.assetClient.ExportAssets(global.ctx, global.request)
	if err != nil {
		return fmt.Errorf("assetClient.ExportAssets: %v", err) // RETRY
	}
	log.Printf("gcloud asset operations describe %s %v", operation.Name(), global.request)
	// do NOT wait for response to save function execution time, and avoid function timeout
	return nil
}
