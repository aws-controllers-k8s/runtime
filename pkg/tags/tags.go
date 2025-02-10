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

package tags

import (
	"strings"
)

// Tags represents the AWS tags which will be added to the AWS resource.
// Inside aws-sdk-go, Tags are represented using multiple types, Ex: map of
// string, list of structs etc...
// Tags type will be used as a hub/mediator to merge tags represented
// using different types.
type Tags map[string]string

// NewTags returns Tags with empty tags
func NewTags() Tags {
	return map[string]string{}
}

// Merge merges two set of Tags and returns the merge result.
// In case of collision precedence is given to tags present in the first
// parameter 'a'.
func Merge(a Tags, b Tags) Tags {
	var result Tags
	// Initialize result with the first set of tags 'a'.
	// If first set is nil, initialize result with empty set of tags.
	if a == nil {
		result = NewTags()
	} else {
		result = a
	}
	if b != nil && len(b) > 0 {
		// Add all the tags which are not already present in result
		for tk, tv := range b {
			if _, found := result[tk]; !found {
				result[tk] = tv
			}
		}
	}
	return result
}

// SyncAWSTags ensures AWS-managed tags (prefixed with "aws:") from the latest resource state
// are preserved in the desired state. This prevents the controller from attempting to
// modify AWS-managed tags, which would result in an error.
//
// AWS-managed tags are automatically added by AWS services (e.g., CloudFormation, Service Catalog)
// and cannot be modified or deleted through normal tag operations. Common examples include:
// - aws:cloudformation:stack-name
// - aws:servicecatalog:productArn
//
// Parameters:
//   - a: The target Tags map to be updated (typically desired state)
//   - b: The source Tags map containing AWS-managed tags (typically latest state)
//
// Example:
//
//	latest := Tags{"aws:cloudformation:stack-name": "my-stack", "environment": "prod"}
//	desired := Tags{"environment": "dev"}
//	SyncAWSTags(desired, latest)
//	desired now contains {"aws:cloudformation:stack-name": "my-stack", "environment": "dev"}
func SyncAWSTags(a Tags, b Tags) {
	for k := range b {
		if strings.HasPrefix(k, "aws:") {
			a[k] = b[k]
		}
	}
}
