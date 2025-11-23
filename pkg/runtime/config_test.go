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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatUserAgent(t *testing.T) {
	tests := []struct {
		name     string
		appName  string
		version  string
		extra    []string
		expected string
	}{
		{
			name:     "basic user agent without extras",
			appName:  "aws-controllers-k8s",
			version:  "s3-v1.2.3",
			extra:    nil,
			expected: "aws-controllers-k8s/s3-v1.2.3",
		},
		{
			name:     "user agent with single extra",
			appName:  "aws-controllers-k8s",
			version:  "s3-v1.2.3",
			extra:    []string{"GitCommit/abc123"},
			expected: "aws-controllers-k8s/s3-v1.2.3 (GitCommit/abc123)",
		},
		{
			name:    "user agent with multiple extras",
			appName: "aws-controllers-k8s",
			version: "dynamodb-v1.2.3",
			extra: []string{
				"GitCommit/abc123",
				"BuildDate/2024-01-01",
				"CRDKind/Table",
				"CRDVersion/v1alpha1",
			},
			expected: "aws-controllers-k8s/dynamodb-v1.2.3 (GitCommit/abc123; BuildDate/2024-01-01; CRDKind/Table; CRDVersion/v1alpha1)",
		},
		{
			name:    "user agent with kro managed info",
			appName: "aws-controllers-k8s",
			version: "s3-v1.2.3",
			extra: []string{
				"GitCommit/abc123",
				"BuildDate/2024-01-01",
				"CRDKind/Bucket",
				"CRDVersion/v1alpha1",
				"ManagedBy/kro",
				"KROVersion/v0.1.0",
			},
			expected: "aws-controllers-k8s/s3-v1.2.3 (GitCommit/abc123; BuildDate/2024-01-01; CRDKind/Bucket; CRDVersion/v1alpha1; ManagedBy/kro; KROVersion/v0.1.0)",
		},
		{
			name:    "user agent with kro managed but no version",
			appName: "aws-controllers-k8s",
			version: "s3-v1.2.3",
			extra: []string{
				"GitCommit/abc123",
				"BuildDate/2024-01-01",
				"CRDKind/Bucket",
				"CRDVersion/v1alpha1",
				"ManagedBy/kro",
			},
			expected: "aws-controllers-k8s/s3-v1.2.3 (GitCommit/abc123; BuildDate/2024-01-01; CRDKind/Bucket; CRDVersion/v1alpha1; ManagedBy/kro)",
		},
		{
			name:     "empty extra slice",
			appName:  "test-app",
			version:  "v1.0.0",
			extra:    []string{},
			expected: "test-app/v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUserAgent(tt.appName, tt.version, tt.extra...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsKROManaged(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name: "managed by kro",
			labels: map[string]string{
				LabelManagedBy: "kro",
			},
			expected: true,
		},
		{
			name: "managed by kro with other labels",
			labels: map[string]string{
				"app":          "myapp",
				LabelManagedBy: "kro",
				"env":          "prod",
			},
			expected: true,
		},
		{
			name: "managed by different controller",
			labels: map[string]string{
				LabelManagedBy: "helm",
			},
			expected: false,
		},
		{
			name: "managed-by label not present",
			labels: map[string]string{
				"app": "myapp",
				"env": "prod",
			},
			expected: false,
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: false,
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name: "legacy kro.run/owned label (backward compatibility)",
			labels: map[string]string{
				LabelKroOwned: "true",
			},
			expected: true,
		},
		{
			name: "legacy kro.run/owned false",
			labels: map[string]string{
				LabelKroOwned: "false",
			},
			expected: false,
		},
		{
			name: "standard label takes precedence over legacy",
			labels: map[string]string{
				LabelManagedBy: "kro",
				LabelKroOwned:  "false",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKROManaged(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetKROVersion(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name: "kro version present",
			labels: map[string]string{
				LabelKroVersion: "v0.1.0",
			},
			expected: "v0.1.0",
		},
		{
			name: "kro version with other labels",
			labels: map[string]string{
				"app":           "myapp",
				LabelKroVersion: "v1.2.3",
				"env":           "prod",
			},
			expected: "v1.2.3",
		},
		{
			name: "kro version not present",
			labels: map[string]string{
				"app": "myapp",
				"env": "prod",
			},
			expected: "",
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: "",
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: "",
		},
		{
			name: "kro version with empty value",
			labels: map[string]string{
				LabelKroVersion: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getKROVersion(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}
