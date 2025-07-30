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

// AWSResourceIdentifiers has methods that returns common identifying
// information about a resource
type AWSResourceIdentifiers interface {
	// OwnerAccountID returns the AWS account identifier in which the
	// backend AWS resource resides, or should reside in.
	OwnerAccountID() *ackv1alpha1.AWSAccountID
	// ARN returns the AWS Resource Name for the backend AWS resource. If nil,
	// this means the resource has not yet been created in the backend AWS
	// service.
	ARN() *ackv1alpha1.AWSResourceName
	// Region is the AWS region in which the resource exists or will exist.
	Region() *ackv1alpha1.AWSRegion
	// Partition is the AWS partition in which the resource exists or will exist.
	Partition() *ackv1alpha1.AWSPartition
}
