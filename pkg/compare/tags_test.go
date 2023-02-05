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
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
)

func TestTagsDifference(t *testing.T) {
	require := require.New(t)

	a := acktags.NewTags()
	b := acktags.NewTags()

	a["tag1"] = "value1"

	a["tag2"] = "value2"
	b["tag2"] = "value2"

	b["tag3"] = "value3"
	b["tag4"] = "value4"

	added, unchanged, removed := compare.GetTagsDifference(a, b)

	require.Len(added, 2)
	require.Len(unchanged, 1)
	require.Len(removed, 1)

	require.Contains(added, "tag3")
	require.Contains(added, "tag4")
	require.Contains(unchanged, "tag2")
	require.Contains(removed, "tag1")

	// add test for updating the value of an existing tag
	a["tag2"] = "oldvalue"
	b["tag2"] = "newvalue"

	added, unchanged, removed = compare.GetTagsDifference(a, b)

	require.Len(added, 3)
	require.Len(unchanged, 0)
	require.Len(removed, 2)

	require.Contains(added, "tag2")
	require.Equal(added["tag2"], "newvalue")
}
