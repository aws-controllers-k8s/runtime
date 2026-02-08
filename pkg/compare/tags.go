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

package compare

import (
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
)

// GetTagsDifference determines which tags have been added, unchanged or have
// been removed between the `from` and the `to` inputs. Tags that are in `from`
// but not in `to` are added to `removed`, whereas tags that are in `to` but not
// in `from` are added to `added`. Tags whose value has changed between `from`
// and `to` are added to `added` only (not `removed`), since tag APIs use
// upsert semantics where adding a tag with an existing key overwrites the value.
func GetTagsDifference(from, to acktags.Tags) (added, unchanged, removed acktags.Tags) {
	added = acktags.NewTags()
	unchanged = acktags.NewTags()
	removed = acktags.NewTags()

	for key, fromVal := range from {
		toVal, existsInTo := to[key]
		if !existsInTo {
			removed[key] = fromVal
		} else if fromVal == toVal {
			unchanged[key] = fromVal
		}
	}

	for key, toVal := range to {
		fromVal, existsInFrom := from[key]
		if !existsInFrom || fromVal != toVal {
			added[key] = toVal
		}
	}

	return added, unchanged, removed
}
