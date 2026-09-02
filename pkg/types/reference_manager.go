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

// ReferenceEnsurer restores cross-resource reference (`*Ref`) fields onto an object
// a resource manager built from an AWS API response.
//
// A `*Ref` is a sibling of the concrete field it resolves into --
// `spec.vpcConfig.subnetRefs` next to `spec.vpcConfig.subnetIDs`. An API response has
// no concept of a reference, so rebuilding the containing struct drops every `*Ref`
// inside it. Losing one disables ClearResolvedReferences, which suppresses a resolved
// value only while the sibling `*Ref` is visible, so the spec patch deletes the
// declared `*Ref` and stores the resolved value in its place. The next apply of the
// manifest puts the `*Ref` back beside that value and validateReferenceFields rejects
// the pair ("both resource reference wrapper and ID cannot be used together"),
// stopping reconciliation. See aws-controllers-k8s/community#2361 and #2431.
//
// A top-level `*Ref` survives, because generated set-output code deep-copies the
// incoming object and overwrites only the concrete field. A hand-written set-output
// hook that rebuilds the object wholesale could still drop one, and then has to carry
// the reference across itself.
//
// The reconciler calls this on what a resource manager returns from Create and from
// Update, sourcing the references from the resource the user declared. It is not
// called on the AdoptionPolicy_Adopt branch of Sync, where populating the spec from
// the observed AWS resource is the intended behaviour, so a declared reference is
// expected to be replaced there like any other declared field.
//
// Which shapes are covered is a property of the generated method, not of this
// interface: today only a reference reached through structs. One reached through a
// list has no fixed address and no sound way to pair an observed element with a
// declared one, so those behave as they did before; giving the generated code a
// declared notion of element identity would cover them without changing this
// interface.
//
// Kept separate from ReferenceManager and reached through a type assertion, so
// controllers generated before the method existed still satisfy AWSResourceManager;
// they opt in by regenerating.
//
// TODO: that separation is a compatibility artifact, not a design boundary.
// EnsureReferences belongs beside ResolveReferences and ClearResolvedReferences on
// ReferenceManager -- fold it in at the next release that may break the interface,
// and drop the type assertion in resourceReconciler.ensureReferences with it.
type ReferenceEnsurer interface {
	// EnsureReferences returns a copy of `latest` with any reference field it is
	// missing restored from `desired`. Only reference fields are written.
	//
	// `desired` must be the resource the user declared, not an object that has been
	// through a resource manager: managers may mutate what they are handed, and some
	// write API response values into it.
	EnsureReferences(desired AWSResource, latest AWSResource) AWSResource
}
