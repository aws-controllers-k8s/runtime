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

// ReferenceManager describes a thing that can resolve and clear references
// within an AWSResource.
type ReferenceManager interface {
	// ResolveReferences finds if there are any Reference field(s) present
	// inside AWSResource passed in the parameter and attempts to resolve those
	// reference field(s) into their respective target field(s). It returns a
	// copy of the input AWSResource with resolved reference(s), a boolean which
	// is set to true if the resource contains any references (regardless of if
	// they are resolved successfully) and an error if the passed AWSResource's
	// reference field(s) could not be resolved.
	ResolveReferences(context.Context, client.Reader, AWSResource) (AWSResource, bool, error)
	// ClearResolvedReferences removes any reference values that were made
	// concrete in the spec. It returns a copy of the input AWSResource which
	// contains the original *Ref values, but none of their respective concrete
	// values.
	ClearResolvedReferences(AWSResource) AWSResource
}

// ReferenceEnsurer restores cross-resource reference (`*Ref`) fields onto an
// object a resource manager built from an AWS API response.
//
// A `*Ref` is generated as a sibling of the concrete field it resolves into --
// `spec.vpcConfig.subnetRefs` next to `spec.vpcConfig.subnetIDs`. An API response
// has no concept of a reference, so rebuilding the containing struct drops every
// `*Ref` inside it. A top-level `*Ref` survives, because generated set-output code
// deep-copies the incoming object and overwrites only the concrete field.
//
// Losing it disables ClearResolvedReferences, which suppresses a resolved value
// only while the sibling `*Ref` is visible. The spec patch then deletes the
// declared `*Ref` and stores the resolved value in its place. Reconciliation
// carries on until the manifest is applied again, at which point the restored
// `*Ref` sits beside the stored value and validateReferenceFields rejects the pair
// ("both resource reference wrapper and ID cannot be used together"), stopping
// reconciliation. See aws-controllers-k8s/community#2361 and #2431.
//
// Kept separate from ReferenceManager and reached through a type assertion, so
// controllers generated before the method existed still satisfy
// AWSResourceManager. They opt in by regenerating.
//
// What actually gets restored is a property of the generated method, not of this
// interface. Today the generator emits an assignment only for a reference reached
// through structs: one reached through a list has no fixed address, and pairing an
// element the service reported with an element the user declared has no sound key,
// so those references behave as they did before. Given a declared notion of element
// identity the generated code could cover them too, without this interface
// changing.
//
// The reconciler calls this after Create and after Update, the two points where a
// manager returns an object whose spec is about to be patched back. The
// ReadOne-derived spec writes -- the adoption branch of Sync, and deleteResource
// -- need the same treatment and do not have it yet.
type ReferenceEnsurer interface {
	// EnsureReferences returns a copy of `latest` with any reference field it is
	// missing restored from `desired`. Only reference fields are written.
	//
	// `desired` must be the DECLARED resource with its references resolved, and must
	// not be an object that has been through a resource manager: managers may mutate
	// what they are handed, and some write API response values into it.
	EnsureReferences(desired AWSResource, latest AWSResource) AWSResource
}
