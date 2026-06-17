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
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// CrossNamespaceOptInRequiredReason is the Reason carried by the ACK.Advisory
// condition that notifies users their cross-namespace usage will require
// explicit opt-in (via --enable-cross-namespace=true) in a future release.
//
// The cross-namespace deprecation notice is surfaced as a Reason on the
// existing ACK.Advisory condition rather than as a dedicated condition type.
// Advisory conditions are advisory-only (no programmatic dependencies are
// expected on them), which keeps this temporary notice cheap to add and to
// stop setting once the rollout completes, without the API-stability concerns
// of introducing and later removing a dedicated condition type. The reason is
// stable so support tooling can grep for it across resources.
const CrossNamespaceOptInRequiredReason = "CrossNamespaceOptInRequired"

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

// conditionManagerContextKey is the (unexported, collision-free) context key
// under which the reconciler stashes the resource being reconciled so that
// helpers without a direct resource handle (e.g. SecretValueFromReference)
// can set conditions on it.
//
// The resource is passed via the context rather than the method signature so
// that the SecretValueFromReference interface stays backward compatible: every
// caller (generated sdk.go, hooks, and hand-written custom update functions)
// is covered without a signature change that would force all controllers to be
// regenerated/updated.
type conditionManagerContextKey struct{}

// WithConditionManager returns a copy of ctx that carries the supplied
// ConditionManager (typically the resource being reconciled). It is used so
// that code paths which only receive a context can still set conditions on
// the resource.
func WithConditionManager(
	ctx context.Context,
	cm acktypes.ConditionManager,
) context.Context {
	return context.WithValue(ctx, conditionManagerContextKey{}, cm)
}

// ConditionManagerFromContext returns the ConditionManager previously stored
// with WithConditionManager, or nil if none is present.
func ConditionManagerFromContext(ctx context.Context) acktypes.ConditionManager {
	if v := ctx.Value(conditionManagerContextKey{}); v != nil {
		if cm, ok := v.(acktypes.ConditionManager); ok {
			return cm
		}
	}
	return nil
}

// SetCrossNamespaceOptInRequired sets or updates the cross-namespace
// deprecation notice in the supplied conditions slice. The notice is surfaced
// as an ACK.Advisory condition carrying the CrossNamespaceOptInRequiredReason
// reason. A lookup-or-create pattern keyed on (type, reason) avoids duplicate
// conditions on repeated reconciles while preserving any other ACK.Advisory
// conditions the resource may carry.
//
// Returns the (possibly modified) conditions slice. Callers must assign the
// result back to the resource's Status.Conditions field.
func SetCrossNamespaceOptInRequired(
	conditions []*ackv1alpha1.Condition,
	message string,
) []*ackv1alpha1.Condition {
	reason := CrossNamespaceOptInRequiredReason
	for i, c := range conditions {
		if c.Type == ackv1alpha1.ConditionTypeAdvisory &&
			c.Reason != nil && *c.Reason == reason {
			conditions[i].Status = corev1.ConditionTrue
			conditions[i].Message = &message
			return conditions
		}
	}
	return append(conditions, &ackv1alpha1.Condition{
		Type:    ackv1alpha1.ConditionTypeAdvisory,
		Status:  corev1.ConditionTrue,
		Reason:  &reason,
		Message: &message,
	})
}

// CrossNamespaceAdvisory returns the cross-namespace deprecation ACK.Advisory
// condition (Reason: CrossNamespaceOptInRequired) carried by subject, or nil if
// none is present. subject may be nil.
//
// This is used to reconcile the advisory from the resource stashed in the
// reconcile context onto the resource that is actually patched back to the
// Kubernetes API, in cases where deep copies inside Sync (e.g. adopt-or-create)
// detach the two objects.
func CrossNamespaceAdvisory(
	subject acktypes.ConditionManager,
) *ackv1alpha1.Condition {
	if subject == nil {
		return nil
	}
	for _, c := range subject.Conditions() {
		if c.Type == ackv1alpha1.ConditionTypeAdvisory &&
			c.Reason != nil && *c.Reason == CrossNamespaceOptInRequiredReason {
			return c
		}
	}
	return nil
}

// reconcileCrossNamespaceAdvisory copies the cross-namespace deprecation
// ACK.Advisory condition from src onto dst when present. It is used to carry
// the advisory from the resource stashed in the reconcile context onto the
// resource that is actually patched back to the Kubernetes API, in cases where
// deep copies inside Sync (e.g. adopt-or-create's resolved.DeepCopy()) detach
// the two objects.
//
// The copy is idempotent: SetCrossNamespaceOptInRequired is keyed on
// (type, reason), so if dst already carries the advisory it is updated in place
// rather than duplicated, and all other conditions on dst are preserved.
func reconcileCrossNamespaceAdvisory(src, dst acktypes.ConditionManager) {
	if src == nil || dst == nil {
		return
	}
	if adv := CrossNamespaceAdvisory(src); adv != nil && adv.Message != nil {
		dst.ReplaceConditions(
			SetCrossNamespaceOptInRequired(dst.Conditions(), *adv.Message),
		)
	}
}

// SetCrossNamespaceOptInRequiredOnSubject sets or updates the cross-namespace
// deprecation ACK.Advisory condition on the supplied ConditionManager
// (typically the resource being reconciled). It is a convenience wrapper for
// callers that hold a ConditionManager rather than a raw conditions slice.
func SetCrossNamespaceOptInRequiredOnSubject(
	subject acktypes.ConditionManager,
	message string,
) {
	if subject == nil {
		return
	}
	reason := CrossNamespaceOptInRequiredReason
	ackcondition.SetAdvisory(subject, corev1.ConditionTrue, &message, &reason)
}

// HandleCrossNamespaceReference emits a Phase 1 deprecation warning log and
// sets the cross-namespace deprecation ACK.Advisory condition on the supplied
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

// ResolveCrossNamespaceReference orchestrates the full Phase 1 cross-namespace
// reference handling flow in a single call. It calls
// ValidateCrossNamespaceReference and, when the reference targets a different
// namespace and the flag is enabled, calls HandleCrossNamespaceReference to
// emit the warning log and set the cross-namespace deprecation ACK.Advisory
// condition.
//
// Parameters:
//   - ctx: passed to the logger
//   - enableCrossNamespace: the value of Config.EnableCrossNamespace
//   - conditions: pointer to the resource's Status.Conditions slice; the
//     slice is updated in place when a deprecation condition is set
//   - refKind: label describing the reference kind (resource, secret, ...)
//   - ownerNamespace: the namespace of the resource containing the reference
//   - refNamespace: the user-supplied namespace (may be nil or empty)
//   - refName: the user-supplied reference name; used for log fields and
//     error message context
//
// Returns the resolved namespace to pass to apiReader.Get and any terminal
// error from ValidateCrossNamespaceReference. Callers do not need to inspect
// an isCrossNamespace flag or manage the conditions slice themselves.
func ResolveCrossNamespaceReference(
	ctx context.Context,
	enableCrossNamespace bool,
	conditions *[]*ackv1alpha1.Condition,
	refKind CrossNamespaceRefKind,
	ownerNamespace string,
	refNamespace *string,
	refName string,
) (string, error) {
	resolved, isCrossNs, err := ValidateCrossNamespaceReference(
		enableCrossNamespace, ownerNamespace, refNamespace, refName,
	)
	if err != nil {
		return "", err
	}
	if isCrossNs && conditions != nil {
		*conditions = HandleCrossNamespaceReference(
			ctx, *conditions, refKind, ownerNamespace, *refNamespace, refName,
		)
	}
	return resolved, nil
}

// ResolveCrossNamespaceReferenceString is the string-namespace counterpart of
// ResolveCrossNamespaceReference, intended for callers that have a plain
// string namespace (e.g. SecretKeyReference.Namespace,
// FieldExportTarget.Namespace) rather than a *string.
func ResolveCrossNamespaceReferenceString(
	ctx context.Context,
	enableCrossNamespace bool,
	conditions *[]*ackv1alpha1.Condition,
	refKind CrossNamespaceRefKind,
	ownerNamespace string,
	refNamespace string,
	refName string,
) (string, error) {
	resolved, isCrossNs, err := ValidateCrossNamespaceReferenceString(
		enableCrossNamespace, ownerNamespace, refNamespace, refName,
	)
	if err != nil {
		return "", err
	}
	if isCrossNs && conditions != nil {
		*conditions = HandleCrossNamespaceReference(
			ctx, *conditions, refKind, ownerNamespace, refNamespace, refName,
		)
	}
	return resolved, nil
}
