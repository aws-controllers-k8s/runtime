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
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func newTestServiceController(cfg ackcfg.Config) *serviceController {
	return &serviceController{
		ServiceControllerMetadata: acktypes.ServiceControllerMetadata{
			VersionInfo: acktypes.VersionInfo{
				GitCommit:  "test-commit",
				BuildDate:  "test-date",
				GitVersion: "v0.0.0-test",
			},
			ServiceAlias:    "test",
			ServiceAPIGroup: "test.services.k8s.aws",
		},
		cfg: cfg,
	}
}

// TestNewAWSConfig_HTTPClientTimeout_Fires verifies that when a non-zero
// HTTPClientTimeout is configured, the resulting aws.Config's HTTPClient
// enforces that timeout on a stuck request.
func TestNewAWSConfig_HTTPClientTimeout_Fires(t *testing.T) {
	require := require.New(t)

	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		// Provide dummy static creds via env so LoadDefaultConfig succeeds.
		t.Setenv("AWS_ACCESS_KEY_ID", "test")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		t.Setenv("AWS_REGION", "us-west-2")
	}

	// Slow-loris server that sleeps past the client timeout before responding.
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Client gave up; let the server unblock.
		}
	})
	server := httptest.NewServer(slowHandler)
	defer server.Close()

	sc := newTestServiceController(ackcfg.Config{
		HTTPClientTimeout: 100 * time.Millisecond,
	})

	awsCfg, err := sc.NewAWSConfig(
		context.Background(),
		ackv1alpha1.AWSRegion("us-west-2"),
		nil, // endpointURL — unused for this test, we hit the http client directly
		"",
		schema.GroupVersionKind{Group: "test", Version: "v1alpha1", Kind: "TestKind"},
		nil,
	)
	require.NoError(err)
	require.NotNil(awsCfg.HTTPClient, "NewAWSConfig should set HTTPClient")

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(err)

	start := time.Now()
	resp, err := awsCfg.HTTPClient.Do(req)
	elapsed := time.Since(start)

	require.Error(err, "request against slow-loris server should time out")
	if resp != nil {
		_ = resp.Body.Close()
	}
	// The error should be a net timeout.
	var netErr interface{ Timeout() bool }
	assert.True(t, errors.As(err, &netErr) && netErr.Timeout(),
		"error should be a timeout error, got: %v", err)

	// Elapsed time should be in [timeout, timeout + slack].
	assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond,
		"timeout fired too early")
	assert.Less(t, elapsed, 500*time.Millisecond,
		"timeout did not fire; elapsed %v suggests client waited for server", elapsed)
}

// TestNewAWSConfig_HTTPClientTimeout_Zero verifies that a zero timeout
// disables the whole-request timeout.
func TestNewAWSConfig_HTTPClientTimeout_Zero(t *testing.T) {
	require := require.New(t)

	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		t.Setenv("AWS_ACCESS_KEY_ID", "test")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		t.Setenv("AWS_REGION", "us-west-2")
	}

	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(slowHandler)
	defer server.Close()

	sc := newTestServiceController(ackcfg.Config{
		HTTPClientTimeout: 0,
	})

	awsCfg, err := sc.NewAWSConfig(
		context.Background(),
		ackv1alpha1.AWSRegion("us-west-2"),
		nil,
		"",
		schema.GroupVersionKind{Group: "test", Version: "v1alpha1", Kind: "TestKind"},
		nil,
	)
	require.NoError(err)
	require.NotNil(awsCfg.HTTPClient)

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(err)

	resp, err := awsCfg.HTTPClient.Do(req)
	require.NoError(err, "with HTTPClientTimeout=0, 150ms server should not time out")
	require.NotNil(resp)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
