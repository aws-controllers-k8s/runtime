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

// ResourceTags represents the AWS tags which will be added to the AWS resource.
// Inside aws-sdk-go, Tags are represented using multiple types, Ex: map of
// string, list of structs etc...
// ResourceTags type will be used as a hub/mediator to merge tags represented
// using different types.
type ResourceTags struct {
	Tags map[string]string
}

// NewResourceTags returns ResourceTags with empty tags
func NewResourceTags() ResourceTags {
	return ResourceTags{make(map[string]string)}
}

// NewResourceTagsFrom creates ResourceTags using the tags passed
// inside the parameter
func NewResourceTagsFrom(tags map[string]string) ResourceTags {
	return ResourceTags{tags}
}

// Merge merges the tags between two ResourceTags.
// If a tag-key is already present inside the original ResourceTag('rt'), it
// does not get overwritten.
func (rt *ResourceTags) Merge(other ResourceTags) {
	if other.Tags != nil && len(other.Tags) > 0 {
		// Initialize if the Tags field is nil
		if rt.Tags == nil {
			rt.Tags = make(map[string]string)
		}
		// Add all the tags which are not already present
		for tk, tv := range other.Tags {
			if _, found := rt.Tags[tk]; !found {
				rt.Tags[tk] = tv
			}
		}
	}
}
