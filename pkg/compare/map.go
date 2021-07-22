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

// MapStringStringPEqual returns true if the supplied maps are equal
func MapStringStringPEqual(a, b map[string]*string) bool {
	if len(a) != len(b) {
		return false
	}
	for aKey, aVal := range a {
		if bVal, ok := b[aKey]; !ok || *bVal != *aVal {
			return false
		}
	}
	return true
}

// MapStringStringEqual returns true if the supplied maps are equal
func MapStringStringEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for aKey, aVal := range a {
		if bVal, ok := b[aKey]; !ok || bVal != aVal {
			return false
		}
	}
	return true
}
