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

package cai

import (
	"testing"
)

func TestUnitGetAssetShortTypeName(t *testing.T) {
	var testCases = []struct {
		name      string
		assetType string
		want      string
	}{
		{"k8srbacRole", "rbac.authorization.k8s.io/Role", "k8srbac-Role"},
		{"k8sextensionsIngress", "extensions.k8s.io/Ingress", "k8sextensions-Ingress"},
		{"k8snetworkingIngress", "networking.k8s.io/Ingress", "k8snetworking-Ingress"},
		{"k8sPod", "k8s.io/Pod", "k8s-Pod"},
		{"gaeApp", "appengine.googleapis.com/Application", "appengine-Application"},
	}

	for _, tc := range testCases {
		tc := tc // https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := GetAssetShortTypeName(tc.assetType)
			if tc.want != got {
				t.Errorf("Want %s got %s", tc.want, got)
			}
		})
	}
}
