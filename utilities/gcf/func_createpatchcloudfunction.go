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

package gcf

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/BrunoReboul/ram/utilities/ram"
)

// CreatePatchCloudFunction looks for and existing cloud function
func (goGCFArtifacts *GoGCFArtifacts) CreatePatchCloudFunction() (err error) {
	location := fmt.Sprintf("projects/%s/locations/%s", goGCFArtifacts.ProjectID, goGCFArtifacts.Region)

	operation, err := goGCFArtifacts.ProjectsLocationsFunctionsService.Create(location,
		&goGCFArtifacts.CloudFunction).Context(goGCFArtifacts.Ctx).Do()

	if err != nil {
		if strings.Contains(err.Error(), "alreadyExists") {
			log.Printf("%s patch existing cloud function", goGCFArtifacts.InstanceName)
			operation, err = goGCFArtifacts.ProjectsLocationsFunctionsService.Patch(goGCFArtifacts.CloudFunction.Name,
				&goGCFArtifacts.CloudFunction).Context(goGCFArtifacts.Ctx).Do()
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	name := operation.Name
	log.Printf("%s cloud function deployment started", goGCFArtifacts.InstanceName)
	log.Println(name)
	for {
		time.Sleep(5 * time.Second)
		operation, err = goGCFArtifacts.OperationsService.Get(name).Context(goGCFArtifacts.Ctx).Do()
		if err != nil {
			return err
		}
		if operation.Done {
			break
		}
	}
	ram.JSONMarshalIndentPrint(operation)
	return nil
}