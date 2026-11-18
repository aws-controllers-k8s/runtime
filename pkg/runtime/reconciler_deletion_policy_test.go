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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

const testServiceAlias = "bookstore"

var testNSDeletionPolicyKey = testServiceAlias + "." + ackv1alpha1.AnnotationDeletionPolicy

// newNamespaceReader returns a client.Reader serving the given namespaces. If
// getErr is not nil, every Get returns it instead.
func newNamespaceReader(getErr error, namespaces ...*corev1.Namespace) client.Reader {
	scheme := k8sruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	objs := make([]client.Object, 0, len(namespaces))
	for _, ns := range namespaces {
		objs = append(objs, ns)
	}
	b := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
	if getErr != nil {
		b = b.WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return getErr
			},
		})
	}
	return b.Build()
}

func namespaceWithAnnotations(name string, annotations map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations}}
}

func newDeletionPolicyServiceController() *ackmocks.ServiceController {
	sc := &ackmocks.ServiceController{}
	sc.On("GetMetadata").Return(acktypes.ServiceControllerMetadata{ServiceAlias: testServiceAlias})
	return sc
}

var errNamespaceForbidden = apierrors.NewForbidden(
	k8srtschema.GroupResource{Resource: "namespaces"}, "default",
	errors.New("cannot get resource \"namespaces\""),
)

// TestGetDeletionPolicy checks that the namespace deletion-policy annotation
// is read from the API server, independently of the CARM and IAMRoleSelector
// caches (which are not running here).
func TestGetDeletionPolicy(t *testing.T) {
	retain := map[string]string{testNSDeletionPolicyKey: string(ackv1alpha1.DeletionPolicyRetain)}

	tests := []struct {
		name          string
		crAnnotations map[string]string
		reader        client.Reader
		want          ackv1alpha1.DeletionPolicy
		wantErr       bool
	}{
		{
			name:          "CR annotation takes precedence over the namespace",
			crAnnotations: map[string]string{ackv1alpha1.AnnotationDeletionPolicy: string(ackv1alpha1.DeletionPolicyDelete)},
			reader:        newNamespaceReader(nil, namespaceWithAnnotations("default", retain)),
			want:          ackv1alpha1.DeletionPolicyDelete,
		},
		{
			name:          "CR annotation is used without reading the namespace",
			crAnnotations: map[string]string{ackv1alpha1.AnnotationDeletionPolicy: string(ackv1alpha1.DeletionPolicyRetain)},
			reader:        newNamespaceReader(errors.New("must not be called")),
			want:          ackv1alpha1.DeletionPolicyRetain,
		},
		{
			name:   "namespace annotation for this service is used",
			reader: newNamespaceReader(nil, namespaceWithAnnotations("default", retain)),
			want:   ackv1alpha1.DeletionPolicyRetain,
		},
		{
			name: "namespace annotation for another service is ignored",
			reader: newNamespaceReader(nil, namespaceWithAnnotations("default", map[string]string{
				"s3." + ackv1alpha1.AnnotationDeletionPolicy: string(ackv1alpha1.DeletionPolicyRetain),
			})),
			want: ackv1alpha1.DeletionPolicyDelete,
		},
		{
			name:   "namespace without annotation falls back to the flag",
			reader: newNamespaceReader(nil, namespaceWithAnnotations("default", nil)),
			want:   ackv1alpha1.DeletionPolicyDelete,
		},
		{
			name:   "namespace not found falls back to the flag",
			reader: newNamespaceReader(nil),
			want:   ackv1alpha1.DeletionPolicyDelete,
		},
		{
			name:   "forbidden namespace read falls back to the flag",
			reader: newNamespaceReader(errNamespaceForbidden),
			want:   ackv1alpha1.DeletionPolicyDelete,
		},
		{
			name:    "other namespace read errors are returned",
			reader:  newNamespaceReader(errors.New("connection refused")),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, metaObj := newMockRes()
			metaObj.SetAnnotations(tt.crAnnotations)

			rec, _ := newPreDeleteReconciler(&ackmocks.AWSResourceDescriptor{})
			rec.sc = newDeletionPolicyServiceController()
			rec.apiReader = tt.reader

			got, err := rec.getDeletionPolicy(context.Background(), res)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// newNamespacePolicyDeleteFixture builds a reconciler and a resource being
// deleted, with no CR-level deletion-policy annotation and the --deletion-policy
// flag set to delete.
func newNamespacePolicyDeleteFixture(reader client.Reader) (
	*resourceReconciler, *ackmocks.AWSResourceManager, *ackmocks.AWSResourceDescriptor, *ackmocks.AWSResource,
) {
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "bookstore.services.k8s.aws", Kind: "Book",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	desired, metaObj := newMockRes()
	metaObj.SetAnnotations(nil)
	desired.On("IsBeingDeleted").Return(true)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()

	rd.On("IsManaged", desired).Return(true)
	rd.On("MarkUnmanaged", desired).Return()
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)
	rd.On("Delta", mock.Anything, mock.Anything).Return(ackcompare.NewDelta())

	rec, kc := newPreDeleteReconciler(rd)
	rec.sc = newDeletionPolicyServiceController()
	rec.apiReader = reader
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	rm := &ackmocks.AWSResourceManager{}
	rm.On("ClearResolvedReferences", mock.Anything).Return(desired)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)
	return rec, rm, rd, desired
}

// TestReconcileDelete_NamespaceRetain is the scenario from
// aws-controllers-k8s/community#3154: CARM and IAMRoleSelector caches are not
// running, the namespace is annotated retain, and the AWS resource must not be
// deleted.
func TestReconcileDelete_NamespaceRetain(t *testing.T) {
	rec, rm, rd, desired := newNamespacePolicyDeleteFixture(newNamespaceReader(nil,
		namespaceWithAnnotations("default", map[string]string{
			testNSDeletionPolicyKey: string(ackv1alpha1.DeletionPolicyRetain),
		}),
	))

	_, err := rec.reconcile(context.Background(), rm, desired)
	require.NoError(t, err)

	rm.AssertNotCalled(t, "ReadOne", mock.Anything, mock.Anything)
	rm.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	rd.AssertCalled(t, "MarkUnmanaged", desired)
}

// TestReconcileDelete_NamespaceReadError checks that a transient error reading
// the namespace stops the delete and is returned, so it is retried.
func TestReconcileDelete_NamespaceReadError(t *testing.T) {
	rec, rm, rd, desired := newNamespacePolicyDeleteFixture(newNamespaceReader(errors.New("connection refused")))

	_, err := rec.reconcile(context.Background(), rm, desired)
	require.Error(t, err)

	rm.AssertNotCalled(t, "ReadOne", mock.Anything, mock.Anything)
	rm.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	rd.AssertNotCalled(t, "MarkUnmanaged", mock.Anything)
}
