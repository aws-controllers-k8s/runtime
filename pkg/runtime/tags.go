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

package runtime

import (
	"strings"

	rtclient "sigs.k8s.io/controller-runtime/pkg/client"

	ackconfig "github.com/aws-controllers-k8s/runtime/pkg/config"
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// resolveTagFormat is a function which returns the resolved value for an
// ACK resource tag format. Ex: %CONTROLLER_SERVICE% -> s3
type resolveTagFormat func(rtclient.Object, acktypes.ServiceControllerMetadata) string

const (
	// MissingImageTagValue is the placeholder value when ACK controller
	// image tag(release semver) cannot be determined.
	MissingImageTagValue = "unknown"
)

// ACKResourceTagFormats is map of ACK resource tag formats to it's
// resolveTagFormat function.
//
// To add a new ACKResourceTag format, include it in this map, along with the
// resolveTagFormat function and expandTagValue() method will start
// expanding the new resource tag format.
var ACKResourceTagFormats = map[string]resolveTagFormat{
	acktags.ServiceAliasTagFormat: func(
		obj rtclient.Object,
		md acktypes.ServiceControllerMetadata,
	) string {
		return md.ServiceAlias
	},

	acktags.ControllerVersionTagFormat: func(
		obj rtclient.Object,
		md acktypes.ServiceControllerMetadata,
	) string {
		controllerImageTag := md.GitVersion
		// ACK controller released from the ACK CD pipeline will have the correct
		// GitVersion. But this value can be empty when manually building ACK
		// controller image locally and not passing the go ldflags.
		// Add a placeholder value when git tag is found missing.
		if controllerImageTag == "" {
			controllerImageTag = MissingImageTagValue
		}
		return controllerImageTag
	},

	acktags.NamespaceTagFormat: func(
		obj rtclient.Object,
		md acktypes.ServiceControllerMetadata,
	) string {
		return obj.GetNamespace()
	},

	acktags.ResourceNameTagFormat: func(
		obj rtclient.Object,
		md acktypes.ServiceControllerMetadata,
	) string {
		return obj.GetName()
	},
}

// GetDefaultTags provides Default tags (key value pairs) for given resource
func GetDefaultTags(
	config *ackconfig.Config,
	obj rtclient.Object,
	md acktypes.ServiceControllerMetadata,
) acktags.Tags {
	defaultTags := acktags.NewTags()
	if obj == nil || config == nil || len(config.ResourceTags) == 0 {
		return defaultTags
	}
	for _, tagKeyVal := range config.ResourceTags {
		keyVal := strings.Split(tagKeyVal, "=")
		if keyVal == nil || len(keyVal) != 2 {
			continue
		}
		key := strings.TrimSpace(keyVal[0])
		val := strings.TrimSpace(keyVal[1])
		if key == "" || val == "" {
			continue
		}
		defaultTags[key] = expandTagValue(val, obj, md)
	}
	return defaultTags
}

// expandTagValue returns the tag value after expanding all the ACKResourceTag
// formats.
func expandTagValue(
	value string,
	obj rtclient.Object,
	md acktypes.ServiceControllerMetadata,
) string {
	for tagFormat, resolveTagFormat := range ACKResourceTagFormats {
		value = strings.ReplaceAll(value, tagFormat, resolveTagFormat(obj, md))
	}
	return value
}
