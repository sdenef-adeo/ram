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

package publish2fs

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/BrunoReboul/ram/utilities/gps"

	"github.com/BrunoReboul/ram/utilities/gcf"
	"gopkg.in/yaml.v2"

	"github.com/BrunoReboul/ram/utilities/grm"
	"github.com/BrunoReboul/ram/utilities/gsu"
	"github.com/BrunoReboul/ram/utilities/iam"
	"github.com/BrunoReboul/ram/utilities/ram"
)

// Deploy a service instance
func (instanceDeployment *InstanceDeployment) Deploy() (err error) {
	start := time.Now()
	if err = instanceDeployment.deployGSUAPI(); err != nil {
		return err
	}
	if err = instanceDeployment.deployIAMServiceAccount(); err != nil {
		return err
	}
	if err = instanceDeployment.deployGRMBindings(); err != nil {
		return err
	}
	if err = instanceDeployment.deployIAMBindings(); err != nil {
		return err
	}
	if err = instanceDeployment.deployGPSTopic(); err != nil {
		return err
	}
	if err = instanceDeployment.deployGCFFunction(); err != nil {
		return err
	}
	log.Printf("%s done in %v minutes", instanceDeployment.Core.InstanceName, time.Since(start).Minutes())
	return nil
}

// ReadValidate reads and validates service and instance settings
func (instanceDeployment *InstanceDeployment) ReadValidate() (err error) {
	serviceConfigFilePath := fmt.Sprintf("%s/%s/%s/%s", instanceDeployment.Core.RepositoryPath, ram.MicroserviceParentFolderName, instanceDeployment.Core.ServiceName, ram.ServiceSettingsFileName)
	if _, err := os.Stat(serviceConfigFilePath); !os.IsNotExist(err) {
		err = ram.ReadValidate(instanceDeployment.Core.ServiceName, "ServiceSettings", serviceConfigFilePath, &instanceDeployment.Settings.Service)
		if err != nil {
			return err
		}
	}
	instanceConfigFilePath := fmt.Sprintf("%s/%s/%s/%s/%s/%s", instanceDeployment.Core.RepositoryPath, ram.MicroserviceParentFolderName, instanceDeployment.Core.ServiceName, ram.InstancesFolderName, instanceDeployment.Core.InstanceName, ram.InstanceSettingsFileName)
	if _, err := os.Stat(instanceConfigFilePath); !os.IsNotExist(err) {
		err = ram.ReadValidate(instanceDeployment.Core.InstanceName, "InstanceSettings", instanceConfigFilePath, &instanceDeployment.Settings.Instance)
		if err != nil {
			return err
		}
	}
	return nil
}

// Situate complement settings taking in account the situation for service and instance settings
func (instanceDeployment *InstanceDeployment) Situate() {
	instanceDeployment.Settings.Service.GCF.FunctionType = "backgroundPubSub"
	instanceDeployment.Settings.Service.GCF.Description = fmt.Sprintf("publish %s assets resource feeds as FireStore documents in collection %s",
		instanceDeployment.Settings.Instance.GCF.TriggerTopic,
		instanceDeployment.Core.SolutionSettings.Hosting.FireStore.CollectionIDs.Assets)
}

func (instanceDeployment *InstanceDeployment) deployGSUAPI() (err error) {
	apiDeployment := gsu.NewAPIDeployment()
	apiDeployment.Core = instanceDeployment.Core
	apiDeployment.Settings.Service.GSU = instanceDeployment.Settings.Service.GSU
	return apiDeployment.Deploy()
}

func (instanceDeployment *InstanceDeployment) deployIAMServiceAccount() (err error) {
	serviceAccountDeployment := iam.NewServiceaccountDeployment()
	serviceAccountDeployment.Core = instanceDeployment.Core
	return serviceAccountDeployment.Deploy()
}

func (instanceDeployment *InstanceDeployment) deployGRMBindings() (err error) {
	bindingsDeployment := grm.NewBindingsDeployment()
	bindingsDeployment.Core = instanceDeployment.Core
	bindingsDeployment.Artifacts.Member = fmt.Sprintf("serviceAccount:%s@%s.iam.gserviceaccount.com", instanceDeployment.Core.ServiceName, instanceDeployment.Core.SolutionSettings.Hosting.ProjectID)
	bindingsDeployment.Settings.Service.GRM = instanceDeployment.Settings.Service.GCF.ServiceAccountBindings.ResourceManager
	return bindingsDeployment.Deploy()
}

func (instanceDeployment *InstanceDeployment) deployIAMBindings() (err error) {
	bindingsDeployment := iam.NewBindingsDeployment()
	bindingsDeployment.Core = instanceDeployment.Core
	bindingsDeployment.Artifacts.Member = fmt.Sprintf("serviceAccount:%s@%s.iam.gserviceaccount.com", instanceDeployment.Core.ServiceName, instanceDeployment.Core.SolutionSettings.Hosting.ProjectID)
	bindingsDeployment.Settings.Service.IAM = instanceDeployment.Settings.Service.GCF.ServiceAccountBindings.IAM
	return bindingsDeployment.Deploy()
}

func (instanceDeployment *InstanceDeployment) deployGPSTopic() (err error) {
	topicDeployment := gps.NewTopicDeployment()
	topicDeployment.Core = instanceDeployment.Core
	topicDeployment.Settings.TopicName = instanceDeployment.Settings.Instance.GCF.TriggerTopic
	return topicDeployment.Deploy()
}

func (instanceDeployment *InstanceDeployment) deployGCFFunction() (err error) {
	instanceDeployment.DumpTimestamp = time.Now()
	instanceDeploymentYAMLBytes, err := yaml.Marshal(instanceDeployment)
	if err != nil {
		return err
	}
	functionDeployment := gcf.NewFunctionDeployment()
	functionDeployment.Core = instanceDeployment.Core
	functionDeployment.Artifacts.InstanceDeploymentYAMLContent = string(instanceDeploymentYAMLBytes)
	functionDeployment.Settings.Service.GCF = instanceDeployment.Settings.Service.GCF
	functionDeployment.Settings.Instance.GCF = instanceDeployment.Settings.Instance.GCF
	return functionDeployment.Deploy()
}
