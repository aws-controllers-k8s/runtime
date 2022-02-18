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
)

// FieldExportReconciler is responsible for reconciling a field export CR.
// It implements the upstream controller-runtime `Reconciler`
// interface.
type FieldExportReconciler interface {
	Reconciler
	// Sync ensures that the supplied FieldExport has been registered and is
	// actively monitoring the resources.
	Sync(
		context.Context,
		AWSResourceDescriptor,
		AWSResourceManager,
		*ackv1alpha1.FieldExport,
	) error
}
