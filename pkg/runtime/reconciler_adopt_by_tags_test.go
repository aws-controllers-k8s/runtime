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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
)

const testTagSelector = `{"Environment": "production"}`

var testTagFilters = map[string]string{"Environment": "production"}

// findCondition returns the first condition of the supplied type in the slice,
// or nil.
func findCondition(
	conds []*ackv1alpha1.Condition,
	condType ackv1alpha1.ConditionType,
) *ackv1alpha1.Condition {
	for _, c := range conds {
		if c.Type == condType {
			return c
		}
	}
	return nil
}

// TestReconcilerAdoptByTags_SingleMatch asserts that when the tag selector
// matches exactly one resource, its identifier is resolved from the ARN and the
// resource is adopted (marked managed+adopted, ReadOne called, patched).
func TestReconcilerAdoptByTags_SingleMatch(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := "arn:aws:eks:us-west-2:123456789012:nodegroup/my-cluster/ng-1/uuid"
	adoptionFields := map[string]string{"clusterName": "my-cluster", "name": "ng-1"}

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy:      "adopt",
		ackv1alpha1.AnnotationAdoptionTagSelector: testTagSelector,
	})
	desired.On("ResourceTypeFilter").Return("eks:nodegroup")
	desired.On("IdentifierFieldsFromARN", arn).Return(adoptionFields, nil)
	desired.On("PopulateResourceFromAnnotation", adoptionFields).Return(nil)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(metav1.ObjectMeta{})
	latest.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ResolveARNsByTags", ctx, testTagFilters, "eks:nodegroup").Return(
		[]string{arn}, nil,
	).Once()
	rm.On("ReadOne", ctx, desired).Return(latest, nil).Once()
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("IsManaged", desired).Return(false)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	rd.On("MarkAdopted", latest).Return()
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertNumberOfCalls(t, "ResolveARNsByTags", 1)
	rm.AssertNumberOfCalls(t, "ReadOne", 1)
	desired.AssertCalled(t, "IdentifierFieldsFromARN", arn)
	desired.AssertCalled(t, "PopulateResourceFromAnnotation", adoptionFields)
	rm.AssertNotCalled(t, "Create")
	rm.AssertNotCalled(t, "Update")
}

// TestReconcilerAdoptByTags_NoMatch_Adopt asserts that with the `adopt` policy
// and zero matches, the resource is set Recoverable and requeued, and NOT
// created.
func TestReconcilerAdoptByTags_NoMatch_Adopt(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy:      "adopt",
		ackv1alpha1.AnnotationAdoptionTagSelector: testTagSelector,
	})
	desired.On("ResourceTypeFilter").Return("eks:nodegroup")
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return().Run(func(args mock.Arguments) {
		conds := args.Get(0).([]*ackv1alpha1.Condition)
		if rec := findCondition(conds, ackv1alpha1.ConditionTypeRecoverable); rec != nil {
			assert.Equal(corev1.ConditionTrue, rec.Status)
			assert.Equal(ackcondition.NoResourcesMatchedReason, *rec.Reason)
		}
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ResolveARNsByTags", ctx, testTagFilters, "eks:nodegroup").Return(
		[]string{}, nil,
	).Once()
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(false, nil)
	rmf, rd := managerFactoryMocks(desired, nil, false)
	rd.On("IsManaged", desired).Return(false)

	r, _, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	latest, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	var requeueNeededAfter *requeue.RequeueNeededAfter
	require.ErrorAs(err, &requeueNeededAfter)
	require.NotNil(latest)
	rm.AssertCalled(t, "ResolveARNsByTags", ctx, testTagFilters, "eks:nodegroup")
	rm.AssertNotCalled(t, "ReadOne")
	rm.AssertNotCalled(t, "Create")
}

// TestReconcilerAdoptByTags_NoMatch_AdoptOrCreate asserts that with the
// `adopt-or-create` policy and zero matches, the resource falls through to the
// create path.
func TestReconcilerAdoptByTags_NoMatch_AdoptOrCreate(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy:      "adopt-or-create",
		ackv1alpha1.AnnotationAdoptionTagSelector: testTagSelector,
	})
	desired.On("ResourceTypeFilter").Return("eks:nodegroup")

	ids := &ackmocks.AWSResourceIdentifiers{}
	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(metav1.ObjectMeta{})
	latest.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ResolveARNsByTags", ctx, testTagFilters, "eks:nodegroup").Return(
		[]string{}, nil,
	).Once()
	// zero match under adopt-or-create -> ReadOne (NotFound) -> Create
	rm.On("ReadOne", ctx, desired).Return(nil, ackerr.NotFound).Once()
	rm.On("ReadOne", ctx, latest).Return(latest, nil)
	rm.On("Create", ctx, desired).Return(latest, nil).Once()
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("IsManaged", desired).Return(false).Once()
	rd.On("IsManaged", desired).Return(true)
	rd.On("MarkManaged", desired).Return()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Status").Return(statusWriter)
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	statusWriter.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "Create", ctx, desired)
	desired.AssertNotCalled(t, "IdentifierFieldsFromARN", mock.Anything)
}

// TestReconcilerAdoptByTags_MultipleMatches asserts that more than one match is
// a terminal condition.
func TestReconcilerAdoptByTags_MultipleMatches(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()
	arns := []string{
		"arn:aws:eks:us-west-2:123456789012:nodegroup/c/ng-1/uuid1",
		"arn:aws:eks:us-west-2:123456789012:nodegroup/c/ng-2/uuid2",
	}

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy:      "adopt",
		ackv1alpha1.AnnotationAdoptionTagSelector: testTagSelector,
	})
	desired.On("ResourceTypeFilter").Return("eks:nodegroup")
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return().Run(func(args mock.Arguments) {
		conds := args.Get(0).([]*ackv1alpha1.Condition)
		if term := findCondition(conds, ackv1alpha1.ConditionTypeTerminal); term != nil {
			assert.Equal(corev1.ConditionTrue, term.Status)
			assert.Equal(ackcondition.MultipleResourcesMatchedReason, *term.Reason)
			assert.Contains(*term.Message, "ng-1")
			assert.Contains(*term.Message, "ng-2")
		}
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ResolveARNsByTags", ctx, testTagFilters, "eks:nodegroup").Return(arns, nil).Once()
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(false, nil)
	rmf, rd := managerFactoryMocks(desired, nil, false)
	rd.On("IsManaged", desired).Return(false)

	r, _, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	latest, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	assert.Equal(ackerr.Terminal, err)
	require.NotNil(latest)
	rm.AssertNotCalled(t, "ReadOne")
	rm.AssertNotCalled(t, "Create")
}

// TestReconcilerAdoptByTags_UnsupportedKind asserts that a kind with an empty
// ResourceTypeFilter is a terminal condition and no lookup is attempted.
func TestReconcilerAdoptByTags_UnsupportedKind(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy:      "adopt",
		ackv1alpha1.AnnotationAdoptionTagSelector: testTagSelector,
	})
	desired.On("ResourceTypeFilter").Return("")
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return().Run(func(args mock.Arguments) {
		conds := args.Get(0).([]*ackv1alpha1.Condition)
		if term := findCondition(conds, ackv1alpha1.ConditionTypeTerminal); term != nil {
			assert.Equal(corev1.ConditionTrue, term.Status)
			assert.Contains(*term.Message, "does not support tag-based adoption")
		}
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(false, nil)
	rmf, rd := managerFactoryMocks(desired, nil, false)
	rd.On("IsManaged", desired).Return(false)

	r, _, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	latest, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	assert.Equal(ackerr.Terminal, err)
	require.NotNil(latest)
	rm.AssertNotCalled(t, "ResolveARNsByTags")
	rm.AssertNotCalled(t, "ReadOne")
	rm.AssertNotCalled(t, "Create")
}

// TestReconcilerAdoptByTags_GateDisabled asserts that using the
// adoption-tag-selector annotation while the AdoptResourcesByTags feature gate
// is disabled is a terminal condition (rather than silently falling back to the
// adoption-fields path or creating the resource).
func TestReconcilerAdoptByTags_GateDisabled(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy:      "adopt",
		ackv1alpha1.AnnotationAdoptionTagSelector: testTagSelector,
	})
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return().Run(func(args mock.Arguments) {
		conds := args.Get(0).([]*ackv1alpha1.Condition)
		if term := findCondition(conds, ackv1alpha1.ConditionTypeTerminal); term != nil {
			assert.Equal(corev1.ConditionTrue, term.Status)
			assert.Contains(*term.Message, "feature gate is not enabled")
		}
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(false, nil)
	rmf, rd := managerFactoryMocks(desired, nil, false)
	rd.On("IsManaged", desired).Return(false)

	// ResourceAdoption on, but AdoptResourcesByTags OFF.
	r, _, _ := reconcilerMocksWithGates(rmf, featuregate.FeatureGates{
		featuregate.ResourceAdoption:     {Enabled: true},
		featuregate.AdoptResourcesByTags: {Enabled: false},
	})

	latest, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	assert.Equal(ackerr.Terminal, err)
	require.NotNil(latest)
	rm.AssertNotCalled(t, "ResolveARNsByTags")
	rm.AssertNotCalled(t, "ReadOne")
	rm.AssertNotCalled(t, "Create")
}

// TestReconcilerAdoptByTags_NoPolicy asserts that setting the
// adoption-tag-selector annotation without an adoption-policy annotation is a
// terminal condition (we never default a policy, so a missing policy can never
// silently create infrastructure).
func TestReconcilerAdoptByTags_NoPolicy(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	// tag selector present, but NO adoption-policy annotation.
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionTagSelector: testTagSelector,
	})
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return().Run(func(args mock.Arguments) {
		conds := args.Get(0).([]*ackv1alpha1.Condition)
		if term := findCondition(conds, ackv1alpha1.ConditionTypeTerminal); term != nil {
			assert.Equal(corev1.ConditionTrue, term.Status)
			assert.Contains(*term.Message, "adoption-policy")
		}
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(false, nil)
	rmf, rd := managerFactoryMocks(desired, nil, false)
	rd.On("IsManaged", desired).Return(false)

	r, _, _ := reconcilerMocks(rmf)

	latest, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	assert.Equal(ackerr.Terminal, err)
	require.NotNil(latest)
	rm.AssertNotCalled(t, "ResolveARNsByTags")
	rm.AssertNotCalled(t, "ReadOne")
	rm.AssertNotCalled(t, "Create")
}
