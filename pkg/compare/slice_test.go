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

	"github.com/stretchr/testify/require"

	"github.com/aws-controllers-k8s/runtime/pkg/compare"
)

func TestSliceStringPEqual(t *testing.T) {
	require := require.New(t)

	aStr := "a"
	bStr := "b"

	empty := []*string{}
	a := []*string{
		&aStr,
	}
	ac := []*string{
		&aStr,
	}
	b := []*string{
		&bStr,
	}
	ab := []*string{
		&aStr, &bStr,
	}
	abc := []*string{
		&aStr, &bStr,
	}
	ba := []*string{
		&bStr, &aStr,
	}
	aab := []*string{
		&aStr, &aStr, &bStr,
	}
	aba := []*string{
		&aStr, &bStr, &aStr,
	}
	bba := []*string{
		&bStr, &bStr, &aStr,
	}

	require.False(compare.SliceStringPEqual(a, empty))
	require.False(compare.SliceStringPEqual(empty, a))
	require.False(compare.SliceStringPEqual(a, b))
	require.False(compare.SliceStringPEqual(b, a))
	require.True(compare.SliceStringPEqual(a, ac))
	require.True(compare.SliceStringPEqual(ab, ba))
	require.True(compare.SliceStringPEqual(ab, abc))
	require.True(compare.SliceStringPEqual(aab, aba))
	require.False(compare.SliceStringPEqual(aab, bba))
}
