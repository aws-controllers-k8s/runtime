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
	"testing"

	"github.com/stretchr/testify/assert"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
)

func TestPathContains(t *testing.T) {
	p := ackcompare.NewPath("A.B")
	assert.True(t, p.Contains("A"))
	assert.True(t, p.Contains("A.B"))
	assert.False(t, p.Contains("A.B.C"))
	assert.False(t, p.Contains("B"))
	assert.False(t, p.Contains("A.C"))
	// Case-sensitive: a differing case does not match.
	assert.False(t, p.Contains("a.b"))
}

func TestPathContainsFold(t *testing.T) {
	p := ackcompare.NewPath("Spec.KMSKeyID")
	// Exact match.
	assert.True(t, p.ContainsFold("Spec.KMSKeyID"))
	// Case-insensitive per segment: the JSON/spec name folds onto the
	// acronym-cased Go field name.
	assert.True(t, p.ContainsFold("spec.kmsKeyID"))
	assert.True(t, p.ContainsFold("SPEC.kmskeyid"))
	// Prefix (at-or-under) matching, folded.
	assert.True(t, p.ContainsFold("spec"))
	// Longer than the path -> no match.
	assert.False(t, p.ContainsFold("spec.kmsKeyID.extra"))
	// Different field -> no match.
	assert.False(t, p.ContainsFold("spec.name"))
}
