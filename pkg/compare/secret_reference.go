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
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// SecretKeyReferenceEqual returns true if the supplied secret key references
// are equal.
func SecretKeyReferenceEqual(a, b *ackv1alpha1.SecretKeyReference) bool {
	if HasNilDifference(a, b) {
		return false
	}
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Key == b.Key
}

// SliceSecretKeyReferenceEqual returns true if the supplied slices of secret
// key reference pointers contain the exact same elements.
func SliceSecretKeyReferenceEqual(
	a, b []*ackv1alpha1.SecretKeyReference,
) bool {
	equal, _, _ := CompareSecretKeyReferences(a, b)
	return equal
}

// CompareSecretKeyReference returns true if the supplied slices of secret
// key reference pointers contain the exact same elements. It will also return 2
// slices containing elements contained in a that aren't in b, and elements
// contained in b that aren't in a, respectively. The comparison doesn't
// take into consideration the order of elements.
// Duplicated elements will not impact the behaviour of this function, meaning
// that the function will always return only unique instances of the added/removed
// elements.
func CompareSecretKeyReferences(
	a, b []*ackv1alpha1.SecretKeyReference,
) (equal bool, added, removed []*ackv1alpha1.SecretKeyReference) {
	// finding the removed elements
	for _, aVal := range a {
		found := false
		for _, bVal := range b {
			if SecretKeyReferenceEqual(aVal, bVal) {
				found = true
				break
			}
		}
		if !found {
			removed = append(removed, aVal)
			continue
		}
	}
	// finding the added elements
	for _, bVal := range b {
		found := false
		for _, aVal := range a {
			if SecretKeyReferenceEqual(aVal, bVal) {
				found = true
				break
			}
		}
		if !found {
			added = append(added, bVal)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		equal = true
	}
	return equal, cleanUpDuplicateSecretReferences(added), cleanUpDuplicateSecretReferences(removed)
}

// cleanUpDuplicateSecretReferences removes duplicate elements from a given slice
// of secret key references.
func cleanUpDuplicateSecretReferences(
	a []*ackv1alpha1.SecretKeyReference,
) (uniqueSecretReferences []*ackv1alpha1.SecretKeyReference) {
	for i := range a {
		foundDuplicate := false
		// just walk the array from index i and do not append anything if a
		// duplicate is found.
		for j := i + 1; j < len(a); j++ {
			if SecretKeyReferenceEqual(a[i], a[j]) {
				foundDuplicate = true
				break
			}
		}
		if !foundDuplicate {
			uniqueSecretReferences = append(uniqueSecretReferences, a[i])
		}
	}
	return uniqueSecretReferences
}
