package runtime_test

import (
	"fmt"
	"testing"

	"github.com/aws-controllers-k8s/runtime/pkg/config"

	mocks "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	"github.com/aws-controllers-k8s/runtime/pkg/runtime"
	"github.com/aws-controllers-k8s/runtime/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestGetDefaultTags(t *testing.T) {
	assert := assert.New(t)
	obj := mocks.Object{}
	obj.On("GetNamespace").Return("ns")
	obj.On("GetName").Return("res")

	cfg := config.Config{}

	md := types.ServiceControllerMetadata{
		ServiceAlias: "s3",
		VersionInfo: types.VersionInfo{
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
			runtime.ServiceAliasTagFormat,
			runtime.ControllerVersionTagFormat,
		),
		fmt.Sprintf("services.k8s.aws/namespace=%s",
			runtime.NamespaceTagFormat,
		),
		fmt.Sprintf("services.k8s.aws/name=%s",
			runtime.ResourceNameTagFormat,
		),
	}
	expandedTags = runtime.GetDefaultTags(&cfg, &obj, md)
	assert.Equal(4, len(expandedTags))
	assert.Equal("bar", expandedTags["foo"])
	assert.Equal("s3-v0.0.10", expandedTags["services.k8s.aws/controller-version"])
	assert.Equal("ns", expandedTags["services.k8s.aws/namespace"])
	assert.Equal("res", expandedTags["services.k8s.aws/name"])
}
