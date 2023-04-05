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
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolvedReferenceManager describes a thing that can set and retrieve the
// value of resolved references within a resource.
type ResolvedReferenceManager interface {
	// ResolveReferences finds if there are any Reference field(s) present
	// inside AWSResource passed in the parameter and attempts to resolve
	// those reference field(s) into the resolved references map.
	// It returns a boolean that is set to true if there the resource is using
	// references, and an error if the passed AWSResource's reference field(s)
	// cannot be resolved.
	ResolveReferences(context.Context, client.Reader, AWSResource) (bool, error)
	CopyWithResolvedReferences(AWSResource) AWSResource
	// ClearResolvedReferences removes any reference values that were made
	// concrete in the spec. It returns a copy of the spec which contains the
	// original *Ref values, but none of their respective values, and optionally
	// an error.
	ClearResolvedReferences(AWSResource) (AWSResource, error)
}
