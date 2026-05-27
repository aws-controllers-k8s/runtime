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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
)

func TestValidateCrossNamespaceReference(t *testing.T) {
	const ownerNs = "owner-ns"
	const otherNs = "other-ns"
	const refName = "my-ref"

	tests := []struct {
		name            string
		flag            bool
		refNs           *string
		expectedNs      string
		expectedCrossNs bool
		expectErr       bool
	}{
		// Same-namespace references always resolve to owner namespace
		// regardless of flag state.
		{
			name:            "nil namespace, flag disabled",
			flag:            false,
			refNs:           nil,
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},
		{
			name:            "nil namespace, flag enabled",
			flag:            true,
			refNs:           nil,
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},
		{
			name:            "empty namespace, flag disabled",
			flag:            false,
			refNs:           strPtr(""),
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},
		{
			name:            "empty namespace, flag enabled",
			flag:            true,
			refNs:           strPtr(""),
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},
		{
			name:            "same namespace, flag disabled",
			flag:            false,
			refNs:           strPtr(ownerNs),
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},
		{
			name:            "same namespace, flag enabled",
			flag:            true,
			refNs:           strPtr(ownerNs),
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},

		// Cross-namespace resolves to the specified namespace with warning
		// indicator when the flag is enabled.
		{
			name:            "different namespace, flag enabled",
			flag:            true,
			refNs:           strPtr(otherNs),
			expectedNs:      otherNs,
			expectedCrossNs: true,
			expectErr:       false,
		},

		// Cross-namespace rejected with terminal error when the flag is
		// disabled.
		{
			name:            "different namespace, flag disabled",
			flag:            false,
			refNs:           strPtr(otherNs),
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, isCrossNs, err := ValidateCrossNamespaceReference(
				tt.flag, ownerNs, tt.refNs, refName,
			)
			assert.Equal(t, tt.expectedNs, ns)
			assert.Equal(t, tt.expectedCrossNs, isCrossNs)
			if tt.expectErr {
				require.Error(t, err)
				assert.True(t,
					errors.Is(err, ackerr.ResourceReferenceCrossNamespaceNotAllowed),
					"error should wrap ResourceReferenceCrossNamespaceNotAllowed sentinel",
				)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	// Rejection error carries diagnostic context.
	t.Run("error contains source namespace", func(t *testing.T) {
		_, _, err := ValidateCrossNamespaceReference(false, ownerNs, strPtr(otherNs), refName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ownerNs)
	})

	t.Run("error contains target namespace", func(t *testing.T) {
		_, _, err := ValidateCrossNamespaceReference(false, ownerNs, strPtr(otherNs), refName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), otherNs)
	})

	t.Run("error contains ref name", func(t *testing.T) {
		_, _, err := ValidateCrossNamespaceReference(false, ownerNs, strPtr(otherNs), refName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), refName)
	})

	t.Run("error contains flag name", func(t *testing.T) {
		_, _, err := ValidateCrossNamespaceReference(false, ownerNs, strPtr(otherNs), refName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "enable-cross-namespace")
	})

	t.Run("error wraps sentinel", func(t *testing.T) {
		_, _, err := ValidateCrossNamespaceReference(false, ownerNs, strPtr(otherNs), refName)
		require.Error(t, err)
		assert.True(t,
			errors.Is(err, ackerr.ResourceReferenceCrossNamespaceNotAllowed),
			"error should be unwrappable to the sentinel via errors.Is",
		)
	})
}

func TestValidateCrossNamespaceReferenceString(t *testing.T) {
	const ownerNs = "owner-ns"
	const otherNs = "other-ns"
	const refName = "my-ref"

	tests := []struct {
		name            string
		flag            bool
		refNs           string
		expectedNs      string
		expectedCrossNs bool
		expectErr       bool
	}{
		// Empty string namespace resolves to owner namespace.
		{
			name:            "empty string namespace",
			flag:            false,
			refNs:           "",
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},
		// Same namespace string resolves to owner namespace.
		{
			name:            "same namespace string",
			flag:            true,
			refNs:           ownerNs,
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       false,
		},
		// Different namespace string with flag enabled resolves to the
		// specified namespace with cross-namespace indicator.
		{
			name:            "different namespace string, flag enabled",
			flag:            true,
			refNs:           otherNs,
			expectedNs:      otherNs,
			expectedCrossNs: true,
			expectErr:       false,
		},
		// Different namespace string with flag disabled returns a terminal
		// error.
		{
			name:            "different namespace string, flag disabled",
			flag:            false,
			refNs:           otherNs,
			expectedNs:      ownerNs,
			expectedCrossNs: false,
			expectErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, isCrossNs, err := ValidateCrossNamespaceReferenceString(
				tt.flag, ownerNs, tt.refNs, refName,
			)
			assert.Equal(t, tt.expectedNs, ns)
			assert.Equal(t, tt.expectedCrossNs, isCrossNs)
			if tt.expectErr {
				require.Error(t, err)
				assert.True(t,
					errors.Is(err, ackerr.ResourceReferenceCrossNamespaceNotAllowed),
					"error should wrap ResourceReferenceCrossNamespaceNotAllowed sentinel",
				)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
