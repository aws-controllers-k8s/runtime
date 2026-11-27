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

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	k8scache "k8s.io/client-go/tools/cache"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// A selector deleted while the watch is broken reaches the handler as a
// DeletedFinalStateUnknown tombstone, with or without its last known state.
// The cache removes it, instead of panicking on the type assertion.
func TestCache_DeleteTombstone(t *testing.T) {
	cache := NewCache(zapr.NewLogger(zap.NewNop()))
	for _, name := range []string{"with-object", "key-only"} {
		cache.handleAdd(createSelector(name, ackv1alpha1.IAMRoleSelector{
			Spec: ackv1alpha1.IAMRoleSelectorSpec{ARN: "arn:aws:iam::123456789012:role/team"},
		}))
	}
	require.Len(t, cache.ListSelectors(), 2)

	cache.handleDelete(k8scache.DeletedFinalStateUnknown{
		Key: "with-object",
		Obj: createSelector("with-object", ackv1alpha1.IAMRoleSelector{}),
	})
	cache.handleDelete(k8scache.DeletedFinalStateUnknown{Key: "key-only"})
	require.Empty(t, cache.ListSelectors())
}
