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

func TestNewResourceTags(t *testing.T) {
	assert := assert.New(t)
	rt := acktags.NewResourceTags()
	assert.NotNil(rt)
	assert.Empty(rt.Tags)

	tags := map[string]string{"tk": "tv"}
	rt = acktags.NewResourceTagsFrom(tags)
	assert.NotNil(rt)
	assert.NotEmpty(rt.Tags)
	assert.Equal("tv", rt.Tags["tk"])
}

func TestNewResourceTagsFrom(t *testing.T) {
	assert := assert.New(t)

	tags := map[string]string{"tk": "tv"}
	rt := acktags.NewResourceTagsFrom(tags)
	assert.NotNil(rt)
	assert.NotEmpty(rt.Tags)
	assert.Equal("tv", rt.Tags["tk"])
}

func TestResourceTags_Merge_NilTags(t *testing.T) {
	assert := assert.New(t)

	rt1 := acktags.NewResourceTagsFrom(nil)
	rt2 := acktags.NewResourceTagsFrom(map[string]string{"tk": "tv", "tk2": "tv2"})
	rt1.Merge(rt2)
	assert.Equal("tv", rt1.Tags["tk"])
	assert.Equal("tv2", rt1.Tags["tk2"])
	assert.Equal(2, len(rt1.Tags))
}

func TestResourceTags_Merge(t *testing.T) {
	assert := assert.New(t)

	rt1 := acktags.NewResourceTagsFrom(map[string]string{"tk": "tv"})
	rt2 := acktags.NewResourceTagsFrom(map[string]string{"tk": "tv1", "tk2": "tv2"})
	rt1.Merge(rt2)
	assert.Equal("tv", rt1.Tags["tk"])
	assert.Equal("tv2", rt1.Tags["tk2"])
	assert.Equal(2, len(rt1.Tags))
}
