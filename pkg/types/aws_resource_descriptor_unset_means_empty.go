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

// AWSResourceDescriptorWithUnsetMeansEmpty is an optional extension of
// AWSResourceDescriptor, implemented by descriptors that the code-generator
// emits from generator.yaml `compare.unset_means_empty` settings.
type AWSResourceDescriptorWithUnsetMeansEmpty interface {
	// UnsetMeansEmptyPaths returns the Spec fields (Go field names, e.g.
	// "Spec.Policy") for which an unset desired value means "must be empty"
	// even when the UnsetFieldsUnmanaged feature gate is enabled. The list
	// includes the resource's tag field.
	UnsetMeansEmptyPaths() []string
}
