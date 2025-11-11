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

package iamroleselector

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

var (
	testGVR = schema.GroupVersionResource{
		Group:    "services.k8s.aws",
		Version:  "v1alpha1",
		Resource: "iamroleselectors",
	}
)

// TestCache_Matches tests the top-level Matches function
func TestCache_Matches(t *testing.T) {
	// Setup with proper list kind mapping
	scheme := runtime.NewScheme()
	watcher := watch.NewFake()

	// Create fake client with list kind mapping
	gvrToListKind := map[schema.GroupVersionResource]string{
		testGVR: "IAMRoleSelectorList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
	client.PrependWatchReactor("iamroleselectors", k8stesting.DefaultWatchReactor(watcher, nil))

	k8sClient := k8sfake.NewSimpleClientset()
	k8sClient.PrependWatchReactor("production", k8stesting.DefaultWatchReactor(watcher, nil))

	logger := zapr.NewLogger(zap.NewNop())
	cache := NewCache(logger)

	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })

	go cache.Run(client, k8sClient, stopCh)

	// Wait for cache to sync
	require.Eventually(t, func() bool {
		return cache.HasSynced()
	}, 5*time.Second, 10*time.Millisecond)

	// Create test selectors
	selector1 := createSelector("prod-s3", ackv1alpha1.IAMRoleSelector{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-s3"},
		Spec: ackv1alpha1.IAMRoleSelectorSpec{
			ARN: "arn:aws:iam::123456789012:role/prod-s3-role",
			NamespaceSelector: ackv1alpha1.NamespaceSelector{
				Names: []string{"production"},
			},
			ResourceTypeSelector: []schema.GroupVersionKind{
				{Kind: "Bucket"},
			},
		},
	})

	selector2 := createSelector("all-rds", ackv1alpha1.IAMRoleSelector{
		ObjectMeta: metav1.ObjectMeta{Name: "all-rds"},
		Spec: ackv1alpha1.IAMRoleSelectorSpec{
			ARN: "arn:aws:iam::123456789012:role/rds-role",
			ResourceTypeSelector: []schema.GroupVersionKind{
				{
					Group: "rds.services.k8s.aws",
					Kind:  "DBInstance",
				},
			},
		},
	})

	selector3 := createSelector("label-based", ackv1alpha1.IAMRoleSelector{
		ObjectMeta: metav1.ObjectMeta{Name: "label-based"},
		Spec: ackv1alpha1.IAMRoleSelectorSpec{
			ARN: "arn:aws:iam::123456789012:role/team-role",
			NamespaceSelector: ackv1alpha1.NamespaceSelector{
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"team": "platform",
					},
				},
			},
		},
	})

	// Simulate adding selectors via watcher
	watcher.Add(selector1)
	watcher.Add(selector2)
	watcher.Add(selector3)

	// Wait for cache to process
	time.Sleep(100 * time.Millisecond)

	// Test cases
	tests := []struct {
		name      string
		resource  runtime.Object
		wantCount int
		wantARNs  []string
	}{
		{
			name:      "matches production S3 bucket",
			resource:  mockResource("production", "s3.services.k8s.aws", "v1alpha1", "Bucket"),
			wantCount: 1,
			wantARNs:  []string{"arn:aws:iam::123456789012:role/prod-s3-role"},
		},
		{
			name:      "matches RDS in any namespace",
			resource:  mockResource("default", "rds.services.k8s.aws", "v1alpha1", "DBInstance"),
			wantCount: 1,
			wantARNs:  []string{"arn:aws:iam::123456789012:role/rds-role"},
		},
		{
			name:      "no match for wrong namespace",
			resource:  mockResource("development", "s3.services.k8s.aws", "v1alpha1", "Bucket"),
			wantCount: 0,
		},
		{
			name:      "no match for wrong resource type",
			resource:  mockResource("production", "dynamodb.services.k8s.aws", "v1alpha1", "Table"),
			wantCount: 0,
		},
	}
	ctx := context.TODO()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := cache.Matches(ctx, tt.resource)
			require.NoError(t, err)
			require.Len(t, matches, tt.wantCount)

			for i, wantARN := range tt.wantARNs {
				require.Equal(t, wantARN, matches[i].Spec.ARN)
			}
		})
	}

	// Test invalid selector handling
	t.Run("invalid selector not cached", func(t *testing.T) {
		invalidSelector := createSelector("invalid", ackv1alpha1.IAMRoleSelector{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid"},
			Spec: ackv1alpha1.IAMRoleSelectorSpec{
				ARN: "", // Invalid - empty ARN
			},
		})

		watcher.Add(invalidSelector)
		time.Sleep(100 * time.Millisecond)

		// Should not be in cache
		_, found := cache.GetSelector("invalid")
		require.False(t, found)
	})

	// Test update to invalid
	t.Run("update valid to invalid removes from cache", func(t *testing.T) {
		validSelector := createSelector("update-test", ackv1alpha1.IAMRoleSelector{
			ObjectMeta: metav1.ObjectMeta{Name: "update-test"},
			Spec: ackv1alpha1.IAMRoleSelectorSpec{
				ARN: "arn:aws:iam::123456789012:role/test-role",
			},
		})

		watcher.Add(validSelector)
		time.Sleep(100 * time.Millisecond)

		// Should be cached
		_, found := cache.GetSelector("update-test")
		require.True(t, found)

		// Update to invalid
		invalidUpdate := createSelector("update-test", ackv1alpha1.IAMRoleSelector{
			ObjectMeta: metav1.ObjectMeta{Name: "update-test"},
			Spec: ackv1alpha1.IAMRoleSelectorSpec{
				ARN: "not-an-arn", // Invalid ARN format
			},
		})

		watcher.Modify(invalidUpdate)
		time.Sleep(100 * time.Millisecond)

		// Should be removed
		_, found = cache.GetSelector("update-test")
		require.False(t, found)
	})

	// Test deletion
	t.Run("delete removes from cache", func(t *testing.T) {
		watcher.Delete(selector1)
		time.Sleep(100 * time.Millisecond)

		_, found := cache.GetSelector("prod-s3")
		require.False(t, found)
	})
}

// Helper functions

func createSelector(name string, selector ackv1alpha1.IAMRoleSelector) *unstructured.Unstructured {
	obj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(&selector)
	u := &unstructured.Unstructured{Object: obj}
	u.SetAPIVersion("services.k8s.aws/v1alpha1")
	u.SetKind("IAMRoleSelector")
	u.SetName(name)
	return u
}

func mockResource(namespace, group, version, kind string) runtime.Object {
	return &testResource{
		namespace: namespace,
		gvk: schema.GroupVersionKind{
			Group:   group,
			Version: version,
			Kind:    kind,
		},
	}
}

// Minimal test resource implementation
type testResource struct {
	namespace string
	gvk       schema.GroupVersionKind
}

func (r *testResource) GetObjectKind() schema.ObjectKind {
	return &testObjectKind{gvk: r.gvk}
}

func (r *testResource) DeepCopyObject() runtime.Object {
	return r
}

func (r *testResource) GetNamespace() string {
	return r.namespace
}

func (r *testResource) SetNamespace(string)                           {}
func (r *testResource) GetName() string                               { return "test" }
func (r *testResource) SetName(string)                                {}
func (r *testResource) GetGenerateName() string                       { return "" }
func (r *testResource) SetGenerateName(string)                        {}
func (r *testResource) GetUID() types.UID                             { return "test-uid" }
func (r *testResource) SetUID(types.UID)                              {}
func (r *testResource) GetResourceVersion() string                    { return "1" }
func (r *testResource) SetResourceVersion(string)                     {}
func (r *testResource) GetGeneration() int64                          { return 1 }
func (r *testResource) SetGeneration(int64)                           {}
func (r *testResource) GetSelfLink() string                           { return "" }
func (r *testResource) SetSelfLink(string)                            {}
func (r *testResource) GetCreationTimestamp() metav1.Time             { return metav1.Time{} }
func (r *testResource) SetCreationTimestamp(metav1.Time)              {}
func (r *testResource) GetDeletionTimestamp() *metav1.Time            { return nil }
func (r *testResource) SetDeletionTimestamp(*metav1.Time)             {}
func (r *testResource) GetDeletionGracePeriodSeconds() *int64         { return nil }
func (r *testResource) SetDeletionGracePeriodSeconds(*int64)          {}
func (r *testResource) GetLabels() map[string]string                  { return nil }
func (r *testResource) SetLabels(map[string]string)                   {}
func (r *testResource) GetAnnotations() map[string]string             { return nil }
func (r *testResource) SetAnnotations(map[string]string)              {}
func (r *testResource) GetFinalizers() []string                       { return nil }
func (r *testResource) SetFinalizers([]string)                        {}
func (r *testResource) GetOwnerReferences() []metav1.OwnerReference   { return nil }
func (r *testResource) SetOwnerReferences([]metav1.OwnerReference)    {}
func (r *testResource) GetManagedFields() []metav1.ManagedFieldsEntry { return nil }
func (r *testResource) SetManagedFields([]metav1.ManagedFieldsEntry)  {}

type testObjectKind struct {
	gvk schema.GroupVersionKind
}

func (o *testObjectKind) SetGroupVersionKind(gvk schema.GroupVersionKind) {
	o.gvk = gvk
}

func (o *testObjectKind) GroupVersionKind() schema.GroupVersionKind {
	return o.gvk
}
