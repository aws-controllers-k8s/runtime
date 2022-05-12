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

type Foo struct {
	Bar string
	Baz Baz
}

type Baz struct {
	Y string
}

func TestDifferentAt(t *testing.T) {
	require := require.New(t)

	a := Foo{
		Bar: "a_bar",
		Baz: Baz{
			Y: "a_baz_y",
		},
	}
	b := Foo{
		Bar: "b_bar",
		Baz: Baz{
			Y: "b_baz_y",
		},
	}

	d := compare.NewDelta()
	d.Add("", a, nil)
	require.True(d.DifferentAt(""))

	d = compare.NewDelta()
	d.Add("Bar", a.Bar, b.Bar)
	require.True(d.DifferentAt("Bar"))
	require.False(d.DifferentAt("Baz")) // diff exists but was not added to Delta

	d = compare.NewDelta()
	d.Add("Baz.Y", a.Baz.Y, b.Baz.Y)
	require.True(d.DifferentAt("Baz"))
	require.True(d.DifferentAt("Baz.Y"))
	require.False(d.DifferentAt("Y"))       // there is no diff for top-level field "Y"
	require.False(d.DifferentAt("Bar"))     // diff exists but it was not added to Delta
	require.False(d.DifferentAt("Baz.Y.Z")) // subject length exceeds length of diff Path
	require.False(d.DifferentAt("Baz.Z"))   // matches Path top-level field but not sub-field
}

func TestDifferentExcept(t *testing.T) {
	require := require.New(t)

	a := Foo{
		Bar: "a_bar",
		Baz: Baz{
			Y: "a_baz_y",
		},
	}
	b := Foo{
		Bar: "b_bar",
		Baz: Baz{
			Y: "b_baz_y",
		},
	}

	d := compare.NewDelta()
	d.Add("", a, nil)
	require.False(d.DifferentExcept(""))

	d = compare.NewDelta()
	d.Add("Baz.Y", a.Baz.Y, b.Baz.Y)
	require.True(d.DifferentExcept("Bar"))    // there is a difference that is *not* Bar
	require.False(d.DifferentExcept("Baz.Y")) // there is *not* a different that is *not* Bar
}
