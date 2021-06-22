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

package types

import (
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// ConditionManager describes a thing that can set and retrieve Condition
// objects.
type ConditionManager interface {
	// Conditions returns the ACK Conditions collection for the AWSResource
	Conditions() []*ackv1alpha1.Condition
	// ReplaceConditions replaces the resource's set of Condition structs with
	// the supplied slice of Conditions.
	ReplaceConditions([]*ackv1alpha1.Condition)
}
