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
	"io/ioutil"

	"github.com/BrunoReboul/ram/utilities/solution"
)

// audit.rego code
const auditRego = `
#
# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

package validator.gcp.lib

# RULE: the audit rule is result if its body {} evaluates to true
# The rule body {} can be understood intuitively as: expression-1 AND expression-2 AND ... AND expression-N
audit[result] {
	# iterate over each asset
	asset := data.assets[_]
	# assign the constrains in a variable
	constraints := data.constraints
	# iterate over each constraint
	constraint := constraints[_]
	# use a custom function to retreive constraint.spec, if not defined returns a default value that is an empty object
	spec := _get_default(constraint, "spec", {})
	# use a custom function to retreive constraint.spec.match, if not defined returns a default value that is an empty objecy
	match := _get_default(spec, "match", {})
	# use a custom function to retreive constraint.spec.match.target, if not defined returns a default value that is an array with one target object targetting any organization, and childs
	target := _get_default(match, "target", ["organization/*"])
	# use a custom function to retreive constraint.spec.match.gcp, if not defined returns a default value that is an empty object
	gcp := _get_default(match, "gcp", {})
	# use a custom function to retreive constraint.spec.match.gcp.target, if not defined returns what we already got in target variable
	gcp_target := _get_default(gcp, "target", target)
	# iterate over each target and use builtin regex to check if the asset ancestry path matches one of them
	# TRUE if the asset ancestry path matches (regex) one of the target (iterate targets)
	# FALSE when the ancestry path do not matches at least one of the targer
	trace(sprintf("targets: %v", [gcp_target]))
	trace(sprintf("asset.ancestry_path: %v", [asset.ancestry_path]))
	re_match(gcp_target[_], asset.ancestry_path)
	# use a custom function to retreive constraint.spec.match.exclude, if not defined returns a default value that is an empty array
	exclude := _get_default(match, "exclude", [])
	# use a custom function to retreive constraint.spec.match.gcp.exclude, if not defined returns what we already got in exlucde variable
	gcp_exclude := _get_default(gcp, "exclude", exclude)
	# iterate over the exclusion list (the pattern) and use regex builtin function to check if the asset ancestry path (the value) matches one of then
	# assign to exclusion_match variable the virtual document generated by a rule which is a set built by using a comprehension
	# This set containts the asset ancestry path if it maches one of the exclusion
	# or is empty (set()) if the asset ancestry path does not matched any of the exclusion
	exclusion_match := {asset.ancestry_path | re_match(gcp_exclude[_], asset.ancestry_path)}
	trace(sprintf("exclusions: %v", [gcp_exclude]))
	trace(sprintf("exclusion_match: %v", [exclusion_match]))
	trace(sprintf("count exclusion_match: %v", [count(exclusion_match)]))
	# this expression evaluate to true when count is zero, aka the ancestry path does not matches any of the exclusion, otherwise evaluates to false
	count(exclusion_match) == 0
	# Use a with statement to programatically call the rego rule that is specified in the YAML constraint file
	violations := data.templates.gcp[constraint.kind].deny with input.asset as asset
		 with input.constraint as constraint
	# Iterates through each violation found
	violation := violations[_]
	# if the asset is in target, and not excluded and at least one violation is founds, then returns for each violation a result object with the 4 following fields:
	result := {
		"asset": asset.name,
		"constraint": constraint.metadata.name,
		"constraint_config": constraint,
		"violation": violation,
	}
}

# has_field returns whether an object has a field
_has_field(object, field) {
	object[field]
}

# False is a tricky special case, as false responses would create an undefined document unless
# they are explicitly tested for
_has_field(object, field) {
	object[field] == false
}

_has_field(object, field) = false {
	not object[field]
	not object[field] == false
}

# get_default returns the value of an object's field or the provided default value.
# It avoids creating an undefined state when trying to access an object attribute that does
# not exist
_get_default(object, field, _default) = output {
	_has_field(object, field)
	output = object[field]
}

_get_default(object, field, _default) = output {
	_has_field(object, field) == false
	output = _default
}
`

// constraints.rego code
const constraintsRego = `
#
# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

package validator.gcp.lib

# Function to fetch the constraint spec
# Usage:
# get_constraint_params(constraint, params)

get_constraint_params(constraint) = params {
	params := constraint.spec.parameters
}

# Function to fetch constraint info
# Usage:
# get_constraint_info(constraint, info)

get_constraint_info(constraint) = info {
	info := {
		"name": constraint.metadata.name,
		"kind": constraint.kind,
	}
}
`

// util.rego code
const utilRego = `
#
# Copyright 2018 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

package validator.gcp.lib

# has_field returns whether an object has a field
has_field(object, field) {
	object[field]
}

# False is a tricky special case, as false responses would create an undefined document unless
# they are explicitly tested for
has_field(object, field) {
	object[field] == false
}

has_field(object, field) = false {
	not object[field]
	not object[field] == false
}

# get_default returns the value of an object's field or the provided default value.
# It avoids creating an undefined state when trying to access an object attribute that does
# not exist
get_default(object, field, _default) = output {
	has_field(object, field)
	output = object[field]
}

get_default(object, field, _default) = output {
	has_field(object, field) == false
	output = _default
}
`

// MakeZipSpecificContent
func (instanceDeployment *InstanceDeployment) makeZipSpecificContent() (specificZipFiles map[string]string, err error) {
	specificZipFiles = make(map[string]string)
	specificZipFiles["opa/modules/audit.rego"] = auditRego
	specificZipFiles["opa/modules/constraints.rego"] = constraintsRego
	specificZipFiles["opa/modules/util.rego"] = utilRego

	regoRuleFilePath := fmt.Sprintf("%s/%s/%s/%s/%s/%s.rego",
		instanceDeployment.Core.RepositoryPath,
		solution.MicroserviceParentFolderName,
		instanceDeployment.Core.ServiceName,
		solution.InstancesFolderName,
		instanceDeployment.Core.InstanceName,
		instanceDeployment.Core.InstanceName)
	bytes, err := ioutil.ReadFile(regoRuleFilePath)
	if err != nil {
		return make(map[string]string), err
	}
	specificZipFiles[fmt.Sprintf("opa/modules/%s.rego", instanceDeployment.Core.InstanceName)] = string(bytes)

	regoConstraintsFolderPath := fmt.Sprintf("%s/%s/%s/%s/%s/%s",
		instanceDeployment.Core.RepositoryPath,
		solution.MicroserviceParentFolderName,
		instanceDeployment.Core.ServiceName,
		solution.InstancesFolderName,
		instanceDeployment.Core.InstanceName,
		solution.RegoConstraintsFolderName)
	childs, err := ioutil.ReadDir(regoConstraintsFolderPath)
	if err != nil {
		return make(map[string]string), err
	}
	for _, child := range childs {
		if child.IsDir() {
			constraintName := child.Name()
			bytes, err := ioutil.ReadFile(fmt.Sprintf("%s/%s/constraint.yaml", regoConstraintsFolderPath, constraintName))
			if err != nil {
				return make(map[string]string), err
			}
			specificZipFiles[fmt.Sprintf("opa/constraints/%s/constraint.yaml", constraintName)] = string(bytes)
		}
	}
	return specificZipFiles, err
}
