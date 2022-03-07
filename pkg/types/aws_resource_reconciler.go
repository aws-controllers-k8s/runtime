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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlrt "sigs.k8s.io/controller-runtime"
)

// AWSResourceReconciler is responsible for reconciling the state of a SINGLE
// KIND of Kubernetes custom resources (CRs) that represent AWS service API
// resources.  It implements the upstream controller-runtime `Reconciler`
// interface.
//
// The upstream controller-runtime.Manager object ends up managing MULTIPLE
// controller-runtime.Controller objects (each containing a single
// AWSResourceReconciler object)s and sharing watch and informer queues across
// those controllers.
type AWSResourceReconciler interface {
	Reconciler
	// BindControllerManager sets up the AWSResourceReconciler with an instance
	// of an upstream controller-runtime.Manager
	BindControllerManager(ctrlrt.Manager) error
	// GroupKind returns the
	// sigs.k8s.io/apimachinery/pkg/apis/meta/v1.GroupKind containing the API
	// group and kind reconciled by this reconciler
	GroupKind() *metav1.GroupKind
	// Sync ensures that the supplied AWSResource's backing API resource
	// matches the supplied desired state.
	//
	// It returns a copy of the resource that represents the latest observed
	// state.
	//
	// NOTE(jaypipes): This is really only here for dependency injection
	// purposes in unit testing in order to simplify test setups.
	Sync(
		context.Context,
		AWSResourceManager,
		AWSResource,
	) (AWSResource, error)
	// HandleReconcileError will handle errors from reconcile handlers, which
	// respects runtime errors.
	//
	// If the `latest` parameter is not nil, this function will ALWAYS patch the
	// latest Status fields back to the Kubernetes API.
	//
	// NOTE(jaypipes): This is really only here for dependency injection
	// purposes in unit testing in order to simplify test setups.
	HandleReconcileError(
		ctx context.Context,
		desired AWSResource,
		latest AWSResource,
		err error,
	) (ctrlrt.Result, error)
}
