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
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sobj "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackrt "github.com/aws-controllers-k8s/runtime/pkg/runtime"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"

	apimachineryruntimemock "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime"
	k8srtschemamocks "github.com/aws-controllers-k8s/runtime/mocks/apimachinery/pkg/runtime/schema"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	mocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
)

const (
	FieldExportNamespace = "default"
	FieldExportName      = "exportedField"
	SourceResourceName   = "my-book"
)

var (
	BookGVK = k8srtschema.GroupVersionKind{
		Group:   "bookstore.services.k8s.aws",
		Kind:    "Book",
		Version: "v1alpha1",
	}
)

// Helper functions for tests

func mockFieldExportReconciler() (acktypes.FieldExportReconciler, *ctrlrtclientmock.Client, *ctrlrtclientmock.Reader) {
	return mockFieldExportReconcilerWithResourceDescriptor(nil)
}

func mockFieldExportReconcilerWithResourceDescriptor(rd *mocks.AWSResourceDescriptor) (acktypes.FieldExportReconciler, *ctrlrtclientmock.Client, *ctrlrtclientmock.Reader) {
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
	return ackrt.NewFieldExportReconcilerWithClient(
		sc,
		fakeLogger,
		cfg,
		metrics,
		ackrtcache.Caches{},
		kc,
		apiReader,
		rd,
	), kc, apiReader
}

func mockResourceDescriptor() *mocks.AWSResourceDescriptor {
	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupKind").Return(
		&metav1.GroupKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "fakeBook",
		},
	)
	rd.On("EmptyRuntimeObject").Return(
		&fakeBook{},
	)
	return rd
}

func setupMockClientForFieldExport(kc *ctrlrtclientmock.Client, statusWriter *ctrlrtclientmock.StatusWriter, ctx context.Context, fieldExport *ackv1alpha1.FieldExport) {
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", ctx, fieldExport, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Patch", ctx, fieldExport, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Patch", ctx, mock.AnythingOfType("*v1.ConfigMap"), mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Patch", ctx, mock.AnythingOfType("*v1.Secret"), mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
}

func setupMockApiReaderForFieldExport(apiReader *ctrlrtclientmock.Reader, ctx context.Context, res *ackmocks.AWSResource) {
	apiReader.On("Get", ctx, types.NamespacedName{
		Namespace: FieldExportNamespace,
		Name:      "fake-export-output",
	}, mock.AnythingOfType("*v1.ConfigMap")).Return(nil)
	apiReader.On("Get", ctx, types.NamespacedName{
		Namespace: FieldExportNamespace,
		Name:      "fake-export-output",
	}, mock.AnythingOfType("*v1.Secret")).Return(nil)
}

func strPtr(str string) *string {
	return &str
}

func fieldExportConfigMap(namespace, name string) *ackv1alpha1.FieldExport {
	return fieldExportWithPath(namespace, name, ackv1alpha1.FieldExportOutputTypeConfigMap, ".spec.name")
}

func fieldExportSecret(namespace, name string) *ackv1alpha1.FieldExport {
	return fieldExportWithPath(namespace, name, ackv1alpha1.FieldExportOutputTypeSecret, ".spec.name")
}

func fieldExportWithPath(namespace, name string, kind ackv1alpha1.FieldExportOutputType, path string) *ackv1alpha1.FieldExport {
	return &ackv1alpha1.FieldExport{
		TypeMeta: v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{
			Namespace:  namespace,
			Name:       name,
			Finalizers: []string{"finalizers.services.k8s.aws/FieldExport"},
		},
		Spec: ackv1alpha1.FieldExportSpec{
			From: &ackv1alpha1.ResourceFieldSelector{
				Path: &path,
				Resource: ackv1alpha1.NamespacedTargetKubernetesResource{
					GroupKind: v1.GroupKind{
						Group: BookGVK.Group,
						Kind:  BookGVK.Kind,
					},
					Name: strPtr(SourceResourceName),
				},
			},
			To: &ackv1alpha1.FieldExportOutputSelector{
				Name: strPtr("fake-export-output"),
				Kind: &kind,
			},
		},
		Status: ackv1alpha1.FieldExportStatus{},
	}
}

func randomFieldExportList() []ackv1alpha1.FieldExport {
	// Matching cases
	defaultConfigMap := fieldExportConfigMap(FieldExportNamespace, "export-1")
	defaultSecret := fieldExportSecret(FieldExportNamespace, "export-2")

	// Non-matching cases
	differentSourceName := fieldExportConfigMap(FieldExportNamespace, "export-3")
	differentSourceName.Spec.From.Resource.Name = strPtr("some-other-name")
	differentSourceKind := fieldExportConfigMap(FieldExportNamespace, "export-4")
	differentSourceKind.Spec.From.Resource.Kind = "some-other-kind"
	notFinalized := fieldExportConfigMap(FieldExportNamespace, "export-5")
	notFinalized.Finalizers = []string{}

	return []ackv1alpha1.FieldExport{
		*defaultConfigMap,
		*defaultSecret,
		*differentSourceName,
		*differentSourceKind,
		*notFinalized,
	}
}

func setupMockUnstructuredConverter() {
	conv := &apimachineryruntimemock.UnstructuredConverter{}
	conv.On("ToUnstructured", mock.AnythingOfType("*mocks.Object")).Return(
		map[string]interface{}{
			"spec": map[string]interface{}{
				"name":  "test-book-name",
				"other": 1,
			},
			"status": map[string]interface{}{
				"other":  "abc",
				"other2": 3,
			},
		}, nil,
	)
	// Update the package variable
	ackrt.UnstructuredConverter = conv
}

func mockSourceResource() (
	*ackmocks.AWSResource, // mocked resource
	*ctrlrtclientmock.Object, // mocked k8s controller-runtime RuntimeObject
	*k8sobj.Unstructured, // NON-mocked k8s apimachinery meta object
) {
	objKind := &k8srtschemamocks.ObjectKind{}
	objKind.On("GroupVersionKind").Return(BookGVK)

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

//Tests

func TestSync_FailureInGetField(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockFieldExportReconciler()
	descriptor, res, _ := mockDescriptorAndAWSResource()
	manager := mockManager()
	fieldExport := fieldExportWithPath(FieldExportNamespace, FieldExportName, ackv1alpha1.FieldExportOutputTypeConfigMap, ".doesnt.exist")
	sourceResource, _, _ := mockSourceResource()
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockClientForFieldExport(kc, statusWriter, ctx, fieldExport)
	setupMockApiReaderForFieldExport(apiReader, ctx, res)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)
	setupMockUnstructuredConverter()

	// Call
	err := r.Sync(ctx, sourceResource, *fieldExport)

	//Assertions
	require.NotNil(err)
	require.Equal("path does not exist in this object", err.Error())
	assertPatchedConfigMap(false, t, ctx, kc)
	assertPatchedSecret(false, t, ctx, kc)
}

func TestSync_FailureInPatchConfigMap(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockFieldExportReconciler()
	descriptor, res, _ := mockDescriptorAndAWSResource()
	manager := mockManager()
	fieldExport := fieldExportConfigMap(FieldExportNamespace, FieldExportName)
	sourceResource, _, _ := mockSourceResource()
	ctx := context.TODO()

	//Mock behavior setup
	setupMockApiReaderForFieldExport(apiReader, ctx, res)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)
	setupMockUnstructuredConverter()
	kc.On("Patch", ctx, mock.AnythingOfType("*v1.ConfigMap"), mock.AnythingOfType("*client.mergeFromPatch")).Return(errors.New("patching denied"))

	// Call
	err := r.Sync(ctx, sourceResource, *fieldExport)

	//Assertions
	require.NotNil(err)
	require.Equal("patching denied", err.Error())
	assertPatchedConfigMap(true, t, ctx, kc)
	assertPatchedSecret(false, t, ctx, kc)
}

func TestSync_HappyCaseConfigMap(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockFieldExportReconciler()
	descriptor, res, _ := mockDescriptorAndAWSResource()
	manager := mockManager()
	fieldExport := fieldExportConfigMap(FieldExportNamespace, FieldExportName)
	sourceResource, _, _ := mockSourceResource()
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockClientForFieldExport(kc, statusWriter, ctx, fieldExport)
	setupMockApiReaderForFieldExport(apiReader, ctx, res)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)
	setupMockUnstructuredConverter()

	// Call
	err := r.Sync(ctx, sourceResource, *fieldExport)

	//Assertions
	require.Nil(err)
	assertPatchedConfigMap(true, t, ctx, kc)
	assertPatchedSecret(false, t, ctx, kc)
}

func TestSync_HappyCaseSecret(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockFieldExportReconciler()
	descriptor, res, _ := mockDescriptorAndAWSResource()
	manager := mockManager()
	fieldExport := fieldExportSecret(FieldExportNamespace, FieldExportName)
	sourceResource, _, _ := mockSourceResource()
	ctx := context.TODO()
	statusWriter := &ctrlrtclientmock.StatusWriter{}

	//Mock behavior setup
	setupMockClientForFieldExport(kc, statusWriter, ctx, fieldExport)
	setupMockApiReaderForFieldExport(apiReader, ctx, res)
	setupMockManager(manager, ctx, res)
	setupMockDescriptor(descriptor, res)
	setupMockUnstructuredConverter()

	// Call
	err := r.Sync(ctx, sourceResource, *fieldExport)

	//Assertions
	require.Nil(err)
	assertPatchedConfigMap(false, t, ctx, kc)
	assertPatchedSecret(true, t, ctx, kc)
}

func TestFilterAllExports_HappyCase(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, _, apiReader := mockFieldExportReconciler()
	ctx := context.TODO()
	mockExports := randomFieldExportList()
	apiReader.On("List", ctx, mock.AnythingOfType("*v1alpha1.FieldExportList"), mock.Anything).Return(nil).
		Run(func(args mock.Arguments) {
			// Replace the field export list argument pointer with our mocks
			list := args.Get(1).(*ackv1alpha1.FieldExportList)
			mockList := ackv1alpha1.FieldExportList{
				Items: mockExports,
			}
			*list = mockList
		})
	gk := metav1.GroupKind{
		Group: BookGVK.Group,
		Kind:  BookGVK.Kind,
	}
	sourceNsn := types.NamespacedName{
		Namespace: FieldExportNamespace,
		Name:      SourceResourceName,
	}

	// Call
	exports, err := r.FilterAllExports(ctx, gk, sourceNsn)

	//Assertions
	require.Nil(err)
	require.EqualValues(exports, mockExports[:2])
}

func TestSync_HappyCaseResourceNoExports(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, _, apiReader := mockFieldExportReconciler()
	ctx := context.TODO()
	mockExports := randomFieldExportList()

	apiReader.On("List", ctx, mock.AnythingOfType("*v1alpha1.FieldExportList"), mock.Anything).Return(nil).
		Run(func(args mock.Arguments) {
			// Replace the field export list argument pointer with our mocks
			list := args.Get(1).(*ackv1alpha1.FieldExportList)
			mockList := ackv1alpha1.FieldExportList{
				Items: mockExports,
			}
			*list = mockList
		})
	gk := metav1.GroupKind{
		Group: BookGVK.Group,
		Kind:  BookGVK.Kind,
	}
	sourceNsn := types.NamespacedName{
		Namespace: FieldExportNamespace,
		Name:      "doesnt-exist",
	}

	// Call
	exports, err := r.FilterAllExports(ctx, gk, sourceNsn)

	//Assertions
	require.Nil(err)
	require.Len(exports, 0)
}

// Assertions

func assertPatchedConfigMap(expected bool, t *testing.T, ctx context.Context, kc *ctrlrtclientmock.Client) {
	dataMatcher := mock.MatchedBy(func(cm *corev1.ConfigMap) bool {
		if cm.Data == nil {
			return false
		}
		key := fmt.Sprintf("%s.%s", FieldExportNamespace, FieldExportName)
		val, ok := cm.Data[key]
		if !ok {
			return false
		}
		return val == "test-book-name"
	})
	if expected {
		kc.AssertCalled(t, "Patch", ctx, dataMatcher, mock.Anything)
	} else {
		kc.AssertNotCalled(t, "Patch", ctx, dataMatcher, mock.Anything)
	}
}

func assertPatchedSecret(expected bool, t *testing.T, ctx context.Context, kc *ctrlrtclientmock.Client) {
	dataMatcher := mock.MatchedBy(func(cm *corev1.Secret) bool {
		if cm.Data == nil {
			return false
		}
		key := fmt.Sprintf("%s.%s", FieldExportNamespace, FieldExportName)
		val, ok := cm.Data[key]
		if !ok {
			return false
		}
		return bytes.Equal(val, []byte("test-book-name"))
	})
	if expected {
		kc.AssertCalled(t, "Patch", ctx, dataMatcher, mock.Anything)
	} else {
		kc.AssertNotCalled(t, "Patch", ctx, dataMatcher, mock.Anything)
	}
}