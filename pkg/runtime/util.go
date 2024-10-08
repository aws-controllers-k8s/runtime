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

package runtime

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// TODO(jaypipes): Place this code somewhere separate

// IsAdopted returns true if the supplied AWSResource was created with a
// non-nil ARN annotation, which indicates that the Kubernetes user who created
// the CR for the resource expects the ACK service controller to "adopt" a
// pre-existing resource and bring it under ACK management.
func IsAdopted(res acktypes.AWSResource) bool {
	mo := res.MetaObject()
	if mo == nil {
		// Should never happen... if it does, it's buggy code.
		panic("IsAdopted received resource with nil RuntimeObject")
	}
	for k, v := range mo.GetAnnotations() {
		if k == ackv1alpha1.AnnotationAdopted {
			return strings.ToLower(v) == "true"
		}
	}
	return false
}

// IsSynced returns true if the supplied AWSResource's CR and associated
// backend AWS service API resource are in sync.
func IsSynced(res acktypes.AWSResource) bool {
	for _, c := range res.Conditions() {
		if c.Type == ackv1alpha1.ConditionTypeResourceSynced {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// IsReadOnly returns true if the supplied AWSResource has an annotation
// indicating that it is in read-only mode.
func IsReadOnly(res acktypes.AWSResource) bool {
	mo := res.MetaObject()
	if mo == nil {
		// Should never happen... if it does, it's buggy code.
		panic("IsReadOnly received resource with nil RuntimeObject")
	}
	for k, v := range mo.GetAnnotations() {
		if k == ackv1alpha1.AnnotationReadOnly {
			return strings.ToLower(v) == "true"
		}
	}
	return false
}
