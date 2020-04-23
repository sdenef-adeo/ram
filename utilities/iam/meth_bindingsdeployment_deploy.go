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

package iam

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/BrunoReboul/ram/utilities/ram"

	"google.golang.org/api/iam/v1"
)

// Retries is the max number of read-modify-write cycles in case of concurrent policy changes detection
const Retries = 5

// Deploy BindingsDeployment use retries on a read-modify-write cycle
func (bindingsDeployment *BindingsDeployment) Deploy() (err error) {
	log.Printf("%s iam bindings on service accounts", bindingsDeployment.Core.InstanceName)
	serviceAccountName := fmt.Sprintf("projects/%s/serviceAccounts/%s@%s.iam.gserviceaccount.com", bindingsDeployment.Core.SolutionSettings.Hosting.ProjectID, bindingsDeployment.Core.ServiceName, bindingsDeployment.Core.SolutionSettings.Hosting.ProjectID)
	projectsServiceAccountsService := bindingsDeployment.Artifacts.IAMService.Projects.ServiceAccounts
	for i := 0; i < Retries; i++ {
		if i > 0 {
			log.Printf("%s retrying a full read-modify-write cycle, iteration %d", bindingsDeployment.Core.InstanceName, i)
		}
		// READ
		var policy *iam.Policy
		policy, err = projectsServiceAccountsService.GetIamPolicy(serviceAccountName).Context(bindingsDeployment.Core.Ctx).Do()
		if err != nil {
			return err
		}
		// MODIFY
		policyIsToBeUpdated := false
		existingRoles := make([]string, 0)
		for _, binding := range policy.Bindings {
			existingRoles = append(existingRoles, binding.Role)
			if ram.Find(bindingsDeployment.Settings.Service.IAM.RolesOnServiceAccounts, binding.Role) {
				isAlreadyMemberOf := false
				for _, member := range binding.Members {
					if member == bindingsDeployment.Artifacts.Member {
						isAlreadyMemberOf = true
					}
				}
				if isAlreadyMemberOf {
					log.Printf("%s member %s already have %s on service account %s", bindingsDeployment.Core.InstanceName, bindingsDeployment.Artifacts.Member, binding.Role, serviceAccountName)
				} else {
					log.Printf("%s add member %s to existing %s on service account %s", bindingsDeployment.Core.InstanceName, bindingsDeployment.Artifacts.Member, binding.Role, serviceAccountName)
					binding.Members = append(binding.Members, bindingsDeployment.Artifacts.Member)
					policyIsToBeUpdated = true
				}
			}
		}
		for _, role := range bindingsDeployment.Settings.Service.IAM.RolesOnServiceAccounts {
			if !ram.Find(existingRoles, role) {
				var binding iam.Binding
				binding.Role = role
				binding.Members = []string{bindingsDeployment.Artifacts.Member}
				log.Printf("%s add new %s with solo member %s on service account %s", bindingsDeployment.Core.InstanceName, binding.Role, bindingsDeployment.Artifacts.Member, serviceAccountName)
				policy.Bindings = append(policy.Bindings, &binding)
				policyIsToBeUpdated = true
			}
		}
		// WRITE
		if policyIsToBeUpdated {
			var setRequest iam.SetIamPolicyRequest
			setRequest.Policy = policy

			var updatedPolicy *iam.Policy
			updatedPolicy, err = projectsServiceAccountsService.SetIamPolicy(serviceAccountName, &setRequest).Context(bindingsDeployment.Core.Ctx).Do()
			if err != nil {
				if !strings.Contains(err.Error(), "There were concurrent policy changes") {
					return err
				}
				log.Printf("%s There were concurrent policy changes, wait 5 sec and retry a full read-modify-write cycle, iteration %d", bindingsDeployment.Core.InstanceName, i)
				time.Sleep(5 * time.Second)
			} else {
				// ram.JSONMarshalIndentPrint(updatedPolicy)
				_ = updatedPolicy
				log.Printf("%s iam policy updated for service account %s iteration %d", bindingsDeployment.Core.InstanceName, serviceAccountName, i)
				break
			}
		} else {
			log.Printf("%s NO need to update iam policy for service account %s", bindingsDeployment.Core.InstanceName, serviceAccountName)
			break
		}
	}
	return err
}