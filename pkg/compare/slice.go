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

import "sort"

// SliceStringPEqual returns true if the supplied slices of string pointers
// have equal values regardless of order.
func SliceStringPEqual(a, b []*string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := make([]string, len(a))
	sb := make([]string, len(a))
	for x, aPtr := range a {
		sa[x] = *aPtr
		sb[x] = *b[x]
	}
	sort.Strings(sa)
	sort.Strings(sb)
	return sortedStringSliceEqual(sa, sb)
}

// SliceStringEqual returns true if the supplied slices of string
// have equal values regardless of order.
func SliceStringEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := make([]string, len(a))
	bCopy := make([]string, len(a))
	for x, aVal := range a {
		aCopy[x] = aVal
		bCopy[x] = b[x]
	}
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	return sortedStringSliceEqual(aCopy, bCopy)
}

// sortedStringSliceEqual returns true if the supplied sorted slices of string
// have equal values considering the order. It is assumed the size is same for
// both slices.
func sortedStringSliceEqual(a, b []string) bool {
	for x, aVal := range a {
		bVal := b[x]
		if aVal != bVal {
			return false
		}
	}
	return true
}

// SliceStringPDifference compares a desired slice of string pointers against an
// observed slice and returns which elements need to be added and which need to
// be removed to make observed match desired. Order is ignored.
func SliceStringPDifference(desired, observed []*string) (toAdd, toRemove []*string) {
	observedSet := make(map[string]bool, len(observed))
	for _, item := range observed {
		if item != nil {
			observedSet[*item] = true
		}
	}
	desiredSet := make(map[string]bool, len(desired))
	for _, item := range desired {
		if item != nil {
			desiredSet[*item] = true
			if !observedSet[*item] {
				toAdd = append(toAdd, item)
			}
		}
	}
	for _, item := range observed {
		if item != nil && !desiredSet[*item] {
			toRemove = append(toRemove, item)
		}
	}
	return toAdd, toRemove
}
