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

// Package ramdeploy is a utility to hel deploying ram
package ramdeploy

// SolutionSettings settings common to all services / all instances
type SolutionSettings struct {
	Hosting struct {
		BillingAccountID string            `yaml:"billingAccountID"`
		FolderIDs        map[string]string `yaml:"folderIDs"`
		ProjectIDs       map[string]string `yaml:"projectIDs"`
		StackDriver      struct {
			ProjectIDs map[string]string `yaml:"projectIDs"`
		}
		Repository struct {
			Name string
		}
		GAE struct {
			Region string
		}
		GCF struct {
			Region string
		}
		GCS struct {
			BucketNames struct {
				CAIExport      map[string]string `yaml:"CAIExport"`
				AssetsJSONFile map[string]string `yaml:"assetsJSONFile"`
			}
		}
		Bigquery struct {
			Dataset struct {
				Name     string
				Location string
			}
		}
		PubSub struct {
			TopicNames struct {
				IAMPolicies         string `yaml:"IAMPolicies"`
				RAMViolation        string `yaml:"RAMViolation"`
				RAMComplianceStatus string `yaml:"RAMComplianceStatus"`
			}
		}
		FireStore struct {
			CollectionIDs struct {
				Assets string
			}
		}
	}
	Monitoring struct {
		OrganizationIDs      []string          `yaml:"organizationIDs"`
		DirectoryCustomerIDs map[string]string `yaml:"directoryCustomerIDs"`
		AssetTypes           struct {
			IAMPolicies []string `yaml:"iamPolicies"`
			Resources   []string `yaml:"resources"`
		}
		LabelKeyNames struct {
			Owner             string
			ViolationResolver string `yaml:"violationResolver"`
		}
	}
}

// GetProjectID returns the project ID for a given environment name
func (solutionSettings *SolutionSettings) GetProjectID(environmentName string) string {
	return solutionSettings.Hosting.ProjectIDs[environmentName]
}
