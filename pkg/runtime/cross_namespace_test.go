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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

func TestSetCrossNamespaceOptInRequired_AppendsWhenAbsent(t *testing.T) {
	conds := []*ackv1alpha1.Condition{}
	out := SetCrossNamespaceOptInRequired(conds, "test message")

	require.Len(t, out, 1)
	assert.Equal(t, ackv1alpha1.ConditionTypeAdvisory, out[0].Type)
	require.NotNil(t, out[0].Reason)
	assert.Equal(t, CrossNamespaceOptInRequiredReason, *out[0].Reason)
	assert.Equal(t, corev1.ConditionTrue, out[0].Status)
	require.NotNil(t, out[0].Message)
	assert.Equal(t, "test message", *out[0].Message)
}

func TestSetCrossNamespaceOptInRequired_UpdatesWhenPresent(t *testing.T) {
	existingMsg := "old message"
	existingReason := CrossNamespaceOptInRequiredReason
	conds := []*ackv1alpha1.Condition{
		{
			Type:    ackv1alpha1.ConditionTypeAdvisory,
			Status:  corev1.ConditionFalse,
			Reason:  &existingReason,
			Message: &existingMsg,
		},
	}
	out := SetCrossNamespaceOptInRequired(conds, "new message")

	// Should still have exactly one condition, not two.
	require.Len(t, out, 1)
	assert.Equal(t, corev1.ConditionTrue, out[0].Status)
	require.NotNil(t, out[0].Message)
	assert.Equal(t, "new message", *out[0].Message)
}

func TestSetCrossNamespaceOptInRequired_PreservesOtherAdvisory(t *testing.T) {
	// An unrelated ACK.Advisory condition (different reason) must not be
	// clobbered by the cross-namespace advisory.
	otherMsg := "immutable field modified"
	otherReason := "ImmutableFieldModified"
	conds := []*ackv1alpha1.Condition{
		{
			Type:    ackv1alpha1.ConditionTypeAdvisory,
			Status:  corev1.ConditionTrue,
			Reason:  &otherReason,
			Message: &otherMsg,
		},
	}
	out := SetCrossNamespaceOptInRequired(conds, "cross-ns")

	require.Len(t, out, 2)
	assert.Equal(t, otherReason, *out[0].Reason)
	assert.Equal(t, CrossNamespaceOptInRequiredReason, *out[1].Reason)
}

func TestSetCrossNamespaceOptInRequired_PreservesOtherConditions(t *testing.T) {
	otherMsg := "other"
	conds := []*ackv1alpha1.Condition{
		{
			Type:    ackv1alpha1.ConditionTypeReferencesResolved,
			Status:  corev1.ConditionTrue,
			Message: &otherMsg,
		},
	}
	out := SetCrossNamespaceOptInRequired(conds, "cross-ns")

	require.Len(t, out, 2)
	assert.Equal(t, ackv1alpha1.ConditionTypeReferencesResolved, out[0].Type)
	assert.Equal(t, ackv1alpha1.ConditionTypeAdvisory, out[1].Type)
	require.NotNil(t, out[1].Reason)
	assert.Equal(t, CrossNamespaceOptInRequiredReason, *out[1].Reason)
}

func TestHandleCrossNamespaceReference_ResourceRef(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}

	out := HandleCrossNamespaceReference(
		ctx, conds,
		CrossNamespaceRefKindResource,
		"owner-ns", "target-ns", "my-ref",
	)

	require.Len(t, out, 1)
	require.NotNil(t, out[0].Message)
	msg := *out[0].Message
	// Message should mention all the relevant pieces.
	assert.True(t, strings.Contains(msg, "resource reference"),
		"message should mention ref kind: %s", msg)
	assert.True(t, strings.Contains(msg, "owner-ns"),
		"message should mention owner namespace: %s", msg)
	assert.True(t, strings.Contains(msg, "target-ns"),
		"message should mention target namespace: %s", msg)
	assert.True(t, strings.Contains(msg, "my-ref"),
		"message should mention ref name: %s", msg)
	assert.True(t, strings.Contains(msg, "--enable-cross-namespace"),
		"message should mention the flag: %s", msg)
}

func TestHandleCrossNamespaceReference_SecretRef(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}

	out := HandleCrossNamespaceReference(
		ctx, conds,
		CrossNamespaceRefKindSecret,
		"owner-ns", "secret-ns", "my-secret",
	)

	require.Len(t, out, 1)
	require.NotNil(t, out[0].Message)
	msg := *out[0].Message
	assert.True(t, strings.Contains(msg, "secret reference"),
		"message should mention ref kind: %s", msg)
}

func TestHandleCrossNamespaceReference_LookupOrCreate(t *testing.T) {
	ctx := context.Background()
	existing := "old"
	existingReason := CrossNamespaceOptInRequiredReason
	conds := []*ackv1alpha1.Condition{
		{
			Type:    ackv1alpha1.ConditionTypeAdvisory,
			Status:  corev1.ConditionFalse,
			Reason:  &existingReason,
			Message: &existing,
		},
	}

	out := HandleCrossNamespaceReference(
		ctx, conds,
		CrossNamespaceRefKindResource,
		"owner-ns", "target-ns", "my-ref",
	)

	// Repeated calls must not produce duplicate conditions.
	require.Len(t, out, 1)
	assert.Equal(t, corev1.ConditionTrue, out[0].Status)
}

func TestResolveCrossNamespaceReference_SameNamespace(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}
	owner := "ns-a"

	resolved, err := ResolveCrossNamespaceReference(
		ctx, true, &conds, CrossNamespaceRefKindResource,
		owner, &owner, "ref",
	)

	require.NoError(t, err)
	assert.Equal(t, owner, resolved)
	assert.Empty(t, conds, "no condition should be set for same-namespace refs")
}

func TestResolveCrossNamespaceReference_NilNamespace(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}

	resolved, err := ResolveCrossNamespaceReference(
		ctx, true, &conds, CrossNamespaceRefKindResource,
		"ns-a", nil, "ref",
	)

	require.NoError(t, err)
	assert.Equal(t, "ns-a", resolved)
	assert.Empty(t, conds)
}

func TestResolveCrossNamespaceReference_CrossNamespace_FlagEnabled(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}
	target := "ns-b"

	resolved, err := ResolveCrossNamespaceReference(
		ctx, true, &conds, CrossNamespaceRefKindResource,
		"ns-a", &target, "ref",
	)

	require.NoError(t, err)
	assert.Equal(t, target, resolved)
	require.Len(t, conds, 1)
	assert.Equal(t, ackv1alpha1.ConditionTypeAdvisory, conds[0].Type)
	require.NotNil(t, conds[0].Reason)
	assert.Equal(t, CrossNamespaceOptInRequiredReason, *conds[0].Reason)
}

func TestResolveCrossNamespaceReference_CrossNamespace_FlagDisabled(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}
	target := "ns-b"

	resolved, err := ResolveCrossNamespaceReference(
		ctx, false, &conds, CrossNamespaceRefKindResource,
		"ns-a", &target, "ref",
	)

	require.Error(t, err)
	assert.Empty(t, resolved)
	assert.Empty(t, conds, "no condition should be set when validation fails")
}

func TestResolveCrossNamespaceReference_NilConditionsPointer(t *testing.T) {
	ctx := context.Background()
	target := "ns-b"

	// Should not panic; cross-namespace flow still resolves the namespace.
	resolved, err := ResolveCrossNamespaceReference(
		ctx, true, nil, CrossNamespaceRefKindResource,
		"ns-a", &target, "ref",
	)

	require.NoError(t, err)
	assert.Equal(t, target, resolved)
}

func TestResolveCrossNamespaceReferenceString_CrossNamespace(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}

	resolved, err := ResolveCrossNamespaceReferenceString(
		ctx, true, &conds, CrossNamespaceRefKindSecret,
		"ns-a", "ns-b", "secret",
	)

	require.NoError(t, err)
	assert.Equal(t, "ns-b", resolved)
	require.Len(t, conds, 1)
	require.NotNil(t, conds[0].Message)
	assert.Contains(t, *conds[0].Message, "secret reference")
}

func TestResolveCrossNamespaceReferenceString_EmptyNamespace(t *testing.T) {
	ctx := context.Background()
	conds := []*ackv1alpha1.Condition{}

	resolved, err := ResolveCrossNamespaceReferenceString(
		ctx, true, &conds, CrossNamespaceRefKindSecret,
		"ns-a", "", "secret",
	)

	require.NoError(t, err)
	assert.Equal(t, "ns-a", resolved)
	assert.Empty(t, conds)
}

// subjectConditionManager is a minimal ConditionManager for exercising
// SetCrossNamespaceOptInRequiredOnSubject.
type subjectConditionManager struct {
	conditions []*ackv1alpha1.Condition
}

func (s *subjectConditionManager) Conditions() []*ackv1alpha1.Condition {
	return s.conditions
}

func (s *subjectConditionManager) ReplaceConditions(conds []*ackv1alpha1.Condition) {
	s.conditions = conds
}

func TestSetCrossNamespaceOptInRequiredOnSubject_SetsCondition(t *testing.T) {
	subject := &subjectConditionManager{}

	SetCrossNamespaceOptInRequiredOnSubject(subject, "test message")

	require.Len(t, subject.conditions, 1)
	assert.Equal(t, ackv1alpha1.ConditionTypeAdvisory, subject.conditions[0].Type)
	require.NotNil(t, subject.conditions[0].Reason)
	assert.Equal(t,
		CrossNamespaceOptInRequiredReason,
		*subject.conditions[0].Reason,
	)
	require.NotNil(t, subject.conditions[0].Message)
	assert.Equal(t, "test message", *subject.conditions[0].Message)
}

func TestSetCrossNamespaceOptInRequiredOnSubject_NilSubject(t *testing.T) {
	// Must not panic on a nil subject.
	SetCrossNamespaceOptInRequiredOnSubject(nil, "test message")
}

func TestSetCrossNamespaceOptInRequiredOnSubject_UpdatesInPlace(t *testing.T) {
	// Repeated calls update the same advisory rather than appending
	// duplicates.
	subject := &subjectConditionManager{}

	SetCrossNamespaceOptInRequiredOnSubject(subject, "first")
	SetCrossNamespaceOptInRequiredOnSubject(subject, "second")

	require.Len(t, subject.conditions, 1)
	require.NotNil(t, subject.conditions[0].Message)
	assert.Equal(t, "second", *subject.conditions[0].Message)
}

func TestConditionManagerFromContext_RoundTrip(t *testing.T) {
	subject := &subjectConditionManager{}
	ctx := WithConditionManager(context.Background(), subject)

	got := ConditionManagerFromContext(ctx)
	assert.Equal(t, subject, got)
}

func TestConditionManagerFromContext_Absent(t *testing.T) {
	assert.Nil(t, ConditionManagerFromContext(context.Background()))
}
