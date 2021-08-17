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

package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sobj "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrt "github.com/aws-controllers-k8s/runtime/pkg/runtime"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"

	k8srtmocks "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime"
	k8srtschemamocks "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime/schema"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
)

func resourceMocks() (
	*ackmocks.AWSResource, // mocked resource
	*k8srtmocks.Object, // mocked k8s controller-runtime RuntimeObject
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

	rtObj := &k8srtmocks.Object{}
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

	return res, rtObj, metaObj
}

func reconcilerMocks(
	rmf acktypes.AWSResourceManagerFactory,
) (
	acktypes.AWSResourceReconciler,
	*ctrlrtclientmock.Client,
) {
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	cfg := ackcfg.Config{}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	kc := &ctrlrtclientmock.Client{}

	return ackrt.NewReconcilerWithClient(
		sc, kc, rmf, fakeLogger, cfg, metrics, ackrtcache.Caches{},
	), kc
}

func managerFactoryMocks(
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
	delta *ackcompare.Delta,
) (
	*ackmocks.AWSResourceManagerFactory,
	*ackmocks.AWSResourceDescriptor,
) {
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupKind").Return(
		&metav1.GroupKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "fakeBook",
		},
	)
	rd.On("EmptyRuntimeObject").Return(
		&fakeBook{},
	)
	rd.On("IsManaged", latest).Return(true)

	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)

	reg := ackrt.NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)
	return rmf, rd
}

func TestReconcilerUpdate(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, desiredRTObj, _ := resourceMocks()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf, rd := managerFactoryMocks(desired, latest, delta)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc := reconcilerMocks(rmf)

	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	// No changes to metadata or spec so Patch on the object shouldn't be done
	kc.AssertNotCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
}

func TestReconcilerUpdate_PatchMetadataAndSpec_DiffInMetadata(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, desiredRTObj, _ := resourceMocks()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, latestMetaObj := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})

	// Note the change in annotations
	latestMetaObj.SetAnnotations(map[string]string{"a": "b"})

	rmf, rd := managerFactoryMocks(desired, latest, delta)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)
	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc := reconcilerMocks(rmf)

	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	kc.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
}

func TestReconcilerUpdate_PatchMetadataAndSpec_DiffInSpec(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, desiredRTObj, _ := resourceMocks()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	// Note no change to metadata...

	rmf, rd := managerFactoryMocks(desired, latest, delta)
	rd.On("Delta", desired, latest).Return(
		delta,
	)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)
	rm.On("LateInitialize", ctx, latest).Return(latest, nil)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc := reconcilerMocks(rmf)

	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	kc.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	// Only the HandleReconcilerError wrapper function ever calls patchResourceStatus
	kc.AssertNotCalled(t, "Status")
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
}

func TestReconcilerHandleReconcilerError_PatchStatus_Latest(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	desired, desiredRTObj, _ := resourceMocks()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, latestMetaObj := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})

	latestMetaObj.SetAnnotations(map[string]string{"a": "b"})

	rmf, _ := managerFactoryMocks(desired, latest, delta)
	r, kc := reconcilerMocks(rmf)

	statusWriter := &ctrlrtclientmock.StatusWriter{}
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)
	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	_, err := r.HandleReconcileError(ctx, desired, latest, nil)
	require.Nil(err)
	statusWriter.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	// The HandleReconcilerError function never updates spec or metadata, so
	// even though there is a change to the annotations we expect no call to
	// patch the spec/metadata...
	kc.AssertNotCalled(t, "Patch")
}

func TestReconcilerHandleReconcilerError_NoPatchStatus_NoLatest(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()

	desired, _, _ := resourceMocks()

	rmf, _ := managerFactoryMocks(desired, nil, nil)
	r, kc := reconcilerMocks(rmf)

	statusWriter := &ctrlrtclientmock.StatusWriter{}
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

	desired, desiredRTObj, _ := resourceMocks()

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latest, latestRTObj, _ := resourceMocks()
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf, rd := managerFactoryMocks(desired, latest, delta)
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())

	requeueError := requeue.NeededAfter(errors.New("error from late initialization"), time.Duration(0)*time.Second)
	rm.On("LateInitialize", ctx, latest).Return(latest, requeueError)
	rd.On("Delta", latest, latest).Return(ackcompare.NewDelta())

	r, kc := reconcilerMocks(rmf)

	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	_, err := r.Sync(ctx, rm, desired)
	// Assert the error from late initialization
	require.NotNil(err)
	assert.Equal(requeueError, err)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	// No difference in desired, latest metadata and spec
	kc.AssertNotCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	rm.AssertCalled(t, "LateInitialize", ctx, latest)
}
