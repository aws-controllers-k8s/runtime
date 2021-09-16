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

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sobj "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	k8srtmocks "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackrt "github.com/aws-controllers-k8s/runtime/pkg/runtime"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

const (
	Namespace = "default"
	Name      = "adoptedRes"
)

// Helper functions for tests

func mockReconciler() (acktypes.AdoptedResourceReconciler, *ctrlrtclientmock.Client, *ctrlrtclientmock.Reader) {
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	cfg := ackcfg.Config{}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	rmfactory := ackmocks.AWSResourceManagerFactory{}
	rmFactoryMap := make(map[string]acktypes.AWSResourceManagerFactory)
	rmFactoryMap["services.k8s.aws"] = &rmfactory
	sc.On("GetResourceManagerFactories").Return(rmFactoryMap)
	kc := &ctrlrtclientmock.Client{}
	apiReader := &ctrlrtclientmock.Reader{}
	return ackrt.NewAdoptionReconcilerWithClient(
		sc,
		fakeLogger,
		cfg,
		metrics,
		ackrtcache.Caches{},
		kc,
		apiReader,
	), kc, apiReader
}

func mockDescriptorAndAWSResource() (*ackmocks.AWSResourceDescriptor, *ackmocks.AWSResource) {
	des := &ackmocks.AWSResourceDescriptor{}
	emptyRuntimeObject := &k8srtmocks.Object{}
	res := &ackmocks.AWSResource{}
	des.On("EmptyRuntimeObject").Return(emptyRuntimeObject)
	des.On("ResourceFromRuntimeObject", emptyRuntimeObject).Return(res)
	return des, res
}

func mockManager() *ackmocks.AWSResourceManager {
	return &ackmocks.AWSResourceManager{}
}

func setupMockClient(kc *ctrlrtclientmock.Client, statusWriter *ctrlrtclientmock.StatusWriter, ctx context.Context, adoptedRes *ackv1alpha1.AdoptedResource) {
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", ctx, adoptedRes, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Patch", ctx, adoptedRes, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
}

func setupMockAwsResource(res *ackmocks.AWSResource, adoptedRes *ackv1alpha1.AdoptedResource) {
	res.On("SetIdentifiers", adoptedRes.Spec.AWS).Return(nil)
	res.On("RuntimeObject").Return(&k8srtmocks.Object{})
	res.On("SetObjectMeta", mock.AnythingOfType("ObjectMeta")).Run(func(args mock.Arguments) {})

	metaObj := &k8sobj.Unstructured{}
	metaObj.SetNamespace(Namespace)
	metaObj.SetName(Name)
	res.On("MetaObject").Return(metaObj)

	rmo := &ackmocks.RuntimeMetaObject{}
	res.On("RuntimeMetaObject").Return(rmo)

	rmo.On("GetLabels").Return(make(map[string]string))
	rmo.On("GetAnnotations").Return(make(map[string]string))
	rmo.On("GetFinalizers").Return(make([]string, 0))
	rmo.On("GetOwnerReferences").Return(make([]v1.OwnerReference, 0))
	rmo.On("GetGenerateName").Return("")
}

func setupMockManager(manager *ackmocks.AWSResourceManager, ctx context.Context, res *ackmocks.AWSResource) {
	manager.On("ReadOne", ctx, res).Return(res, nil)
}

func setupMockDescriptor(descriptor *ackmocks.AWSResourceDescriptor, res *ackmocks.AWSResource) {
	descriptor.On("MarkManaged", res).Run(func(args mock.Arguments) {})
	descriptor.On("MarkAdopted", res).Run(func(args mock.Arguments) {})
}

func setupMockApiReader(apiReader *ctrlrtclientmock.Reader, ctx context.Context, res *ackmocks.AWSResource) {
	apiReader.On("Get", ctx, types.NamespacedName{
		Namespace: Namespace,
		Name:      Name,
	}, res.RuntimeObject()).Return(k8serrors.NewNotFound(schema.GroupResource{}, ""))
}

func adoptedResource(namespace, name string) *ackv1alpha1.AdoptedResource {
	return &ackv1alpha1.AdoptedResource{
		TypeMeta: v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: ackv1alpha1.AdoptedResourceSpec{
			Kubernetes: nil,
			AWS:        &ackv1alpha1.AWSIdentifiers{NameOrID: "name"},
		},
		Status: ackv1alpha1.AdoptedResourceStatus{},
	}
}

//Tests

func TestSync_FailureInSettingIdentifiers(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockReconciler()
	descriptor, res := mockDescriptorAndAWSResource()
	manager := mockManager()
	adoptedRes := adoptedResource(Namespace, Name)
	res.On("SetIdentifiers", adoptedRes.Spec.AWS).Return(errors.New("unable to set Identifier"))
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockAwsResource(res, adoptedRes)
	setupMockClient(kc, statusWriter, ctx, adoptedRes)

	// Call
	err := r.Sync(ctx, descriptor, manager, adoptedRes)

	// Assertions
	// error occured
	require.NotNil(err)
	require.Equal("unable to set Identifier", err.Error())
	// Attempt to set Identifiers from AdoptedResource into AWSResource
	res.AssertCalled(t, "SetIdentifiers", adoptedRes.Spec.AWS)
	// ReadOne call is not made to find observed state of AWSResource because
	// of SetIdentifiers failure
	manager.AssertNotCalled(t, "ReadOne", ctx, res)
	// No calls to findout if the AWSResource already exists
	apiReader.AssertNotCalled(t, "Get", ctx, types.NamespacedName{
		Namespace: Namespace,
		Name:      Name,
	}, res.RuntimeObject())
	assertAWSResourceCreation(false, t, ctx, kc, statusWriter, res)
	assertManaged(false, t, ctx, kc, adoptedRes)
	assertAdoptedCondition("False", require, t, ctx, kc, statusWriter, adoptedRes)
}

func TestSync_FailureInReadOne(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockReconciler()
	descriptor, res := mockDescriptorAndAWSResource()
	manager := mockManager()
	adoptedRes := adoptedResource(Namespace, Name)
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockAwsResource(res, adoptedRes)
	setupMockClient(kc, statusWriter, ctx, adoptedRes)
	manager.On("ReadOne", ctx, res).Return(res, errors.New("failed to perform ReadOne"))

	// Call
	err := r.Sync(ctx, descriptor, manager, adoptedRes)

	//Assertions
	require.NotNil(err)
	require.Equal("failed to perform ReadOne", err.Error())
	// Identifiers are set from AdoptedResource into AWSResource
	res.AssertCalled(t, "SetIdentifiers", adoptedRes.Spec.AWS)
	// ReadOne call is made to find observed state of AWSResource
	manager.AssertCalled(t, "ReadOne", ctx, res)
	// No calls to findout if the AWSResource already exists because of ReadOne
	// failure
	apiReader.AssertNotCalled(t, "Get", ctx, types.NamespacedName{
		Namespace: Namespace,
		Name:      Name,
	}, res.RuntimeObject())
	assertAWSResourceCreation(false, t, ctx, kc, statusWriter, res)
	assertManaged(false, t, ctx, kc, adoptedRes)
	assertAdoptedCondition("False", require, t, ctx, kc, statusWriter, adoptedRes)
}

func TestSync_AWSResourceAlreadyExists(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockReconciler()
	descriptor, res := mockDescriptorAndAWSResource()
	manager := mockManager()
	adoptedRes := adoptedResource(Namespace, Name)
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockAwsResource(res, adoptedRes)
	setupMockClient(kc, statusWriter, ctx, adoptedRes)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)

	apiReader.On("Get", ctx, types.NamespacedName{
		Namespace: Namespace,
		Name:      Name,
	}, res.RuntimeObject()).Return(nil)

	// Call
	err := r.Sync(ctx, descriptor, manager, adoptedRes)

	//Assertions
	require.Nil(err)
	assertAWSResourceRead(t, ctx, manager, apiReader, adoptedRes, res)
	assertAWSResourceCreation(false, t, ctx, kc, statusWriter, res)
	assertManaged(true, t, ctx, kc, adoptedRes)
	assertAdoptedCondition("True", require, t, ctx, kc, statusWriter, adoptedRes)
}

func TestSync_APIReaderUnknownError(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockReconciler()
	descriptor, res := mockDescriptorAndAWSResource()
	manager := mockManager()
	adoptedRes := adoptedResource(Namespace, Name)
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockAwsResource(res, adoptedRes)
	setupMockClient(kc, statusWriter, ctx, adoptedRes)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)

	apiReader.On("Get", ctx, types.NamespacedName{
		Namespace: Namespace,
		Name:      Name,
	}, res.RuntimeObject()).Return(errors.New("unknown error"))

	// Call
	err := r.Sync(ctx, descriptor, manager, adoptedRes)

	//Assertions
	require.NotNil(err)
	require.Equal("unknown error", err.Error())
	assertAWSResourceRead(t, ctx, manager, apiReader, adoptedRes, res)
	assertAWSResourceCreation(false, t, ctx, kc, statusWriter, res)
	assertManaged(false, t, ctx, kc, adoptedRes)
	assertAdoptedCondition("False", require, t, ctx, kc, statusWriter, adoptedRes)
}

func TestSync_ErrorInResourceCreation(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockReconciler()
	descriptor, res := mockDescriptorAndAWSResource()
	manager := mockManager()
	adoptedRes := adoptedResource(Namespace, Name)
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockAwsResource(res, adoptedRes)
	setupMockClient(kc, statusWriter, ctx, adoptedRes)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)
	setupMockApiReader(apiReader, ctx, res)
	kc.On("Create", ctx, res.RuntimeObject()).Return(errors.New("creation failure"))

	// Call
	err := r.Sync(ctx, descriptor, manager, adoptedRes)

	//Assertions
	require.NotNil(err)
	require.Equal("creation failure", err.Error())
	assertAWSResourceRead(t, ctx, manager, apiReader, adoptedRes, res)
	kc.AssertCalled(t, "Create", ctx, res.RuntimeObject())
	// Update status of AWSResource should not happen due to creation failure
	statusWriter.AssertNotCalled(t, "Update", ctx, res.RuntimeObject())
	assertManaged(false, t, ctx, kc, adoptedRes)
	assertAdoptedCondition("False", require, t, ctx, kc, statusWriter, adoptedRes)
}

func TestSync_ErrorInStatusUpdate(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockReconciler()
	descriptor, res := mockDescriptorAndAWSResource()
	manager := mockManager()
	adoptedRes := adoptedResource(Namespace, Name)
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockAwsResource(res, adoptedRes)
	setupMockClient(kc, statusWriter, ctx, adoptedRes)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)
	setupMockApiReader(apiReader, ctx, res)
	kc.On("Create", ctx, res.RuntimeObject()).Return(nil)
	statusWriter.On("Update", ctx, res.RuntimeObject()).Return(errors.New("status update failure"))

	// Call
	err := r.Sync(ctx, descriptor, manager, adoptedRes)

	//Assertions
	require.NotNil(err)
	require.Equal("status update failure", err.Error())
	assertAWSResourceRead(t, ctx, manager, apiReader, adoptedRes, res)
	assertAWSResourceCreation(true, t, ctx, kc, statusWriter, res)
	assertManaged(false, t, ctx, kc, adoptedRes)
	assertAdoptedCondition("False", require, t, ctx, kc, statusWriter, adoptedRes)
}

func TestSync_HappyCase(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockReconciler()
	descriptor, res := mockDescriptorAndAWSResource()
	manager := mockManager()
	adoptedRes := adoptedResource(Namespace, Name)
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockAwsResource(res, adoptedRes)
	setupMockClient(kc, statusWriter, ctx, adoptedRes)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)
	setupMockApiReader(apiReader, ctx, res)
	kc.On("Create", ctx, res.RuntimeObject()).Return(nil)
	statusWriter.On("Update", ctx, res.RuntimeObject()).Return(nil)

	// Call
	err := r.Sync(ctx, descriptor, manager, adoptedRes)

	//Assertions
	require.Nil(err)
	assertAWSResourceRead(t, ctx, manager, apiReader, adoptedRes, res)
	assertAWSResourceCreation(true, t, ctx, kc, statusWriter, res)
	assertManaged(true, t, ctx, kc, adoptedRes)
	assertAdoptedCondition("True", require, t, ctx, kc, statusWriter, adoptedRes)
}

// Assertion Helpers

// assertAdoptedCondition asserts that 'ConditionTypeAdopted' condition is
// present in AdoptedResource status and that it's value is equal to
// 'conditionStatus' parameter
func assertAdoptedCondition(
	conditionStatus string,
	require *require.Assertions,
	t *testing.T,
	ctx context.Context,
	kc *ctrlrtclientmock.Client,
	statusWriter *ctrlrtclientmock.StatusWriter,
	adoptedRes *ackv1alpha1.AdoptedResource,
) {
	kc.AssertCalled(t, "Status")
	statusWriter.AssertCalled(t, "Patch", ctx, adoptedRes, mock.AnythingOfType("*client.mergeFromPatch"))
	// Only one kind of condition present
	require.Equal(1, len(adoptedRes.Status.Conditions))
	require.Equal(ackv1alpha1.ConditionTypeAdopted, adoptedRes.Status.Conditions[0].Type)
	require.Equal(conditionStatus, string(adoptedRes.Status.Conditions[0].Status))
}

// assertManaged asserts that adoptedResource was patched when 'expectedManaged'
// parameter is true.
// If 'expectedManaged' parameter is false, this function asserts that
// adoptedResource was never patched.
func assertManaged(
	expectedManaged bool,
	t *testing.T,
	ctx context.Context,
	kc *ctrlrtclientmock.Client,
	adoptedRes *ackv1alpha1.AdoptedResource,
) {
	if expectedManaged {
		kc.AssertCalled(t, "Patch", ctx, adoptedRes, mock.AnythingOfType("*client.mergeFromPatch"))
	} else {
		kc.AssertNotCalled(t, "Patch", ctx, adoptedRes, mock.AnythingOfType("*client.mergeFromPatch"))
	}
}

// assertAWSResourceCreation asserts that AWSResource was created and it's spec
// was updated when 'expectedCreation' is true
// If 'expectedCreation' is false, this function asserts that AWSResource was
// neither created nor was the status updated.
func assertAWSResourceCreation(
	expectedCreation bool,
	t *testing.T,
	ctx context.Context,
	kc *ctrlrtclientmock.Client,
	statusWriter *ctrlrtclientmock.StatusWriter,
	res *ackmocks.AWSResource,
) {
	if expectedCreation {
		kc.AssertCalled(t, "Create", ctx, res.RuntimeObject())
		statusWriter.AssertCalled(t, "Update", ctx, res.RuntimeObject())
	} else {
		kc.AssertNotCalled(t, "Create", ctx, res.RuntimeObject())
		statusWriter.AssertNotCalled(t, "Update", ctx, res.RuntimeObject())
	}
}

// assertAWSResourceRead asserts that
// a) Identifiers are set from AdoptedResource to AWSResource
// b) ReadOne call is made to find observed state of AWSResource
// c) APIReader.Get call is made to validate that AWSResource does not already
// exist in k8s cluster
func assertAWSResourceRead(
	t *testing.T,
	ctx context.Context,
	manager *ackmocks.AWSResourceManager,
	apiReader *ctrlrtclientmock.Reader,
	adoptedRes *ackv1alpha1.AdoptedResource,
	res *ackmocks.AWSResource,
) {
	res.AssertCalled(t, "SetIdentifiers", adoptedRes.Spec.AWS)
	manager.AssertCalled(t, "ReadOne", ctx, res)
	apiReader.AssertCalled(t, "Get", ctx, types.NamespacedName{
		Namespace: Namespace,
		Name:      Name,
	}, res.RuntimeObject())
}
