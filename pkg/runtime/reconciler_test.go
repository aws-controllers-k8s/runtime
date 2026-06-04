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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sobj "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlrt "sigs.k8s.io/controller-runtime"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	k8srtschemamocks "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime/schema"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	"github.com/aws-controllers-k8s/runtime/pkg/runtime/iamroleselector"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// isWithoutCancelContext checks if the context is a WithoutCancel context
// This provides more specific matching than mock.Anything
func isWithoutCancelContext(ctx interface{}) bool {
	ctxVal, ok := ctx.(context.Context)
	if !ok {
		return false
	}

	// Check the type name for WithoutCancel context
	typeName := fmt.Sprintf("%T", ctxVal)
	return strings.Contains(typeName, "withoutCancelCtx")
}

// withoutCancelContextMatcher returns a matcher for WithoutCancel contexts
var withoutCancelContextMatcher = mock.MatchedBy(isWithoutCancelContext)

func resourceMocks() (
	*ackmocks.AWSResource, // mocked resource
	*ctrlrtclientmock.Object, // mocked k8s controller-runtime RuntimeObject
	*k8sobj.Unstructured, // NON-mocked k8s apimachinery meta object
) {
	objKind := &k8srtschemamocks.ObjectKind{}
	objKind.On("GroupVersionKind").Return(
		k8srtschema.GroupVersionKind{
			Group:   "bookstore.services.k8s.aws",
			Kind:    "Book",
			Version: "v1alpha1",
		},
	)

	rtObj := &ctrlrtclientmock.Object{}
	rtObj.On("GetObjectKind").Return(objKind)
	rtObj.On("DeepCopyObject").Return(rtObj)

	metaObj := &k8sobj.Unstructured{}
	metaObj.SetAnnotations(map[string]string{})
	metaObj.SetNamespace("default")
	metaObj.SetName("mybook")
	metaObj.SetGeneration(int64(1))

	res := &ackmocks.AWSResource{}
	res.On("MetaObject").Return(metaObj)
	res.On("RuntimeObject").Return(rtObj)
	res.On("DeepCopy").Return(res)
	// DoNothing on SetStatus call.
	res.On("SetStatus", res).Return(func(res ackmocks.AWSResource) {})

	return res, rtObj, metaObj
}

func reconcilerMocks(
	rmf acktypes.AWSResourceManagerFactory,
) (
	acktypes.AWSResourceReconciler,
	*ctrlrtclientmock.Client,
	acktypes.ServiceControllerMetadata,
) {
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	cfg := ackcfg.Config{
		FeatureGates: featuregate.FeatureGates{
			featuregate.ReadOnlyResources: {Enabled: true},
			featuregate.ResourceAdoption:  {Enabled: true},
		},
		ResourceTagKeys: []string{},
	}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	scmd := acktypes.ServiceControllerMetadata{}
	sc.On("GetMetadata").Return(scmd)
	kc := &ctrlrtclientmock.Client{}

	return NewReconcilerWithClient(
		sc, kc, rmf, fakeLogger, cfg, metrics, ackrtcache.Caches{}, &iamroleselector.Cache{},
	), kc, scmd
}

func managedResourceManagerFactoryMocks(
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) (
	*ackmocks.AWSResourceManagerFactory,
	*ackmocks.AWSResourceDescriptor,
) {
	return managerFactoryMocks(desired, latest, true)
}

func managerFactoryMocks(
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
	isManaged bool,
) (
	*ackmocks.AWSResourceManagerFactory,
	*ackmocks.AWSResourceDescriptor,
) {
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		k8srtschema.GroupVersionKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "fakeBook",
		},
	)
	rd.On("EmptyRuntimeObject").Return(
		&fakeBook{},
	)
	rd.On("IsManaged", latest).Return(isManaged)

	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	reg := NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)
	return rmf, rd
}

func TestReconcilerCreate_BackoffRetries(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Times(2)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, ackerr.NotFound,
	).Once()
	rm.On("ReadOne", ctx, latest).Return(
		latest, ackerr.NotFound,
	).Times(4)
	rm.On("ReadOne", ctx, latest).Return(
		latest, nil,
	)
	rm.On("Create", ctx, desired).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	// Use specific matcher for WithoutCancel context instead of mock.Anything
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertNumberOfCalls(t, "ReadOne", 6)
}

type awsError struct {
	smithy.APIError
}

func (err awsError) Error() string {
	return "mock error"
}

func TestReconcilerCreate_UnmanageResourceOnAWSErrors(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	desired, desiredRTObj, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Times(2)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, ackerr.NotFound,
	).Once()
	rm.On("Create", ctx, desired).Return(
		latest, awsError{},
	)
	rm.On("IsSynced", ctx, latest).Return(false, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(false).Twice()
	rd.On("IsManaged", desired).Return(true)
	rd.On("MarkUnmanaged", desired)
	rd.On("MarkManaged", desired)
	rd.On("MarkUnmanaged", desired)
	rd.On("ResourceFromRuntimeObject", desiredRTObj).Return(desired)
	rd.On("Delta", desired, desired).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	// Use specific matcher for WithoutCancel context instead of mock.Anything
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	_, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	rm.AssertNumberOfCalls(t, "ReadOne", 1)
	rd.AssertCalled(t, "MarkUnmanaged", desired)
	rd.AssertCalled(t, "MarkManaged", desired)
}

func TestReconcilerReadOnlyResource(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("my-read-only-book-arn")

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationReadOnly: "true",
	})

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationReadOnly: "true",
		},
	})
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Times(2)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	).Once()
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertNumberOfCalls(t, "ReadOne", 1)
	rd.AssertCalled(t, "Delta", desired, latest)
	// Assert that the resource is not created or updated
	rm.AssertNotCalled(t, "Create", 0)
	rm.AssertNotCalled(t, "Update", 0)
}

func TestReconcilerAdoptResource(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	adoptionFieldsString := "{\"arn\": \"my-adopt-book-arn\"}"
	adoptionFields := map[string]string{
		"arn": "my-adopt-book-arn",
	}

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy: "adopt",
		ackv1alpha1.AnnotationAdoptionFields: adoptionFieldsString,
	})

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdoptionPolicy: "adopt",
			ackv1alpha1.AnnotationAdoptionFields: adoptionFieldsString,
		},
	})
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()
	desired.On("PopulateResourceFromAnnotation", adoptionFields).Return(nil)
	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Times(2)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	).Once()
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("IsManaged", desired).Return(false)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	rd.On("MarkAdopted", latest).Return()
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertNumberOfCalls(t, "ReadOne", 1)
	// Assert that the resource is not created or updated
	rm.AssertNotCalled(t, "Create", 0)
	rm.AssertNotCalled(t, "Update", 0)
	rm.AssertNotCalled(t, "Delta", 0)
}

func TestReconcilerAdopt_InvalidAdoptionPolicy_TerminalCondition(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy: "invalid-policy",
	})

	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		hasTerminal := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeTerminal {
				continue
			}
			hasTerminal = true
			assert.Equal(corev1.ConditionTrue, condition.Status)
			assert.Contains(*condition.Message, "unrecognized adoption policy invalid-policy")
		}
		assert.True(hasTerminal)
	}).Once()
	desired.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rmf, rd := managerFactoryMocks(desired, nil, false)
	rd.On("IsManaged", desired).Return(false)

	r, _, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	latest, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	assert.Equal(ackerr.Terminal, err)
	require.NotNil(latest)
	rm.AssertNotCalled(t, "ResolveReferences")
	rm.AssertNotCalled(t, "ReadOne")
	rm.AssertNotCalled(t, "Create")
	rm.AssertNotCalled(t, "Update")
}

func TestReconcilerAdopt_InvalidAdoptionFields_TerminalCondition(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy: "adopt",
		ackv1alpha1.AnnotationAdoptionFields: "not-valid-json",
	})

	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		hasTerminal := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeTerminal {
				continue
			}
			hasTerminal = true
			assert.Equal(corev1.ConditionTrue, condition.Status)
			assert.Contains(*condition.Message, "unmarshalling adoption-fields annotation")
		}
		assert.True(hasTerminal)
	}).Once()
	desired.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
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
	rm.AssertNotCalled(t, "Update")
}

func TestReconcilerAdoptOrCreateResource_Create(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	adoptionFieldsString := "{\"arn\": \"my-adopt-book-arn\"}"
	adoptionFields := map[string]string{
		"arn": "my-adopt-book-arn",
	}

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy: "adopt-or-create",
		ackv1alpha1.AnnotationAdoptionFields: adoptionFieldsString,
	})
	desired.On("PopulateResourceFromAnnotation", adoptionFields).Return(nil)

	ids := &ackmocks.AWSResourceIdentifiers{}

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdoptionPolicy: "adopt-or-create",
			ackv1alpha1.AnnotationAdoptionFields: adoptionFieldsString,
		},
	})
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Times(2)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		nil, ackerr.NotFound,
	).Once()
	rm.On("ReadOne", ctx, latest).Return(
		latest, nil,
	)
	rm.On("Create", ctx, desired).Return(
		latest, nil,
	).Once()
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("IsManaged", desired).Return(false).Once()
	rd.On("IsManaged", desired).Return(true)
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
	rm.AssertNumberOfCalls(t, "ReadOne", 2)
	rm.AssertNumberOfCalls(t, "Create", 1)
	// Assert that the resource is not created or updated
	rm.AssertNotCalled(t, "Update", 0)
	rm.AssertNotCalled(t, "Delta", 0)
}

func TestReconcilerAdoptOrCreateResource_Adopt(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()
	adoptionFieldsString := "{\"arn\": \"my-adopt-book-arn\"}"
	adoptionFields := map[string]string{
		"arn": "my-adopt-book-arn",
	}

	desired, _, metaObj := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy: "adopt-or-create",
		ackv1alpha1.AnnotationAdoptionFields: adoptionFieldsString,
	})

	ids := &ackmocks.AWSResourceIdentifiers{}
	delta := ackcompare.NewDelta()
	delta.Add("Spec", "val1", "val2")

	latest, latestRTObj, latestMetaObj := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		hasSynced := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeResourceSynced {
				continue
			}

			hasSynced = true
			assert.Equal(corev1.ConditionUnknown, condition.Status)
			assert.Equal(ackcondition.UnknownSyncedMessage, *condition.Message)
		}
		assert.True(hasSynced)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	latestMetaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy: "adopt-or-create",
		ackv1alpha1.AnnotationAdoptionFields: adoptionFieldsString,
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Times(2)
	desired.On("PopulateResourceFromAnnotation", adoptionFields).Return(nil)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	).Once()
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)

	rd.On("IsManaged", desired).Return(false).Once()
	rd.On("IsManaged", desired).Return(true)
	rd.On("MarkAdopted", latest).Return().Once()
	latestMetaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationAdoptionPolicy: "adopt-or-create",
		ackv1alpha1.AnnotationAdoptionFields: adoptionFieldsString,
		ackv1alpha1.AnnotationAdopted:        "true",
	})

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Status").Return(statusWriter)
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	statusWriter.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil).Once()
	_, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	rm.AssertNumberOfCalls(t, "ReadOne", 1)
	// Assert that the resource is not created or updated
	rm.AssertNumberOfCalls(t, "Create", 0)
}

func TestReconcilerCreate_UnManagedResource_CheckReferencesResolveOnce(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		assert.Equal(t, corev1.ConditionTrue, cond.Status)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Times(2)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, ackerr.NotFound,
	).Once()
	rm.On("ReadOne", ctx, latest).Return(
		latest, nil,
	)
	rm.On("Create", ctx, desired).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	// Mark the resource as NotManaged before the Create call
	rd.On("IsManaged", desired).Return(false).Once()
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// pointers returned from "client.MergeFrom" fails the equality check during
	// assertion even when parameters inside two objects are same.
	// hence we use mock.AnythingOfType parameter to assert patch call
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// NotFound error return from `AWSResourceManager.ReadOne()` that we end
	// up calling the AWSResourceManager.Create() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	// Make sure references are only resolved once for the resource creation.
	// Only before the ReadOne call do they need to be resolved, and then the
	// referenced values are cleared when calling patch so they aren't persisted to etcd.
	rm.AssertNumberOfCalls(t, "ResolveReferences", 1)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rm.AssertCalled(t, "Create", ctx, desired)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "IsSynced", ctx, latest)
	rm.AssertNumberOfCalls(t, "EnsureTags", 2)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerCreate_ManagedResource_CheckReferencesResolveOnce(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		assert.Equal(t, corev1.ConditionTrue, cond.Status)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Once()
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, ackerr.NotFound,
	).Once()
	rm.On("ReadOne", ctx, latest).Return(
		latest, nil,
	)
	rm.On("Create", ctx, desired).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// pointers returned from "client.MergeFrom" fails the equality check during
	// assertion even when parameters inside two objects are same.
	// hence we use mock.AnythingOfType parameter to assert patch call
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// NotFound error return from `AWSResourceManager.ReadOne()` that we end
	// up calling the AWSResourceManager.Create() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	// Make sure references are resolved once for the resource creation when
	// the resource is already managed
	rm.AssertNumberOfCalls(t, "ResolveReferences", 1)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rm.AssertCalled(t, "Create", ctx, desired)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "IsSynced", ctx, latest)
	rm.AssertNumberOfCalls(t, "EnsureTags", 1)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		assert.Equal(t, corev1.ConditionTrue, cond.Status)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	).Once()
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("FilterSystemTags", mock.Anything, []string{})
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// pointers returned from "client.MergeFrom" fails the equality check during
	// assertion even when parameters inside two objects are same.
	// hence we use mock.AnythingOfType parameter to assert patch call
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	// Assert that References are resolved only once during resource update
	rm.AssertNumberOfCalls(t, "ResolveReferences", 1)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "IsSynced", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_ResourceNotSynced(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		// Synced condition is false because rm.IsSynced() method returns
		// False
		assert.Equal(t, corev1.ConditionFalse, cond.Status)
		assert.Equal(t, ackcondition.NotSyncedMessage, *cond.Message)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(false, nil)

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// pointers returned from "client.MergeFrom" fails the equality check during
	// assertion even when parameters inside two objects are same.
	// hence we use mock.AnythingOfType parameter to assert patch call
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "IsSynced", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_NoDelta_ResourceNotSynced(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		// Synced condition is false because rm.IsSynced() method returns
		// False
		assert.Equal(t, corev1.ConditionFalse, cond.Status)
		assert.Equal(t, ackcondition.NotSyncedMessage, *cond.Message)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(false, nil)

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(delta)

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(delta)

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// pointers returned from "client.MergeFrom" fails the equality check during
	// assertion even when parameters inside two objects are same.
	// hence we use mock.AnythingOfType parameter to assert patch call
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	// Update is not called because there is no delta
	rm.AssertNotCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "IsSynced", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_NoDelta_ResourceSynced(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		// Synced condition is true because rm.IsSynced() method returns
		// True
		assert.Equal(t, corev1.ConditionTrue, cond.Status)
		assert.Equal(t, ackcondition.SyncedMessage, *cond.Message)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(true, nil)

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(delta)

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(delta)

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// pointers returned from "client.MergeFrom" fails the equality check during
	// assertion even when parameters inside two objects are same.
	// hence we use mock.AnythingOfType parameter to assert patch call
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	// Update is not called because there is no delta
	rm.AssertNotCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "IsSynced", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_IsSyncedError(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	syncedError := errors.New("rm.IsSynced failed")

	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		// Synced condition is false because rm.IsSynced() method returns
		// an error
		assert.Equal(t, corev1.ConditionFalse, cond.Status)
		assert.Equal(t, ackcondition.NotSyncedMessage, *cond.Message)
		assert.Equal(t, syncedError.Error(), *cond.Reason)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(
		true, syncedError)

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// pointers returned from "client.MergeFrom" fails the equality check during
	// assertion even when parameters inside two objects are same.
	// hence we use mock.AnythingOfType parameter to assert patch call
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "IsSynced", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_PatchMetadataAndSpec_DiffInMetadata(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, latestMetaObj := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	// Note the change in annotations
	latestMetaObj.SetAnnotations(map[string]string{"a": "b"})

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)
	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	kc.AssertCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	latest.AssertCalled(t, "DeepCopy")
	latest.AssertCalled(t, "SetStatus", latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_PatchMetadataAndSpec_DiffInSpec(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		hasSynced := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeResourceSynced {
				continue
			}

			hasSynced = true
			assert.Equal(corev1.ConditionTrue, condition.Status)
			assert.Equal(ackcondition.SyncedMessage, *condition.Message)
		}
		assert.True(hasSynced)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()
	// Note no change to metadata...

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(
		delta,
	)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)
	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	kc.AssertCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerHandleReconcilerError_PatchStatus_Latest(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, latestMetaObj := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	latestMetaObj.SetAnnotations(map[string]string{"a": "b"})

	rmf, _ := managedResourceManagerFactoryMocks(desired, latest)
	r, kc, _ := reconcilerMocks(rmf)

	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	_, err := r.HandleReconcileError(ctx, desired, latest, nil)
	require.Nil(err)
	statusWriter.AssertCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// The HandleReconcilerError function never updates spec or metadata, so
	// even though there is a change to the annotations we expect no call to
	// patch the spec/metadata...
	kc.AssertNotCalled(t, "Patch")
}

func TestReconcilerHandleReconcilerError_NoPatchStatus_NoLatest(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	rmf, _ := managedResourceManagerFactoryMocks(desired, nil)
	r, kc, _ := reconcilerMocks(rmf)

	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Status").Return(statusWriter)

	_, err := r.HandleReconcileError(ctx, desired, nil, nil)
	require.Nil(err)
	// If latest is nil, we should not call patch status...
	statusWriter.AssertNotCalled(t, "Patch")
	// The HandleReconcilerError function never updates spec or metadata, so
	// even though there is a change to the annotations we expect no call to
	// patch the spec/metadata...
	kc.AssertNotCalled(t, "Patch")
}

func TestReconcilerUpdate_ErrorInLateInitialization(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	requeueError := requeue.NeededAfter(errors.New("error from late initialization"), time.Duration(0)*time.Second)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		hasSynced := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeResourceSynced {
				continue
			}
			hasSynced = true
			// Even though mocked IsSynced method returns (true, nil),
			// the reconciler error from late initialization correctly causes
			// the ResourceSynced condition to be Unknown since the reconciler
			// error is not a Terminal error.
			assert.Equal(corev1.ConditionUnknown, condition.Status)
			assert.Equal(ackcondition.UnknownSyncedMessage, *condition.Message)
			assert.Equal(requeueError.Error(), *condition.Reason)
		}
		assert.True(hasSynced)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("IsManaged", desired).Return(true)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm.On("LateInitialize", ctx, latest).Return(latest, requeueError)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	// Assert the error from late initialization
	require.NotNil(err)
	assert.Equal(requeueError, err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	// No difference in desired, latest metadata and spec
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_ResourceNotManaged(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, _, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	terminalCondition := ackv1alpha1.Condition{
		Type:    ackv1alpha1.ConditionTypeTerminal,
		Status:  corev1.ConditionTrue,
		Reason:  &ackcondition.NotManagedReason,
		Message: &ackcondition.NotManagedMessage,
	}
	// Return empty conditions for first two times
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{}).Times(2)
	// Once the terminal condition is added, return terminal condition
	// These calls will be made from ensureConditions method, which sets
	// ACK.ResourceSynced condition correctly
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{&terminalCondition})

	// Verify once when the terminal condition is set
	latest.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return([]*ackv1alpha1.Condition{&terminalCondition}).Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		hasTerminal := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeTerminal {
				continue
			}

			hasTerminal = true
			assert.Equal(terminalCondition.Message, condition.Message)
			assert.Equal(terminalCondition.Reason, condition.Reason)
		}
		assert.True(hasTerminal)
	}).Once()

	// Verify again when ResourceSynced condition is set that both Terminal
	// and ResourceSynced condition are present
	latest.On("ReplaceConditions", mock.AnythingOfType("[]*v1alpha1.Condition")).Return([]*ackv1alpha1.Condition{&terminalCondition}).Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		hasTerminal := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeTerminal {
				continue
			}

			hasTerminal = true
			assert.Equal(terminalCondition.Message, condition.Message)
			assert.Equal(terminalCondition.Reason, condition.Reason)
		}
		assert.True(hasTerminal)

		hasSynced := false
		for _, condition := range conditions {
			if condition.Type != ackv1alpha1.ConditionTypeResourceSynced {
				continue
			}
			hasSynced = true
			// The terminal error from reconciler correctly causes
			// the ResourceSynced condition to be False
			assert.Equal(corev1.ConditionFalse, condition.Status)
			assert.Equal(ackcondition.NotSyncedMessage, *condition.Message)
		}
		assert.True(hasSynced)
	}).Once()
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()

	rm := &ackmocks.AWSResourceManager{}
	rmf, rd := managerFactoryMocks(desired, latest, false)

	r, _, scmd := reconcilerMocks(rmf)
	rd.On("IsManaged", desired).Return(false)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, false, nil,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("IsSynced", ctx, latest).Return(true, nil)

	_, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	assert.Equal(ackerr.Terminal, err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertNotCalled(t, "Delta", desired, latest)
	rm.AssertNotCalled(t, "Update", ctx, desired, latest, delta)
	rm.AssertNotCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_ResolveReferencesError(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	resolveReferenceError := errors.New("failed to resolve reference")

	// resourceReconciler.ensureConditions will ensure that if the resource
	// manager has not set any Conditions on the resource, that at least an
	// ACK.ResourceSynced condition with status Unknown will be set on the
	// resource.
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeReferencesResolved, cond.Type)
		// The non-terminal reconciler error causes the ReferencesResolved
		// condition to be Unknown
		assert.Equal(t, corev1.ConditionUnknown, cond.Status)
		assert.Equal(t, ackcondition.FailedReferenceResolutionMessage, *cond.Message)
		assert.Equal(t, resolveReferenceError.Error(), *cond.Reason)
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, true, resolveReferenceError,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertNotCalled(t, "ReadOne", ctx, desired)
	rd.AssertNotCalled(t, "Delta", desired, latest)
	rm.AssertNotCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertNotCalled(t, "LateInitialize", ctx, latest)
	rm.AssertNotCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcilerUpdate_EnsureControllerTagsError(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)

	ensureControllerTagsError := errors.New("failed to ensure controller tags")

	// resourceReconciler.ensureConditions will ensure that if the resource
	// manager has not set any Conditions on the resource, that at least an
	// ACK.ResourceSynced condition with status Unknown will be set on the
	// resource.
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		assert.Equal(t, 1, len(conditions))
		cond := conditions[0]
		assert.Equal(t, ackv1alpha1.ConditionTypeResourceSynced, cond.Type)
		// The non-terminal reconciler error causes the ResourceSynced
		// condition to be False
		assert.Equal(t, corev1.ConditionFalse, cond.Status)
		assert.Equal(t, ackcondition.NotSyncedMessage, *cond.Message)
		assert.Equal(t, ensureControllerTagsError.Error(), *cond.Reason)
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", desired).Return(desired)
	rm.On("ClearResolvedReferences", latest).Return(latest)
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf, rd := managedResourceManagerFactoryMocks(desired, latest)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rm.On("IsSynced", ctx, latest).Return(true, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(
		ensureControllerTagsError,
	)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	rm.AssertCalled(t, "ResolveReferences", ctx, nil, desired)
	rm.AssertNotCalled(t, "ReadOne", ctx, desired)
	rd.AssertNotCalled(t, "Delta", desired, latest)
	rm.AssertNotCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertNotCalled(t, "LateInitialize", ctx, latest)
	rm.AssertCalled(t, "EnsureTags", ctx, desired, scmd)
}

func TestReconcile_AccountDrifted(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	req := ctrlrt.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "production",
			Name:      "mybook",
		},
	}

	// Create resource with existing account
	existingAccount := ackv1alpha1.AWSAccountID("111111111111")

	desired, _, metaObj := resourceMocks()
	metaObj.SetNamespace("production")

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("Region").Return(nil)
	ids.On("OwnerAccountID").Return(&existingAccount)
	desired.On("Identifiers").Return(ids)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return()
	desired.On("IsBeingDeleted").Return(false)

	// Setup resource descriptor
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(schema.GroupVersionKind{
		Group:   "test.services.k8s.aws",
		Kind:    "Book",
		Version: "v1alpha1",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)

	// Setup service controller
	sc := &ackmocks.ServiceController{}
	sc.On("GetMetadata").Return(acktypes.ServiceControllerMetadata{})
	sc.On("NewAWSConfig",
		mock.Anything,
		mock.AnythingOfType("v1alpha1.AWSRegion"),
		mock.Anything,
		mock.AnythingOfType("v1alpha1.AWSResourceName"),
		mock.AnythingOfType("schema.GroupVersionKind"),
		mock.Anything, // labels map[string]string
	).Return(aws.Config{}, nil)

	// Get fakeLogger
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))

	// Create fake k8s client with namespace that has owner account annotation
	k8sClient := k8sfake.NewSimpleClientset()

	// Create namespace with owner account annotation
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			Annotations: map[string]string{
				ackv1alpha1.AnnotationOwnerAccountID: "222222222222",
			},
		},
	}
	k8sClient.CoreV1().Namespaces().Create(context.Background(), namespace, metav1.CreateOptions{})

	// Create CARM configmap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ackrtcache.ACKRoleAccountMap,
			Namespace: "ack-system",
		},
		Data: map[string]string{
			"222222222222": "arn:aws:iam::222222222222:role/ACKRole",
		},
	}
	k8sClient.CoreV1().ConfigMaps("ack-system").Create(context.Background(), configMap, metav1.CreateOptions{})

	// Create caches with the k8s client
	caches := ackrtcache.New(fakeLogger, ackrtcache.Config{}, featuregate.FeatureGates{})
	irscaches := iamroleselector.NewCache(fakeLogger)

	// Run the caches
	stopCh := make(chan struct{})
	defer close(stopCh)
	caches.Run(k8sClient)

	// Wait for caches to sync
	time.Sleep(100 * time.Millisecond)

	kc := &ctrlrtclientmock.Client{}
	statusWriter := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	rm := &ackmocks.AWSResourceManager{}
	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("ManagerFor",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.AnythingOfType("v1alpha1.AWSAccountID"),
		mock.AnythingOfType("v1alpha1.AWSRegion"),
		mock.AnythingOfType("v1alpha1.AWSResourceName"),
	).Return(rm, nil)
	rm.On("ResolveReferences", mock.Anything, mock.Anything, mock.Anything).Return(
		desired, false, nil,
	)
	rm.On("EnsureTags", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	rm.On("FilterSystemTags", mock.Anything, []string{})

	// Create reconciler with namespace cache
	r := &resourceReconciler{
		reconciler: reconciler{
			kc:        kc,
			sc:        sc,
			log:       fakeLogger,
			cfg:       ackcfg.Config{AccountID: "333333333333"},
			carmCache: caches,
			irsCache:  irscaches,
			metrics:   ackmetrics.NewMetrics("test"),
		},
		rmf: rmf,
		rd:  rd,
	}

	apiReader := &ctrlrtclientmock.Reader{}
	apiReader.On("Get", ctx, req.NamespacedName, mock.AnythingOfType("*runtime.fakeBook")).Return(nil)
	r.apiReader = apiReader

	// Call Reconcile
	_, err := r.Reconcile(ctx, req)

	// Should get terminal error for account drift
	require.NotNil(err)
	assert.Contains(t, err.Error(), "Resource already exists in account 111111111111")
	assert.Contains(t, err.Error(), "but the role used for reconciliation is in account 222222222222")
}

// preDeleteDescriptorMock implements both AWSResourceDescriptor and
// AWSResourceDescriptorWithPreDeleteDelta so the reconciler's type
// assertion succeeds in pre-delete sync tests.
type preDeleteDescriptorMock struct {
	ackmocks.AWSResourceDescriptor
}

// DeltaForPreDelete satisfies the AWSResourceDescriptorWithPreDeleteDelta
// interface. It delegates to the testify mock machinery.
func (d *preDeleteDescriptorMock) DeltaForPreDelete(a, b acktypes.AWSResource) (*ackcompare.Delta, acktypes.AWSResource) {
	ret := d.Called(a, b)
	return ret.Get(0).(*ackcompare.Delta), ret.Get(1).(acktypes.AWSResource)
}

// newPreDeleteReconciler builds a resourceReconciler wired to the given
// descriptor for pre-delete sync tests.
func newPreDeleteReconciler(
	rd acktypes.AWSResourceDescriptor,
) (*resourceReconciler, *ctrlrtclientmock.Client) {
	zapOpts := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	logger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOpts))
	cfg := ackcfg.Config{
		DeletionPolicy: ackv1alpha1.DeletionPolicyDelete,
		FeatureGates: featuregate.FeatureGates{
			featuregate.ReadOnlyResources: {Enabled: true},
			featuregate.ResourceAdoption:  {Enabled: true},
		},
	}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	sc.On("GetMetadata").Return(acktypes.ServiceControllerMetadata{})

	kc := &ctrlrtclientmock.Client{}

	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	rec := &resourceReconciler{
		reconciler: reconciler{
			sc:        sc,
			kc:        kc,
			log:       logger.WithName("ackrt"),
			cfg:       cfg,
			metrics:   metrics,
			carmCache: ackrtcache.Caches{},
			irsCache:  &iamroleselector.Cache{},
		},
		rmf: rmf,
		rd:  rd,
	}
	return rec, kc
}

// newMockRes creates a mock AWSResource with standard bookstore metadata
// and the deletion-policy annotation set to "delete".
func newMockRes() (*ackmocks.AWSResource, *k8sobj.Unstructured) {
	objKind := &k8srtschemamocks.ObjectKind{}
	objKind.On("GroupVersionKind").Return(
		k8srtschema.GroupVersionKind{
			Group:   "bookstore.services.k8s.aws",
			Kind:    "Book",
			Version: "v1alpha1",
		},
	)
	rtObj := &ctrlrtclientmock.Object{}
	rtObj.On("GetObjectKind").Return(objKind)
	rtObj.On("DeepCopyObject").Return(rtObj)

	metaObj := &k8sobj.Unstructured{}
	metaObj.SetAnnotations(map[string]string{
		ackv1alpha1.AnnotationDeletionPolicy: string(ackv1alpha1.DeletionPolicyDelete),
	})
	metaObj.SetNamespace("default")
	metaObj.SetName("mybook")
	metaObj.SetGeneration(int64(1))

	res := &ackmocks.AWSResource{}
	res.On("MetaObject").Return(metaObj)
	res.On("RuntimeObject").Return(rtObj)
	res.On("DeepCopy").Return(res)
	res.On("SetStatus", mock.Anything).Return()
	return res, metaObj
}

// TestPreDeleteSync_ReadOneNotFound verifies that when ReadOne returns
// NotFound during deletion, the finalizer is removed and neither Update
// nor Delete is called.
func TestPreDeleteSync_ReadOneNotFound(t *testing.T) {
	require := require.New(t)

	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "bookstore.services.k8s.aws", Kind: "Book",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	desired, _ := newMockRes()
	desired.On("IsBeingDeleted").Return(true)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()

	rd.On("IsManaged", desired).Return(true)
	rd.On("MarkUnmanaged", desired).Return()
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)
	// Delta is called by patchResourceMetadataAndSpec inside setResourceUnmanaged
	noDelta := ackcompare.NewDelta()
	rd.On("Delta", mock.Anything, mock.Anything).Return(noDelta)

	rec, kc := newPreDeleteReconciler(rd)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", mock.Anything, desired).Return(nil, ackerr.NotFound)
	rm.On("ResolveReferences", mock.Anything, mock.Anything, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", mock.Anything).Return(desired)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)

	// Patch for setResourceUnmanaged
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	_, err := rec.reconcile(ctx, rm, desired)
	require.NoError(err)

	// Update and Delete should never be called
	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	rm.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

// TestPreDeleteSync_ReadOneError verifies that when ReadOne returns a
// non-NotFound error, the error is returned and neither Update nor Delete
// is called.
func TestPreDeleteSync_ReadOneError(t *testing.T) {
	assert := assert.New(t)

	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "bookstore.services.k8s.aws", Kind: "Book",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	desired, _ := newMockRes()
	desired.On("IsBeingDeleted").Return(true)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()

	rd.On("IsManaged", desired).Return(true)

	rec, _ := newPreDeleteReconciler(rd)

	readOneErr := errors.New("DescribeCluster API throttled")
	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", mock.Anything, desired).Return(nil, readOneErr)
	rm.On("ResolveReferences", mock.Anything, mock.Anything, desired).Return(desired, false, nil)

	ctx := context.Background()
	_, err := rec.reconcile(ctx, rm, desired)
	assert.Error(err)
	assert.Equal(readOneErr, err)

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	rm.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

// TestPreDeleteSync_DescriptorImplementsDeltaForPreDelete verifies that when
// the resource descriptor implements AWSResourceDescriptorWithPreDeleteDelta,
// the DeltaForPreDelete method is used (not the standard Delta).
func TestPreDeleteSync_DescriptorImplementsDeltaForPreDelete(t *testing.T) {
	require := require.New(t)

	rd := &preDeleteDescriptorMock{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "bookstore.services.k8s.aws", Kind: "Book",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	desired, _ := newMockRes()
	desired.On("IsBeingDeleted").Return(true)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()

	observed, _ := newMockRes()

	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("MarkUnmanaged", mock.Anything).Return()
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)
	// Delta is called by patchResourceMetadataAndSpec (in setResourceUnmanaged
	// and in the post-delete patch path). We set up a no-diff delta.
	noDelta := ackcompare.NewDelta()
	rd.On("Delta", mock.Anything, mock.Anything).Return(noDelta)

	// DeltaForPreDelete returns a delta with a spec difference and a merged resource
	preDeleteDelta := ackcompare.NewDelta()
	preDeleteDelta.Add("Spec.DeletionProtectionEnabled", true, false)
	merged, _ := newMockRes()
	rd.On("DeltaForPreDelete", desired, observed).Return(preDeleteDelta, merged)

	rec, kc := newPreDeleteReconciler(rd)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", mock.Anything, desired).Return(observed, nil)
	rm.On("ResolveReferences", mock.Anything, mock.Anything, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", mock.Anything).Return(desired)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)

	// Update succeeds — receives the merged resource (observed + pre-delete fields from desired)
	updated, _ := newMockRes()
	rm.On("Update", mock.Anything, merged, observed, preDeleteDelta).Return(updated, nil)

	// Delete succeeds — receives the updated resource
	rm.On("Delete", mock.Anything, updated).Return(updated, nil)

	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	_, err := rec.reconcile(ctx, rm, desired)
	require.NoError(err)

	// DeltaForPreDelete should have been called
	rd.AssertCalled(t, "DeltaForPreDelete", desired, observed)
	rm.AssertCalled(t, "Update", mock.Anything, merged, observed, preDeleteDelta)
	rm.AssertCalled(t, "Delete", mock.Anything, updated)
}

// TestPreDeleteSync_DescriptorDoesNotImplementDeltaForPreDelete verifies that
// when the resource descriptor does NOT implement
// AWSResourceDescriptorWithPreDeleteDelta, preDeleteSync returns observed
// directly (no-op) and deletion proceeds.
func TestPreDeleteSync_DescriptorDoesNotImplementDeltaForPreDelete(t *testing.T) {
	require := require.New(t)

	// Use the plain AWSResourceDescriptor mock — no DeltaForPreDelete method.
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "bookstore.services.k8s.aws", Kind: "Book",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	desired, _ := newMockRes()
	desired.On("IsBeingDeleted").Return(true)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()

	observed, _ := newMockRes()

	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("MarkUnmanaged", mock.Anything).Return()
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)
	// Delta is called by patchResourceMetadataAndSpec — no-diff
	noDelta := ackcompare.NewDelta()
	rd.On("Delta", mock.Anything, mock.Anything).Return(noDelta)

	rec, kc := newPreDeleteReconciler(rd)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", mock.Anything, desired).Return(observed, nil)
	rm.On("ResolveReferences", mock.Anything, mock.Anything, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", mock.Anything).Return(desired)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)

	// Delete succeeds — preDeleteSync is a no-op, so Delete receives observed
	rm.On("Delete", mock.Anything, observed).Return(observed, nil)

	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	_, err := rec.reconcile(ctx, rm, desired)
	require.NoError(err)

	// Update should NOT be called — preDeleteSync short-circuits
	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	rm.AssertCalled(t, "Delete", mock.Anything, observed)
}

// TestPreDeleteSync_DeletionProtectionScenario is an end-to-end scenario:
// desired has DeletionProtectionEnabled=false, observed has true.
// Verifies Update is called (to disable protection) then Delete is called
// with the synced resource.
func TestPreDeleteSync_DeletionProtectionScenario(t *testing.T) {
	require := require.New(t)

	rd := &preDeleteDescriptorMock{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "dsql.services.k8s.aws", Kind: "Cluster",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	desired, _ := newMockRes()
	desired.On("IsBeingDeleted").Return(true)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()

	observed, _ := newMockRes()

	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("MarkUnmanaged", mock.Anything).Return()
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)
	noDelta := ackcompare.NewDelta()
	rd.On("Delta", mock.Anything, mock.Anything).Return(noDelta)

	// DeltaForPreDelete detects the DeletionProtectionEnabled difference
	preDeleteDelta := ackcompare.NewDelta()
	preDeleteDelta.Add("Spec.DeletionProtectionEnabled", true, false)
	merged, _ := newMockRes()
	rd.On("DeltaForPreDelete", desired, observed).Return(preDeleteDelta, merged)

	rec, kc := newPreDeleteReconciler(rd)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", mock.Anything, desired).Return(observed, nil)
	rm.On("ResolveReferences", mock.Anything, mock.Anything, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", mock.Anything).Return(desired)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)

	// Update succeeds — protection is now disabled on AWS side
	synced, _ := newMockRes()
	rm.On("Update", mock.Anything, merged, observed, preDeleteDelta).Return(synced, nil)

	// Delete succeeds with the synced resource
	rm.On("Delete", mock.Anything, synced).Return(synced, nil)

	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	_, err := rec.reconcile(ctx, rm, desired)
	require.NoError(err)

	// Verify the call order: Update before Delete
	rm.AssertCalled(t, "Update", mock.Anything, merged, observed, preDeleteDelta)
	rm.AssertCalled(t, "Delete", mock.Anything, synced)
}

// TestPreDeleteSync_UpdateAndDeleteBothFail verifies that when the
// pre-delete sync update fails, deletion is NOT attempted and the update
// error is returned directly for requeue.
func TestPreDeleteSync_UpdateAndDeleteBothFail(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	rd := &preDeleteDescriptorMock{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "dsql.services.k8s.aws", Kind: "Cluster",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	desired, _ := newMockRes()
	desired.On("IsBeingDeleted").Return(true)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()

	observed, _ := newMockRes()

	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)
	noDelta := ackcompare.NewDelta()
	rd.On("Delta", mock.Anything, mock.Anything).Return(noDelta)

	// DeltaForPreDelete detects a difference
	preDeleteDelta := ackcompare.NewDelta()
	preDeleteDelta.Add("Spec.DeletionProtectionEnabled", true, false)
	merged, _ := newMockRes()
	rd.On("DeltaForPreDelete", desired, observed).Return(preDeleteDelta, merged)

	rec, kc := newPreDeleteReconciler(rd)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", mock.Anything, desired).Return(observed, nil)
	rm.On("ResolveReferences", mock.Anything, mock.Anything, desired).Return(desired, false, nil)
	rm.On("ClearResolvedReferences", mock.Anything).Return(desired)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)

	// Update fails
	updateErr := fmt.Errorf("AccessDeniedException: not authorized")
	rm.On("Update", mock.Anything, merged, observed, preDeleteDelta).Return(nil, updateErr)

	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	_, err := rec.reconcile(ctx, rm, desired)
	require.Error(err)

	// Error should be the update error
	assert.Contains(err.Error(), "AccessDeniedException")

	// Delete should NOT have been called — fail fast on pre-delete sync error
	rm.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestReconcilerSync_CrossNamespaceReferenceRejected(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	ctx := context.TODO()

	desired, _, _ := resourceMocks()
	desired.On("ReplaceConditions", []*ackv1alpha1.Condition{}).Return()

	crossNsErr := ackerr.ResourceReferenceCrossNamespaceNotAllowedFor(
		"default", "other-ns", "my-ref",
	)

	// When ResolveReferences returns a cross-namespace error, the reconciler
	// should set ReferencesResolved=False on the resource and return the
	// error without calling ReadOne.
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On(
		"ReplaceConditions",
		mock.AnythingOfType("[]*v1alpha1.Condition"),
	).Return().Run(func(args mock.Arguments) {
		conditions := args.Get(0).([]*ackv1alpha1.Condition)
		// Find the ReferencesResolved condition
		var refCond *ackv1alpha1.Condition
		for _, c := range conditions {
			if c.Type == ackv1alpha1.ConditionTypeReferencesResolved {
				refCond = c
				break
			}
		}
		require.NotNil(refCond, "expected ReferencesResolved condition to be set")
		assert.Equal(corev1.ConditionFalse, refCond.Status,
			"ReferencesResolved should be False for cross-namespace rejection")
		assert.Equal(ackcondition.FailedReferenceResolutionMessage, *refCond.Message)
		assert.Contains(*refCond.Reason, "cross-namespace resource reference is not allowed")
		assert.Contains(*refCond.Reason, "default")
		assert.Contains(*refCond.Reason, "other-ns")
		assert.Contains(*refCond.Reason, "my-ref")
	})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", ctx, nil, desired).Return(
		desired, true, crossNsErr,
	)
	rm.On("ClearResolvedReferences", desired).Return(desired)

	latest, latestRTObj, _ := resourceMocks()
	rm.On("ClearResolvedReferences", latest).Return(latest)
	// ReadOne should NOT be called — set it up but assert it's never invoked
	rm.On("ReadOne", ctx, desired).Return(latest, nil)

	rmf, _ := managedResourceManagerFactoryMocks(desired, latest)

	r, kc, scmd := reconcilerMocks(rmf)
	rm.On("EnsureTags", ctx, desired, scmd).Return(nil)
	kc.On("Patch", withoutCancelContextMatcher, latestRTObj, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	require.NotNil(err)
	assert.True(errors.Is(err, ackerr.ResourceReferenceCrossNamespaceNotAllowed),
		"error should wrap ResourceReferenceCrossNamespaceNotAllowed sentinel")

	// ReadOne must NOT be invoked when references fail to resolve
	rm.AssertNotCalled(t, "ReadOne", ctx, desired)
	// No downstream operations should occur
	rm.AssertNotCalled(t, "Create", ctx, desired)
	rm.AssertNotCalled(t, "Update")
	rm.AssertNotCalled(t, "LateInitialize")
	rm.AssertNotCalled(t, "EnsureTags", ctx, desired, scmd)
}

// secretReconciler builds a minimal reconciler with the supplied
// cross-namespace flag and a mocked apiReader for exercising
// SecretValueFromReference.
func secretReconciler(
	enableCrossNamespace bool,
) (*reconciler, *ctrlrtclientmock.Reader) {
	zapOptions := ctrlrtzap.Options{Development: true, Level: zapcore.InfoLevel}
	logger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	apiReader := &ctrlrtclientmock.Reader{}
	r := &reconciler{
		apiReader: apiReader,
		log:       logger,
		cfg: ackcfg.Config{
			EnableCrossNamespace: enableCrossNamespace,
		},
	}
	return r, apiReader
}

// expectSecretGet wires the mocked apiReader to return an Opaque secret with
// the supplied key/value for a Get against the given namespace/name.
func expectSecretGet(
	apiReader *ctrlrtclientmock.Reader,
	namespace, name, key, value string,
) {
	apiReader.On(
		"Get", mock.Anything,
		types.NamespacedName{Namespace: namespace, Name: name},
		mock.AnythingOfType("*v1.Secret"),
	).Run(func(args mock.Arguments) {
		secret := args.Get(2).(*corev1.Secret)
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{key: []byte(value)}
	}).Return(nil)
}

func ctxWithNamespace(ns string) context.Context {
	return context.WithValue(context.Background(), "resourceNamespace", ns)
}

func newSecretRef(namespace, name, key string) *ackv1alpha1.SecretKeyReference {
	ref := &ackv1alpha1.SecretKeyReference{Key: key}
	ref.Namespace = namespace
	ref.Name = name
	return ref
}

// fakeConditionManager is a minimal in-memory acktypes.ConditionManager used
// to assert that SecretValueFromReference sets conditions when one is stashed
// in the context.
type fakeConditionManager struct {
	conditions []*ackv1alpha1.Condition
}

func (f *fakeConditionManager) Conditions() []*ackv1alpha1.Condition {
	return f.conditions
}

func (f *fakeConditionManager) ReplaceConditions(conds []*ackv1alpha1.Condition) {
	f.conditions = conds
}

func TestSecretValueFromReference_SameNamespace(t *testing.T) {
	r, apiReader := secretReconciler(false)
	ctx := ctxWithNamespace("ns-a")
	expectSecretGet(apiReader, "ns-a", "sec", "pw", "value")

	// Empty namespace falls back to the owner namespace; allowed regardless
	// of the flag.
	val, err := r.SecretValueFromReference(ctx, newSecretRef("", "sec", "pw"))

	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestSecretValueFromReference_ExplicitSameNamespace(t *testing.T) {
	r, apiReader := secretReconciler(false)
	ctx := ctxWithNamespace("ns-a")
	expectSecretGet(apiReader, "ns-a", "sec", "pw", "value")

	val, err := r.SecretValueFromReference(ctx, newSecretRef("ns-a", "sec", "pw"))

	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestSecretValueFromReference_CrossNamespace_FlagDisabled(t *testing.T) {
	r, _ := secretReconciler(false)
	ctx := ctxWithNamespace("ns-a")

	val, err := r.SecretValueFromReference(ctx, newSecretRef("ns-b", "sec", "pw"))

	require.Error(t, err)
	assert.Empty(t, val)
	// The error must be terminal so the caller's condition machinery sets
	// ACK.Terminal, and must wrap the cross-namespace sentinel.
	var termErr *ackerr.TerminalError
	assert.True(t, errors.As(err, &termErr),
		"expected a terminal error, got %T", err)
	assert.True(t,
		errors.Is(err, ackerr.ResourceReferenceCrossNamespaceNotAllowed),
		"error should wrap the cross-namespace sentinel: %v", err)
}

func TestSecretValueFromReference_CrossNamespace_FlagEnabled(t *testing.T) {
	r, apiReader := secretReconciler(true)
	ctx := ctxWithNamespace("ns-a")
	// With the flag enabled the secret is fetched from the target namespace.
	expectSecretGet(apiReader, "ns-b", "sec", "pw", "value")

	val, err := r.SecretValueFromReference(ctx, newSecretRef("ns-b", "sec", "pw"))

	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestSecretValueFromReference_CrossNamespace_FlagEnabled_SetsCondition(t *testing.T) {
	r, apiReader := secretReconciler(true)
	cm := &fakeConditionManager{}
	ctx := WithConditionManager(ctxWithNamespace("ns-a"), cm)
	expectSecretGet(apiReader, "ns-b", "sec", "pw", "value")

	val, err := r.SecretValueFromReference(ctx, newSecretRef("ns-b", "sec", "pw"))

	require.NoError(t, err)
	assert.Equal(t, "value", val)
	// The deprecation condition should be set on the stashed resource.
	require.Len(t, cm.conditions, 1)
	assert.Equal(t,
		ackv1alpha1.ConditionTypeCrossNamespaceOptInRequired,
		cm.conditions[0].Type,
	)
	require.NotNil(t, cm.conditions[0].Message)
	assert.Contains(t, *cm.conditions[0].Message, "secret reference")
	assert.Contains(t, *cm.conditions[0].Message, "--enable-cross-namespace")
}

func TestSecretValueFromReference_SameNamespace_NoCondition(t *testing.T) {
	r, apiReader := secretReconciler(true)
	cm := &fakeConditionManager{}
	ctx := WithConditionManager(ctxWithNamespace("ns-a"), cm)
	expectSecretGet(apiReader, "ns-a", "sec", "pw", "value")

	_, err := r.SecretValueFromReference(ctx, newSecretRef("ns-a", "sec", "pw"))

	require.NoError(t, err)
	// Same-namespace refs must not set the deprecation condition.
	assert.Empty(t, cm.conditions)
}

func TestSecretValueFromReference_CrossNamespace_FlagEnabled_NoConditionManager(t *testing.T) {
	// When no ConditionManager is stashed in the context, resolution still
	// succeeds and does not panic.
	r, apiReader := secretReconciler(true)
	ctx := ctxWithNamespace("ns-a")
	expectSecretGet(apiReader, "ns-b", "sec", "pw", "value")

	val, err := r.SecretValueFromReference(ctx, newSecretRef("ns-b", "sec", "pw"))

	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestSecretValueFromReference_NilRef(t *testing.T) {
	r, _ := secretReconciler(false)
	val, err := r.SecretValueFromReference(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, val)
}
