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

package monitor

import (
	"fmt"

	"github.com/BrunoReboul/ram/utilities/grm"
)

func (instanceDeployment *InstanceDeployment) deployGRMMonitoringOrgBindings() (err error) {
	orgBindingsDeployment := grm.NewOrgBindingsDeployment()
	orgBindingsDeployment.Core = instanceDeployment.Core
	orgBindingsDeployment.Settings.Roles = instanceDeployment.Settings.Service.GCF.ServiceAccountBindings.GRM.Monitoring.Org.Roles
	orgBindingsDeployment.Settings.CustomRoles = instanceDeployment.Settings.Service.GCF.ServiceAccountBindings.GRM.Monitoring.Org.CustomRoles
	for _, organizationID := range orgBindingsDeployment.Core.SolutionSettings.Monitoring.OrganizationIDs {
		orgBindingsDeployment.Artifacts.OrganizationID = organizationID
		orgBindingsDeployment.Artifacts.Member = fmt.Sprintf("serviceAccount:%s@%s.iam.gserviceaccount.com", instanceDeployment.Core.ServiceName, instanceDeployment.Core.SolutionSettings.Hosting.ProjectID)
		err = orgBindingsDeployment.Deploy()
		if err != nil {
			break
		}
	}
	return err
}
