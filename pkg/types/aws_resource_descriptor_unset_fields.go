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

// AWSResourceDescriptorWithUnsetFields is an optional extension of
// AWSResourceDescriptor. The code-generator emits it for every resource of a
// controller whose generator.yaml sets `unset_fields_unmanaged: true`. It is
// a maintainer setting, chosen per controller at code generation time, not a
// user-facing feature gate.
type AWSResourceDescriptorWithUnsetFields interface {
	// UnsetFieldsUnmanaged returns true when a Spec field the user never set
	// must not drive an Update when the AWS API reports a value for it.
	UnsetFieldsUnmanaged() bool
	// UnsetMeansEmptyPaths returns the Spec fields (Go field names, e.g.
	// "Spec.Policy") for which an unset desired value still means "must be
	// empty" (`compare.unset_means_empty`, plus the resource's tag field).
	UnsetMeansEmptyPaths() []string
}
