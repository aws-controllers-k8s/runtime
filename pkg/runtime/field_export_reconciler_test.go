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
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
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
)

const (
	FieldExportNamespace = "default"
	FieldExportName      = "exportedField"
)

// Helper functions for tests

func mockFieldExportReconciler() (acktypes.FieldExportReconciler, *ctrlrtclientmock.Client, *ctrlrtclientmock.Reader) {
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
		nil,
	), kc, apiReader
}

func setupMockClientForFieldExport(kc *ctrlrtclientmock.Client, statusWriter *ctrlrtclientmock.StatusWriter, ctx context.Context, fieldExport *ackv1alpha1.FieldExport) {
	kc.On("Status").Return(statusWriter)
	statusWriter.On("Patch", ctx, fieldExport, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Patch", ctx, fieldExport, mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
	kc.On("Patch", ctx, mock.AnythingOfType("*v1.ConfigMap"), mock.AnythingOfType("*client.mergeFromPatch")).Return(nil)
}

func setupMockApiReaderForFieldExport(apiReader *ctrlrtclientmock.Reader, ctx context.Context, res *ackmocks.AWSResource) {
	apiReader.On("Get", ctx, types.NamespacedName{
		Namespace: FieldExportNamespace,
		Name:      "fake-export-configmap",
	}, mock.AnythingOfType("*v1.ConfigMap")).Return(nil)
}

func strPtr(str string) *string {
	return &str
}

func fieldExport(namespace, name string) *ackv1alpha1.FieldExport {
	exportType := ackv1alpha1.FieldExportOutputTypeConfigMap
	return &ackv1alpha1.FieldExport{
		TypeMeta: v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: ackv1alpha1.FieldExportSpec{
			From: &ackv1alpha1.ResourceFieldSelector{
				Path: strPtr(".spec.name"),
				Resource: ackv1alpha1.NamespacedTargetKubernetesResource{
					GroupKind: v1.GroupKind{
						Group: "fake-api-group",
						Kind:  "fake-kind",
					},
					Name: strPtr("fake-resource"),
				},
			},
			To: &ackv1alpha1.FieldExportOutputSelector{
				Name: strPtr("fake-export-configmap"),
				Kind: &exportType,
			},
		},
		Status: ackv1alpha1.FieldExportStatus{},
	}
}

func setupMockUnstructuredConverter() {
	conv := &apimachineryruntimemock.UnstructuredConverter{}
	conv.On("ToUnstructured", mock.AnythingOfType("*mocks.Object")).Return(
		map[string]interface{}{
			"spec": map[string]interface{}{
				"name": "test-book-name",
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

//Tests
func TestSync_FailureInGetResource(t *testing.T) {

}

func TestSync_FailureInGetField(t *testing.T) {

}

func TestSync_FailureInPatchConfigMap(t *testing.T) {

}

func TestSync_HappyCaseConfigMap(t *testing.T) {
	// Setup
	require := require.New(t)
	// Mock resource creation
	r, kc, apiReader := mockFieldExportReconciler()
	descriptor, res, _ := mockDescriptorAndAWSResource()
	manager := mockManager()
	fieldExport := fieldExport(FieldExportNamespace, FieldExportName)
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
	assertPatchedConfigMap(t, ctx, kc)
}

func TestSync_HappyCaseSecret(t *testing.T) {
}

func TestSync_HappyCaseResourceUpdated(t *testing.T) {
}

func TestSync_HappyCaseResourceNoExports(t *testing.T) {
}

// Assertions

func assertPatchedConfigMap(t *testing.T, ctx context.Context, kc *ctrlrtclientmock.Client) {
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
	kc.AssertCalled(t, "Patch", ctx, dataMatcher, mock.Anything)
}
