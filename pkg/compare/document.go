// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package compare

import (
	"fmt"
	"reflect"

	k8syaml "sigs.k8s.io/yaml"
)

// DocumentEqual compares two strings that may contain JSON or YAML content.
// Returns true if they are semantically equivalent, ignoring whitespace and
// key ordering differences. Uses sigs.k8s.io/yaml for parsing, which
// normalizes both JSON and YAML into consistent Go types (handling the
// YAML-is-a-superset-of-JSON relationship and number type mismatches).
//
// Returns an error if either string cannot be parsed as JSON or YAML.
func DocumentEqual(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	var aObj, bObj interface{}
	if err := k8syaml.Unmarshal([]byte(a), &aObj); err != nil {
		return false, fmt.Errorf("failed to parse first document: %w", err)
	}
	if err := k8syaml.Unmarshal([]byte(b), &bObj); err != nil {
		return false, fmt.Errorf("failed to parse second document: %w", err)
	}
	return reflect.DeepEqual(aObj, bObj), nil
}
