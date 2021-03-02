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

package v1alpha1

import (
	k8scorev1 "k8s.io/api/core/v1"
)

// SecretKeyReference combines a k8s corev1.SecretReference with a
// specific key within the referred-to Secret
type SecretKeyReference struct {
	// Empty JSON tag is required to solve encountered struct field "" without JSON tag  error.
	k8scorev1.SecretReference `json:""`
	// Key is the key within the secret
	Key string `json:"key"`
}