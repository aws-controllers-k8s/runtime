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

package secret_test

import (
	"context"
	"testing"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	acksecret "github.com/aws-controllers-k8s/runtime/pkg/secret"
	k8scorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIndexKey(t *testing.T) {
	tests := []struct {
		name              string
		ref               *ackv1alpha1.SecretKeyReference
		resourceNamespace string
		want              string
	}{
		{
			name: "uses ref namespace",
			ref: &ackv1alpha1.SecretKeyReference{
				SecretReference: k8scorev1.SecretReference{
					Namespace: "custom-ns",
					Name:      "my-secret",
				},
			},
			resourceNamespace: "default",
			want:              "custom-ns/my-secret",
		},
		{
			name: "falls back to resource namespace",
			ref: &ackv1alpha1.SecretKeyReference{
				SecretReference: k8scorev1.SecretReference{
					Name: "my-secret",
				},
			},
			resourceNamespace: "default",
			want:              "default/my-secret",
		},
		{
			name:              "nil ref",
			ref:               nil,
			resourceNamespace: "default",
			want:              "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := acksecret.IndexKey(tt.ref, tt.resourceNamespace)
			if got != tt.want {
				t.Errorf("IndexKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetResourceVersionsAnnotation_DeduplicatesLookups(t *testing.T) {
	lookupCount := 0
	lookupFn := func(_ context.Context, _ *ackv1alpha1.SecretKeyReference, _ string) (string, error) {
		lookupCount++
		return "42", nil
	}

	refs := []*ackv1alpha1.SecretKeyReference{
		{
			SecretReference: k8scorev1.SecretReference{Namespace: "default", Name: "my-secret"},
			Key:             "username",
		},
		{
			SecretReference: k8scorev1.SecretReference{Namespace: "default", Name: "my-secret"},
			Key:             "password",
		},
		{
			SecretReference: k8scorev1.SecretReference{Namespace: "default", Name: "other-secret"},
			Key:             "token",
		},
	}

	obj := &metav1.ObjectMeta{
		Namespace:   "default",
		Annotations: map[string]string{},
	}

	acksecret.SetResourceVersionsAnnotation(context.Background(), refs, obj, lookupFn)

	if lookupCount != 2 {
		t.Errorf("expected 2 lookups (one per unique secret), got %d", lookupCount)
	}

	ann := obj.GetAnnotations()[ackv1alpha1.AnnotationSecretResourceVersions]
	if ann == "" {
		t.Fatal("expected annotation to be set")
	}
}
