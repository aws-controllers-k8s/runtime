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

package runtime_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	mocks "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	"github.com/aws-controllers-k8s/runtime/pkg/config"
	"github.com/aws-controllers-k8s/runtime/pkg/runtime"
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

func TestGetDefaultTags(t *testing.T) {
	assert := assert.New(t)
	obj := mocks.Object{}
	obj.On("GetNamespace").Return("ns")
	obj.On("GetName").Return("res")

	cfg := config.Config{}

	md := acktypes.ServiceControllerMetadata{
		ServiceAlias: "s3",
		VersionInfo: acktypes.VersionInfo{
			GitVersion: "v0.0.10",
		},
	}

	// nil config
	assert.Nil(runtime.GetDefaultTags(nil, &obj, md))

	// nil object
	assert.Nil(runtime.GetDefaultTags(&cfg, nil, md))

	// no resource tags
	assert.Nil(runtime.GetDefaultTags(&cfg, &obj, md))

	// ill formed tags
	cfg.ResourceTags = []string{"foobar"}
	expandedTags := runtime.GetDefaultTags(&cfg, &obj, md)
	assert.Empty(expandedTags)

	// ill formed tags
	cfg.ResourceTags = []string{"foo=bar=baz"}
	expandedTags = runtime.GetDefaultTags(&cfg, &obj, md)
	assert.Empty(expandedTags)

	// tags without any ack resource tag format
	cfg.ResourceTags = []string{"foo=bar"}
	expandedTags = runtime.GetDefaultTags(&cfg, &obj, md)
	assert.Equal(1, len(expandedTags))
	assert.Equal("bar", expandedTags["foo"])

	// expand ack resource tag formats
	cfg.ResourceTags = []string{
		"foo=bar",
		fmt.Sprintf("services.k8s.aws/controller-version=%s-%s",
			acktags.ServiceAliasTagFormat,
			acktags.ControllerVersionTagFormat,
		),
		fmt.Sprintf("services.k8s.aws/namespace=%s",
			acktags.NamespaceTagFormat,
		),
		fmt.Sprintf("services.k8s.aws/name=%s",
			acktags.ResourceNameTagFormat,
		),
	}
	expandedTags = runtime.GetDefaultTags(&cfg, &obj, md)
	assert.Equal(4, len(expandedTags))
	assert.Equal("bar", expandedTags["foo"])
	assert.Equal("s3-v0.0.10", expandedTags["services.k8s.aws/controller-version"])
	assert.Equal("ns", expandedTags["services.k8s.aws/namespace"])
	assert.Equal("res", expandedTags["services.k8s.aws/name"])
}
