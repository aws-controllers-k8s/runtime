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
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

func TestMatches(t *testing.T) {
	tests := []struct {
		name     string
		selector *ackv1alpha1.IAMRoleSelector
		ctx      MatchContext
		want     bool
	}{
		{
			name: "empty selector matches everything",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
				},
			},
			ctx: MatchContext{
				Namespace: "default",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: true,
		},
		{
			name: "matches specific namespace by name",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"production", "staging"},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "production",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: true,
		},
		{
			name: "does not match wrong namespace",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"production", "staging"},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "development",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: false,
		},
		{
			name: "matches namespace by labels",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						LabelSelector: ackv1alpha1.LabelSelector{
							MatchLabels: map[string]string{
								"env":  "prod",
								"team": "platform",
							},
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "any-namespace",
				NamespaceLabels: map[string]string{
					"env":  "prod",
					"team": "platform",
					"foo":  "bar", // extra labels should be ignored
				},
			},
			want: true,
		},
		{
			name: "does not match wrong namespace labels",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						LabelSelector: ackv1alpha1.LabelSelector{
							MatchLabels: map[string]string{
								"env": "prod",
							},
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "any-namespace",
				NamespaceLabels: map[string]string{
					"env": "dev",
				},
			},
			want: false,
		},
		{
			name: "matches namespace by name AND labels",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"production"},
						LabelSelector: ackv1alpha1.LabelSelector{
							MatchLabels: map[string]string{
								"env": "prod",
							},
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "production",
				NamespaceLabels: map[string]string{
					"env": "prod",
				},
			},
			want: true,
		},
		{
			name: "does not match if namespace name matches but labels don't",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"production"},
						LabelSelector: ackv1alpha1.LabelSelector{
							MatchLabels: map[string]string{
								"env": "prod",
							},
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "production",
				NamespaceLabels: map[string]string{
					"env": "dev", // wrong label value
				},
			},
			want: false,
		},
		{
			name: "matches resource type by exact GVK",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Group:   "s3.services.k8s.aws",
							Version: "v1alpha1",
							Kind:    "Bucket",
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "default",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: true,
		},
		{
			name: "matches resource type by partial GVK (only kind)",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Kind: "Bucket",
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "default",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: true,
		},
		{
			name: "matches resource type with OR logic (multiple selectors)",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Group:   "rds.services.k8s.aws",
							Version: "v1alpha1",
							Kind:    "DBInstance",
						},
						{
							Group:   "s3.services.k8s.aws",
							Version: "v1alpha1",
							Kind:    "Bucket",
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "default",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: true,
		},
		{
			name: "does not match wrong resource type",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Group:   "rds.services.k8s.aws",
							Version: "v1alpha1",
							Kind:    "DBInstance",
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "default",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: false,
		},
		{
			name: "matches both namespace and resource type",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"production"},
					},
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Kind: "Bucket",
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "production",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: true,
		},
		{
			name: "does not match if namespace matches but resource type doesn't",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"production"},
					},
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Kind: "DBInstance",
						},
					},
				},
			},
			ctx: MatchContext{
				Namespace: "production",
				GVK: schema.GroupVersionKind{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Matches(tt.selector, tt.ctx)
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector *ackv1alpha1.IAMRoleSelector
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "nil selector",
			selector: nil,
			wantErr:  true,
			errMsg:   "selector cannot be nil",
		},
		{
			name: "empty ARN",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "",
				},
			},
			wantErr: true,
			errMsg:  "ARN cannot be empty",
		},
		{
			name: "invalid ARN format",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "not-an-arn",
				},
			},
			wantErr: true,
			errMsg:  "invalid ARN",
		},
		{
			name: "valid minimal selector",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate namespace names",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"prod", "staging", "prod"},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate namespace name: prod",
		},
		{
			name: "empty namespace name",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"prod", ""},
					},
				},
			},
			wantErr: true,
			errMsg:  "namespace name cannot be empty",
		},
		{
			name: "empty label key",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						LabelSelector: ackv1alpha1.LabelSelector{
							MatchLabels: map[string]string{
								"":    "value",
								"env": "prod",
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "label key cannot be empty",
		},
		{
			name: "empty resource type selector",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							// all fields empty
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "at least one of group, version, or kind must be specified at index 0",
		},
		{
			name: "duplicate resource type selectors",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Group:   "s3.services.k8s.aws",
							Version: "v1alpha1",
							Kind:    "Bucket",
						},
						{
							Group:   "s3.services.k8s.aws",
							Version: "v1alpha1",
							Kind:    "Bucket",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate resource type selector: s3.services.k8s.aws/v1alpha1/Bucket",
		},
		{
			name: "valid complex selector",
			selector: &ackv1alpha1.IAMRoleSelector{
				Spec: ackv1alpha1.IAMRoleSelectorSpec{
					ARN: "arn:aws:iam::123456789012:role/test-role",
					NamespaceSelector: ackv1alpha1.NamespaceSelector{
						Names: []string{"prod", "staging"},
						LabelSelector: ackv1alpha1.LabelSelector{
							MatchLabels: map[string]string{
								"env": "production",
							},
						},
					},
					ResourceTypeSelector: []schema.GroupVersionKind{
						{
							Kind: "Bucket",
						},
						{
							Group:   "rds.services.k8s.aws",
							Version: "v1alpha1",
							Kind:    "DBInstance",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSelector(tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err.Error() != tt.errMsg {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateSelector() error message = %v, want substring %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestMatchesNamespace(t *testing.T) {
	tests := []struct {
		name            string
		nsSelector      ackv1alpha1.NamespaceSelector
		namespace       string
		namespaceLabels map[string]string
		want            bool
	}{
		{
			name:       "empty selector matches all",
			nsSelector: ackv1alpha1.NamespaceSelector{},
			namespace:  "any-namespace",
			want:       true,
		},
		{
			name: "matches by name - single",
			nsSelector: ackv1alpha1.NamespaceSelector{
				Names: []string{"production"},
			},
			namespace: "production",
			want:      true,
		},
		{
			name: "matches by name - multiple",
			nsSelector: ackv1alpha1.NamespaceSelector{
				Names: []string{"prod", "staging", "dev"},
			},
			namespace: "staging",
			want:      true,
		},
		{
			name: "does not match by name",
			nsSelector: ackv1alpha1.NamespaceSelector{
				Names: []string{"prod", "staging"},
			},
			namespace: "development",
			want:      false,
		},
		{
			name: "matches by labels",
			nsSelector: ackv1alpha1.NamespaceSelector{
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"env": "prod",
					},
				},
			},
			namespace: "any-namespace",
			namespaceLabels: map[string]string{
				"env": "prod",
			},
			want: true,
		},
		{
			name: "matches by multiple labels",
			nsSelector: ackv1alpha1.NamespaceSelector{
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"env":  "prod",
						"team": "platform",
					},
				},
			},
			namespace: "any-namespace",
			namespaceLabels: map[string]string{
				"env":    "prod",
				"team":   "platform",
				"region": "us-east-1", // extra labels are ok
			},
			want: true,
		},
		{
			name: "does not match - missing label",
			nsSelector: ackv1alpha1.NamespaceSelector{
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"env":  "prod",
						"team": "platform",
					},
				},
			},
			namespace: "any-namespace",
			namespaceLabels: map[string]string{
				"env": "prod", // missing "team" label
			},
			want: false,
		},
		{
			name: "does not match - wrong label value",
			nsSelector: ackv1alpha1.NamespaceSelector{
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"env": "prod",
					},
				},
			},
			namespace: "any-namespace",
			namespaceLabels: map[string]string{
				"env": "dev",
			},
			want: false,
		},
		{
			name: "matches by name AND labels",
			nsSelector: ackv1alpha1.NamespaceSelector{
				Names: []string{"production", "staging"},
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"env": "prod",
					},
				},
			},
			namespace: "production",
			namespaceLabels: map[string]string{
				"env": "prod",
			},
			want: true,
		},
		{
			name: "does not match - correct name but wrong labels",
			nsSelector: ackv1alpha1.NamespaceSelector{
				Names: []string{"production"},
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"env": "prod",
					},
				},
			},
			namespace: "production",
			namespaceLabels: map[string]string{
				"env": "dev",
			},
			want: false,
		},
		{
			name: "does not match - wrong name but correct labels",
			nsSelector: ackv1alpha1.NamespaceSelector{
				Names: []string{"production"},
				LabelSelector: ackv1alpha1.LabelSelector{
					MatchLabels: map[string]string{
						"env": "prod",
					},
				},
			},
			namespace: "development",
			namespaceLabels: map[string]string{
				"env": "prod",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesNamespace(tt.nsSelector, tt.namespace, tt.namespaceLabels)
			if got != tt.want {
				t.Errorf("matchesNamespace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesResourceType(t *testing.T) {
	tests := []struct {
		name        string
		rtSelectors []schema.GroupVersionKind
		gvk         schema.GroupVersionKind
		want        bool
	}{
		{
			name:        "empty selector matches all",
			rtSelectors: []schema.GroupVersionKind{},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: true,
		},
		{
			name: "exact match",
			rtSelectors: []schema.GroupVersionKind{
				{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: true,
		},
		{
			name: "partial match - only kind",
			rtSelectors: []schema.GroupVersionKind{
				{
					Kind: "Bucket",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: true,
		},
		{
			name: "partial match - only group",
			rtSelectors: []schema.GroupVersionKind{
				{
					Group: "s3.services.k8s.aws",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: true,
		},
		{
			name: "partial match - group and version",
			rtSelectors: []schema.GroupVersionKind{
				{
					Group:   "s3.services.k8s.aws",
					Version: "v1alpha1",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: true,
		},
		{
			name: "no match - wrong kind",
			rtSelectors: []schema.GroupVersionKind{
				{
					Kind: "DBInstance",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: false,
		},
		{
			name: "no match - wrong group",
			rtSelectors: []schema.GroupVersionKind{
				{
					Group:   "rds.services.k8s.aws",
					Version: "v1alpha1",
					Kind:    "Bucket",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: false,
		},
		{
			name: "OR logic - multiple selectors",
			rtSelectors: []schema.GroupVersionKind{
				{
					Kind: "DBInstance",
				},
				{
					Kind: "Bucket",
				},
				{
					Kind: "Queue",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: true,
		},
		{
			name: "OR logic - no match",
			rtSelectors: []schema.GroupVersionKind{
				{
					Kind: "DBInstance",
				},
				{
					Kind: "Queue",
				},
			},
			gvk: schema.GroupVersionKind{
				Group:   "s3.services.k8s.aws",
				Version: "v1alpha1",
				Kind:    "Bucket",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesResourceType(tt.rtSelectors, tt.gvk)
			if got != tt.want {
				t.Errorf("matchesResourceType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && contains(s[1:], substr)
}
