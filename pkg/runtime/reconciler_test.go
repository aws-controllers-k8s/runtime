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
	"testing"

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
	ackrt "github.com/aws-controllers-k8s/runtime/pkg/runtime"

	k8srtmocks "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime"
	k8srtschemamocks "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime/schema"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
)

func TestReconcilerUpdate(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	objKind := &k8srtschemamocks.ObjectKind{}
	objKind.On("GroupVersionKind").Return(
		k8srtschema.GroupVersionKind{
			Group:   "bookstore.services.k8s.aws",
			Kind:    "Book",
			Version: "v1alpha1",
		},
	)

	desiredRTObj := &k8srtmocks.Object{}
	desiredRTObj.On("GetObjectKind").Return(objKind)

	desiredMetaObj := &k8sobj.Unstructured{}
	desiredMetaObj.SetAnnotations(map[string]string{})
	desiredMetaObj.SetNamespace("default")
	desiredMetaObj.SetName("mybook")
	desiredMetaObj.SetGeneration(int64(1))

	desired := &ackmocks.AWSResource{}
	desired.On("MetaObject").Return(desiredMetaObj)
	desired.On("RuntimeObject").Return(desiredRTObj)

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latestRTObj := &k8srtmocks.Object{}
	latestRTObj.On("GetObjectKind").Return(objKind)
	latestRTObj.On("DeepCopyObject").Return(latestRTObj)

	latestMetaObj := &k8sobj.Unstructured{}
	latestMetaObj.SetAnnotations(map[string]string{})
	latestMetaObj.SetNamespace("default")
	latestMetaObj.SetName("mybook")
	latestMetaObj.SetGeneration(int64(1))

	latest := &ackmocks.AWSResource{}
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(latestMetaObj)
	latest.On("RuntimeObject").Return(latestRTObj)

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
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())
	rd.On("UpdateCRStatus", latest).Return(true, nil)
	rd.On("IsManaged", desired).Return(true)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)

	reg := ackrt.NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)

	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	cfg := ackcfg.Config{}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	kc := &ctrlrtclientmock.Client{}
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)
	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	// TODO(jaypipes): Place the above setup into helper functions that can be
	// re-used by future unit tests of the reconciler code paths.

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	r := ackrt.NewReconcilerWithClient(sc, kc, rmf, fakeLogger, cfg, metrics)

	err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	kc.AssertNotCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	statusWriter.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
}

func TestReconcilerUpdate_PatchMetadataAndSpec_DiffInMetadata(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	objKind := &k8srtschemamocks.ObjectKind{}
	objKind.On("GroupVersionKind").Return(
		k8srtschema.GroupVersionKind{
			Group:   "bookstore.services.k8s.aws",
			Kind:    "Book",
			Version: "v1alpha1",
		},
	)

	desiredRTObj := &k8srtmocks.Object{}
	desiredRTObj.On("GetObjectKind").Return(objKind)

	desiredMetaObj := &k8sobj.Unstructured{}
	desiredMetaObj.SetAnnotations(map[string]string{})
	desiredMetaObj.SetNamespace("default")
	desiredMetaObj.SetName("mybook")
	desiredMetaObj.SetGeneration(int64(1))

	desired := &ackmocks.AWSResource{}
	desired.On("MetaObject").Return(desiredMetaObj)
	desired.On("RuntimeObject").Return(desiredRTObj)

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latestRTObj := &k8srtmocks.Object{}
	latestRTObj.On("GetObjectKind").Return(objKind)
	latestRTObj.On("DeepCopyObject").Return(latestRTObj)

	latestMetaObj := &k8sobj.Unstructured{}
	// Note the change in annotaions
	latestMetaObj.SetAnnotations(map[string]string{"a": "b"})
	latestMetaObj.SetNamespace("default")
	latestMetaObj.SetName("mybook")
	latestMetaObj.SetGeneration(int64(1))

	latest := &ackmocks.AWSResource{}
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(latestMetaObj)
	latest.On("RuntimeObject").Return(latestRTObj)

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
	rd.On("Delta", desired, latest).Return(
		delta,
	).Once()
	rd.On("Delta", desired, latest).Return(ackcompare.NewDelta())
	rd.On("UpdateCRStatus", latest).Return(true, nil)
	rd.On("IsManaged", desired).Return(true)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)

	reg := ackrt.NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)

	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	cfg := ackcfg.Config{}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	kc := &ctrlrtclientmock.Client{}
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)
	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	// TODO(jaypipes): Place the above setup into helper functions that can be
	// re-used by future unit tests of the reconciler code paths.

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	r := ackrt.NewReconcilerWithClient(sc, kc, rmf, fakeLogger, cfg, metrics)

	err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	kc.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	statusWriter.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
}

func TestReconcilerUpdate_PatchMetadataAndSpec_DiffInSpec(t *testing.T) {
	require := require.New(t)

	ctx := context.TODO()
	arn := ackv1alpha1.AWSResourceName("mybook-arn")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.A", "val1", "val2")

	objKind := &k8srtschemamocks.ObjectKind{}
	objKind.On("GroupVersionKind").Return(
		k8srtschema.GroupVersionKind{
			Group:   "bookstore.services.k8s.aws",
			Kind:    "Book",
			Version: "v1alpha1",
		},
	)

	desiredRTObj := &k8srtmocks.Object{}
	desiredRTObj.On("GetObjectKind").Return(objKind)

	desiredMetaObj := &k8sobj.Unstructured{}
	desiredMetaObj.SetAnnotations(map[string]string{})
	desiredMetaObj.SetNamespace("default")
	desiredMetaObj.SetName("mybook")
	desiredMetaObj.SetGeneration(int64(1))

	desired := &ackmocks.AWSResource{}
	desired.On("MetaObject").Return(desiredMetaObj)
	desired.On("RuntimeObject").Return(desiredRTObj)

	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("ARN").Return(&arn)

	latestRTObj := &k8srtmocks.Object{}
	latestRTObj.On("GetObjectKind").Return(objKind)
	latestRTObj.On("DeepCopyObject").Return(latestRTObj)

	latestMetaObj := &k8sobj.Unstructured{}
	// Note Similar metadata
	latestMetaObj.SetAnnotations(map[string]string{})
	latestMetaObj.SetNamespace("default")
	latestMetaObj.SetName("mybook")
	latestMetaObj.SetGeneration(int64(1))

	latest := &ackmocks.AWSResource{}
	latest.On("Identifiers").Return(ids)
	latest.On("Conditions").Return([]*ackv1alpha1.Condition{})
	latest.On("MetaObject").Return(latestMetaObj)
	latest.On("RuntimeObject").Return(latestRTObj)

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
	rd.On("Delta", desired, latest).Return(
		delta,
	)
	rd.On("UpdateCRStatus", latest).Return(true, nil)
	rd.On("IsManaged", desired).Return(true)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ReadOne", ctx, desired).Return(
		latest, nil,
	)
	rm.On("Update", ctx, desired, latest, delta).Return(
		latest, nil,
	)

	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)

	reg := ackrt.NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)

	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	cfg := ackcfg.Config{}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	kc := &ctrlrtclientmock.Client{}
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)
	kc.On("Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj)).Return(nil)

	// TODO(jaypipes): Place the above setup into helper functions that can be
	// re-used by future unit tests of the reconciler code paths.

	// With the above mocks and below assertions, we check that if we got a
	// non-error return from `AWSResourceManager.ReadOne()` and the
	// `AWSResourceDescriptor.Delta()` returned a non-empty Delta, that we end
	// up calling the AWSResourceManager.Update() call in the Reconciler.Sync()
	// method,
	r := ackrt.NewReconcilerWithClient(sc, kc, rmf, fakeLogger, cfg, metrics)

	err := r.Sync(ctx, rm, desired)
	require.Nil(err)
	rm.AssertCalled(t, "ReadOne", ctx, desired)
	rd.AssertCalled(t, "Delta", desired, latest)
	rm.AssertCalled(t, "Update", ctx, desired, latest, delta)
	kc.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
	statusWriter.AssertCalled(t, "Patch", ctx, latestRTObj, client.MergeFrom(desiredRTObj))
}
