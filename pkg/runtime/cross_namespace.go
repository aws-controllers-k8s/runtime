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

package runtime

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
)

// CrossNamespaceRefKind is a label used in cross-namespace warning logs and
// condition messages to describe which kind of reference triggered the
// deprecation warning.
type CrossNamespaceRefKind string

const (
	// CrossNamespaceRefKindResource indicates an AWSResourceReference
	// targeting a different namespace.
	CrossNamespaceRefKindResource CrossNamespaceRefKind = "resource reference"
	// CrossNamespaceRefKindSecret indicates a SecretKeyReference targeting
	// a different namespace.
	CrossNamespaceRefKindSecret CrossNamespaceRefKind = "secret reference"
)

// SetCrossNamespaceOptInRequired sets or updates the
// ACK.CrossNamespaceOptInRequired condition in the supplied conditions slice
// using a lookup-or-create pattern (avoids duplicate conditions on repeated
// reconciles).
//
// Returns the (possibly modified) conditions slice. Callers must assign the
// result back to the resource's Status.Conditions field.
func SetCrossNamespaceOptInRequired(
	conditions []*ackv1alpha1.Condition,
	message string,
) []*ackv1alpha1.Condition {
	for i, c := range conditions {
		if c.Type == ackv1alpha1.ConditionTypeCrossNamespaceOptInRequired {
			conditions[i].Status = corev1.ConditionTrue
			conditions[i].Message = &message
			return conditions
		}
	}
	return append(conditions, &ackv1alpha1.Condition{
		Type:    ackv1alpha1.ConditionTypeCrossNamespaceOptInRequired,
		Status:  corev1.ConditionTrue,
		Message: &message,
	})
}

// HandleCrossNamespaceReference emits a Phase 1 deprecation warning log and
// sets the ACK.CrossNamespaceOptInRequired condition on the supplied
// conditions slice. It is intended to be called from generated code after
// ValidateCrossNamespaceReference reports isCrossNamespace=true.
//
// Returns the updated conditions slice. Callers must assign the result back
// to the resource's Status.Conditions field.
func HandleCrossNamespaceReference(
	ctx context.Context,
	conditions []*ackv1alpha1.Condition,
	refKind CrossNamespaceRefKind,
	ownerNamespace string,
	targetNamespace string,
	refName string,
) []*ackv1alpha1.Condition {
	ackrtlog.FromContext(ctx).Info(
		fmt.Sprintf(
			"cross-namespace %s detected; this behavior will be disabled by "+
				"default in a future release. Set --enable-cross-namespace to "+
				"preserve this behavior.",
			refKind,
		),
		"ownerNamespace", ownerNamespace,
		"targetNamespace", targetNamespace,
		"referenceName", refName,
	)
	message := fmt.Sprintf(
		"Cross-namespace %s detected: resource in namespace %q references "+
			"%q in namespace %q. Cross-namespace behavior will be disabled "+
			"by default in a future release. Set --enable-cross-namespace=true "+
			"to preserve this behavior.",
		refKind, ownerNamespace, refName, targetNamespace,
	)
	return SetCrossNamespaceOptInRequired(conditions, message)
}
