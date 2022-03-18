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

	"github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ctrlreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler is responsible for reconciling the state of any single custom
// resource within the cluster.
//
// The upstream controller-runtime.Manager object ends up managing MULTIPLE
// controller-runtime.Controller objects (each containing a single
// Reconciler object)s and sharing watch and informer queues across
// those controllers.
type Reconciler interface {
	ctrlreconcile.Reconciler
	// SecretValueFromReference fetches the value of a Secret given a
	// SecretKeyReference
	SecretValueFromReference(context.Context, *v1alpha1.SecretKeyReference) (string, error)
}
