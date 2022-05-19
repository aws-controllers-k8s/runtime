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
	"fmt"
	"strings"

	rtclient "sigs.k8s.io/controller-runtime/pkg/client"

	ackconfig "github.com/aws-controllers-k8s/runtime/pkg/config"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

const (
	// MissingImageTagValue is the placeholder value when ACK controller
	// image tag(release semver) cannot be determined.
	MissingImageTagValue = "unknown"
)

// GetDefaultTags provides Default tags (key value pairs) for given resource
func GetDefaultTags(
	config *ackconfig.Config,
	object rtclient.Object,
	md acktypes.ServiceControllerMetadata,
) map[string]string {
	if object == nil || config == nil || len(config.ResourceTags) == 0 {
		return nil
	}
	var populatedTags = make(map[string]string)
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
		populatedValue := expandTagValue(&val, object, md)
		populatedTags[key] = *populatedValue
	}
	if len(populatedTags) == 0 {
		return nil
	}
	return populatedTags
}

func expandTagValue(
	value *string,
	obj rtclient.Object,
	md acktypes.ServiceControllerMetadata,
) *string {
	if value == nil || obj == nil {
		return nil
	}
	var expandedValue string = ""
	switch *value {
	case "%CONTROLLER_VERSION%":
		expandedValue = generateControllerVersion(md)
	case "%K8S_NAMESPACE%":
		expandedValue = obj.GetNamespace()
	default:
		expandedValue = *value
	}
	return &expandedValue
}

// generateControllerVersion creates the tag value for key
// "services.k8s.aws/controller-version". The value for this tag is in the
// format "<service-name>-<controller-image-tag>". Ex: s3-v0.0.10
func generateControllerVersion(md acktypes.ServiceControllerMetadata) string {
	controllerImageTag := md.GitVersion
	// ACK controller released from the ACK CD pipeline will have the correct
	// GitVersion. But this value can be empty when manually building ACK
	// controller image locally and not passing the go ldflags.
	// Add a placeholder value when git tag is found missing.
	if controllerImageTag == "" {
		controllerImageTag = MissingImageTagValue
	}
	return fmt.Sprintf("%s-%s", md.ServiceAlias, controllerImageTag)
}
