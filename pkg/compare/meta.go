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
	"encoding/json"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetaV1ObjectEqual returns true if the supplied k8s.io/apimachinery/pkg/apis/meta/v1.Object
// have equal values.
func MetaV1ObjectEqual(a, b k8smetav1.Object) (bool, error) {
	// If both parameters are nil, return true
	if IsNil(a) && IsNil(b) {
		return true, nil
	}

	// If only one parameter is nil, return false
	if HasNilDifference(a, b) {
		return false, nil
	}

	// Marshall both objects and compare for equality
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false, err
	}

	bBytes, err := json.Marshal(b)
	if err != nil {
		return false, err
	}

	return string(aBytes) == string(bBytes), nil
}
