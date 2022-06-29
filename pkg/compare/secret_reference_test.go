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

package compare_test

import (
	"reflect"
	"testing"

	k8scorev1 "k8s.io/api/core/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
)

// newSecretReference is used a instantiate a secret reference used for testing purposes.
func newSecretReference(name string) *ackv1alpha1.SecretKeyReference {
	return &ackv1alpha1.SecretKeyReference{
		SecretReference: k8scorev1.SecretReference{
			Namespace: "default",
			Name:      name,
		},
		Key: "password",
	}
}

func TestSliceSecretKeyReferenceEqual(t *testing.T) {
	type args struct {
		a []*ackv1alpha1.SecretKeyReference
		b []*ackv1alpha1.SecretKeyReference
	}
	tests := []struct {
		name        string
		args        args
		wantEqual   bool
		wantAdded   []*ackv1alpha1.SecretKeyReference
		wantRemoved []*ackv1alpha1.SecretKeyReference
	}{
		{
			name: "empty slices",
			args: args{
				a: nil,
				b: nil,
			},
			wantEqual: true,
		},
		{
			name: "empty slices - only one non nil slice",
			args: args{
				a: nil,
				b: []*ackv1alpha1.SecretKeyReference{},
			},
			wantEqual: true,
		},
		{
			name: "empty slices - two non nil slices",
			args: args{
				a: []*ackv1alpha1.SecretKeyReference{},
				b: []*ackv1alpha1.SecretKeyReference{},
			},
			wantEqual: true,
		},
		{
			name: "added secrets",
			args: args{
				a: []*ackv1alpha1.SecretKeyReference{},
				b: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret1"),
					newSecretReference("secret2"),
				},
			},
			wantEqual: false,
			wantAdded: []*ackv1alpha1.SecretKeyReference{
				newSecretReference("secret1"),
				newSecretReference("secret2"),
			},
		},
		{
			name: "removed secrets",
			args: args{
				a: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret1"),
					newSecretReference("secret2"),
				},
				b: []*ackv1alpha1.SecretKeyReference{},
			},
			wantEqual: false,
			wantRemoved: []*ackv1alpha1.SecretKeyReference{
				newSecretReference("secret1"),
				newSecretReference("secret2"),
			},
		},
		{
			name: "added and removed secrets",
			args: args{
				a: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret1"),
				},
				b: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret2"),
				},
			},
			wantEqual: false,
			wantAdded: []*ackv1alpha1.SecretKeyReference{
				newSecretReference("secret2"),
			},
			wantRemoved: []*ackv1alpha1.SecretKeyReference{
				newSecretReference("secret1"),
			},
		},
		{
			name: "equal slices with duplicate elements",
			args: args{
				a: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret1"),
					newSecretReference("secret1"),
					newSecretReference("secret1"),
					newSecretReference("secret2"),
				},
				b: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret2"),
					newSecretReference("secret2"),
					newSecretReference("secret2"),
					newSecretReference("secret1"),
				},
			},
			wantEqual:   true,
			wantAdded:   nil,
			wantRemoved: nil,
		},
		{
			name: "added and removed secrets with duplicate elements",
			args: args{
				a: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret1"),
					newSecretReference("secret2"),
					newSecretReference("secret2"),
					newSecretReference("secret3"),
				},
				b: []*ackv1alpha1.SecretKeyReference{
					newSecretReference("secret3"),
					newSecretReference("secret4"),
					newSecretReference("secret4"),
				},
			},
			wantEqual: false,
			wantAdded: []*ackv1alpha1.SecretKeyReference{
				newSecretReference("secret4"),
			},
			wantRemoved: []*ackv1alpha1.SecretKeyReference{
				newSecretReference("secret1"),
				newSecretReference("secret2"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEqual, gotAdded, gotRemoved := ackcompare.CompareSecretKeyReferences(tt.args.a, tt.args.b)
			if gotEqual != tt.wantEqual {
				t.Errorf("SliceSecretKeyReferenceEqual() gotEqual = %v, want %v", gotEqual, tt.wantEqual)
			}
			if !reflect.DeepEqual(gotAdded, tt.wantAdded) {
				t.Errorf("SliceSecretKeyReferenceEqual() gotAdded = %v, want %v", gotAdded, tt.wantAdded)
			}
			if !reflect.DeepEqual(gotRemoved, tt.wantRemoved) {
				t.Errorf("SliceSecretKeyReferenceEqual() gotRemoved = %v, want %v", gotRemoved, tt.wantRemoved)
			}
		})
	}
}
