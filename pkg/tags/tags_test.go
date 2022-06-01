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

package tags_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
)

func TestNewTags(t *testing.T) {
	assert := assert.New(t)
	tags := acktags.NewTags()
	assert.NotNil(tags)
	assert.Empty(tags)
}

func TestResourceTags_Merge_NilTags(t *testing.T) {
	assert := assert.New(t)

	var t1, t2 acktags.Tags
	res := acktags.Merge(t1, t2)
	assert.NotNil(res)
	assert.Empty(res)

	t2 = acktags.Tags{"tk": "tv", "tk2": "tv2"}
	res = acktags.Merge(t1, t2)
	assert.Equal("tv", res["tk"])
	assert.Equal("tv2", res["tk2"])
	assert.Equal(2, len(res))
}

func TestResourceTags_Merge(t *testing.T) {
	assert := assert.New(t)

	t1 := acktags.Tags{"tk": "tv"}
	t2 := acktags.Tags{"tk": "tv1", "tk2": "tv2"}
	res := acktags.Merge(t1, t2)
	assert.Equal("tv", res["tk"])
	assert.Equal("tv2", res["tk2"])
	assert.Equal(2, len(res))
}
