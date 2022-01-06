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
	"time"

	ackconfig "github.com/aws-controllers-k8s/runtime/pkg/config"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// GetDefaultTags provides Default tags (key value pairs) for given resource
func GetDefaultTags(
	config *ackconfig.Config,
	object rtclient.Object,
) map[string]string {
	if object == nil || config == nil || len(config.ResourceTags) == 0 {
		return nil
	}
	var populatedTags = make(map[string]string)
	for _, tagKeyVal := range config.ResourceTags {
		keyVal := strings.Split(tagKeyVal, "=")
		if keyVal == nil && len(keyVal) != 2 {
			continue
		}
		key := strings.TrimSpace(keyVal[0])
		val := strings.TrimSpace(keyVal[1])
		if key == "" || val == "" {
			continue
		}
		populatedValue := expandTagValue(&val, object)
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
) *string {
	if value == nil || obj == nil {
		return nil
	}
	var expandedValue string = ""
	switch *value {
	case "%UTCNOW%":
		expandedValue = time.Now().UTC().String()
	case "%KUBERNETES_NAMESPACE%":
		expandedValue = obj.GetNamespace()
	default:
		expandedValue = *value
	}
	return &expandedValue
}
