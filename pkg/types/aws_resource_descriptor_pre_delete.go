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
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
)

// AWSResourceDescriptorWithPreDeleteDelta is an optional extension of
// AWSResourceDescriptor that provides a delta function including fields
// normally excluded by compare.is_ignored. This is used during pre-delete
// sync to ensure fields like DeletionProtectionEnabled are compared before
// deletion.
type AWSResourceDescriptorWithPreDeleteDelta interface {
	// DeltaForPreDelete returns an `ackcompare.Delta` that includes fields
	// normally excluded by compare.is_ignored configuration. Used during
	// pre-delete sync to detect all spec differences before calling Delete.
	DeltaForPreDelete(a, b AWSResource) *ackcompare.Delta
}
