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
	"github.com/samber/lo"

	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
)

// GetTagsDifference determines which tags have been added, unchanged or have
// been removed between the `from` and the `to` inputs. Tags that are in `from`
// but not in `to` are added to `removed`, whereas tags that are in `to` but not
// in `from` are added to `added`.
func GetTagsDifference(from, to acktags.Tags) (added, unchanged, removed acktags.Tags) {
	// we need to convert the tag tuples to a comparable interface type
	fromPairs := lo.ToPairs(from)
	toPairs := lo.ToPairs(to)

	left, right := lo.Difference(fromPairs, toPairs)
	middle := lo.Intersect(fromPairs, toPairs)

	removed = lo.FromPairs(left)
	added = lo.FromPairs(right)
	unchanged = lo.FromPairs(middle)

	return added, unchanged, removed
}
