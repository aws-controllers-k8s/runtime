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

// Delta represents differences between two AWSResources. The
// underlying types of the two supplied AWSResources should be the same. In
// other words, the Delta() method should be called with the same concrete
// implementing AWSResource type
type Delta struct {
	// Differences is a slice of *ackcompare.Difference structs representing
	// differences in values of two resources under comparison
	Differences []*Difference
}

// DifferentAt returns whether there is a difference at the supplied JSONPath
// expression in the resources under comparison
func (d *Delta) DifferentAt(subject string) bool {
	for _, diff := range d.Differences {
		if diff.Path.Contains(subject) {
			return true
		}
	}
	return false
}

// DifferentExcept returns true if the delta contains any differences *other*
// than any of the supplied path strings.
//
// This method is useful if you have a scenario where you don't want to proceed
// with a modification action if certain fields in a resource have not been
// changed.
//
// For example, consider this code:
//
//	if delta.DifferentAt("Spec.Tags") {
//	    if err = rm.SyncTags(ctx, desired, latest); err != nil {
//	        return nil, err
//	    }
//	}
//
//	if !delta.DifferentExcept("Spec.Tags") {
//	    // We don't want to proceed to call the ModifyDBInstance API since
//	    // no other resource fields have changed.
//	    return desired, nil
//	}
//
// might be placed in an sdk_update_pre_build_request custom code hook to
// prevent the ModifyDBInstance call from being executed if the DBInstance's
// Tags field is the only field with changes.
//
// Each difference is checked on its own, so several differences under one
// except path (for example two changed members of an excepted struct) still
// count as excepted.
func (d *Delta) DifferentExcept(
	exceptPaths ...string,
) bool {
	for _, diff := range d.Differences {
		if !diffMatchesAny(diff, exceptPaths) {
			return true
		}
	}
	return false
}

// diffMatchesAny returns true if the difference's path is at or under any of
// the supplied paths.
func diffMatchesAny(diff *Difference, paths []string) bool {
	for _, p := range paths {
		if diff.Path.Contains(p) {
			return true
		}
	}
	return false
}

// Add adds a new Difference to the Delta
func (d *Delta) Add(
	path string,
	a interface{},
	b interface{},
) {
	d.Differences = append(
		d.Differences,
		&Difference{NewPath(path), a, b},
	)
}

// NewDelta returns a new Delta struct used to compare two resources.
func NewDelta() *Delta {
	return &Delta{
		Differences: []*Difference{},
	}
}
