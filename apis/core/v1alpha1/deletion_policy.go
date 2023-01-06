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

import "errors"

// DeletionPolicy represents how the ACK reconciler will handle the deletion of
// a resource. A DeletionPolicy of "delete" will delete the underlying AWS
// resource, whereas a DeletionPolicy of "retain" will only delete the K8s
// object leaving the AWS resource intact.
type DeletionPolicy string

const (
	DeletionPolicyDelete DeletionPolicy = "delete"
	DeletionPolicyRetain DeletionPolicy = "retain"
)

func (e *DeletionPolicy) String() string {
	return string(*e)
}

func (e *DeletionPolicy) Set(v string) error {
	switch v {
	case string(DeletionPolicyDelete), string(DeletionPolicyRetain):
		*e = DeletionPolicy(v)
		return nil
	default:
		return errors.New(`invalid DeletionPolicy value`)
	}
}

func (e *DeletionPolicy) Type() string {
	return "DeletionPolicy"
}
