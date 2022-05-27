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

package tags

// Tags represents the AWS tags which will be added to the AWS resource.
// Inside aws-sdk-go, Tags are represented using multiple types, Ex: map of
// string, list of structs etc...
// Tags type will be used as a hub/mediator to merge tags represented
// using different types.
type Tags map[string]string

// NewTags returns Tags with empty tags
func NewTags() Tags {
	return map[string]string{}
}

// Merge merges two set of Tags and returns the merge result.
// In case of collision precedence is given to tags present in the first
// parameter 'a'.
func Merge(a Tags, b Tags) Tags {
	var result Tags
	// Initialize result with the first set of tags 'a'.
	// If first set is nil, initialize result with empty set of tags.
	if a == nil {
		result = NewTags()
	} else {
		result = a
	}
	if b != nil && len(b) > 0 {
		// Add all the tags which are not already present in result
		for tk, tv := range b {
			if _, found := result[tk]; !found {
				result[tk] = tv
			}
		}
	}
	return result
}
