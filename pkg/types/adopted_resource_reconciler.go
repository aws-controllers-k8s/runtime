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

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ctrlrt "sigs.k8s.io/controller-runtime"
)

// AdoptedResourceReconciler is responsible for reconciling an adopted resource
// that represent AWS service API resource.
// It implements the upstream controller-runtime `Reconciler`
// interface.
type AdoptedResourceReconciler interface {
	Reconciler
	// BindControllerManager sets up the AdoptedResourceReconciler with an
	// instance of an upstream controller-runtime.Manager
	BindControllerManager(ctrlrt.Manager) error
	// Sync ensures that the supplied AdoptedResource creates the matching
	// AWSResource based on observed state from ReadOne method
	//
	//
	// NOTE(vijtrip2): This is really only here for dependency injection
	// purposes in unit testing in order to simplify test setups.
	Sync(
		context.Context,
		AWSResourceDescriptor,
		AWSResourceManager,
		*ackv1alpha1.AdoptedResource,
	) error
}
