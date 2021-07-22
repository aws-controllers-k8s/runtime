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

func TestMapStringStringPEqual(t *testing.T) {
	require := require.New(t)

	aStr := "a"
	bStr := "b"

	empty := map[string]*string{}
	a := map[string]*string{
		"a": &aStr,
	}
	ac := map[string]*string{
		"a": &aStr,
	}
	b := map[string]*string{
		"b": &bStr,
	}
	ab := map[string]*string{
		"a": &aStr, "b": &bStr,
	}
	abc := map[string]*string{
		"a": &aStr, "b": &bStr,
	}
	ba := map[string]*string{
		"b": &bStr, "a": &aStr,
	}

	require.False(compare.MapStringStringPEqual(a, empty))
	require.False(compare.MapStringStringPEqual(empty, a))
	require.False(compare.MapStringStringPEqual(a, b))
	require.False(compare.MapStringStringPEqual(b, a))
	require.True(compare.MapStringStringPEqual(a, ac))
	require.True(compare.MapStringStringPEqual(ab, ba))
	require.True(compare.MapStringStringPEqual(ab, abc))
}

func TestMapStringStringEqual(t *testing.T) {
	require := require.New(t)

	aStr := "a"
	bStr := "b"

	empty := map[string]string{}
	a := map[string]string{
		"a": aStr,
	}
	ac := map[string]string{
		"a": aStr,
	}
	b := map[string]string{
		"b": bStr,
	}
	ab := map[string]string{
		"a": aStr, "b": bStr,
	}
	abc := map[string]string{
		"a": aStr, "b": bStr,
	}
	ba := map[string]string{
		"b": bStr, "a": aStr,
	}

	require.False(compare.MapStringStringEqual(a, empty))
	require.False(compare.MapStringStringEqual(empty, a))
	require.False(compare.MapStringStringEqual(a, nil))
	require.False(compare.MapStringStringEqual(nil, a))
	require.True(compare.MapStringStringEqual(nil, nil))
	require.False(compare.MapStringStringEqual(a, b))
	require.False(compare.MapStringStringEqual(b, a))
	require.True(compare.MapStringStringEqual(a, ac))
	require.True(compare.MapStringStringEqual(ab, ba))
	require.True(compare.MapStringStringEqual(ab, abc))
}
