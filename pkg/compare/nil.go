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
	"reflect"
)

// HasNilDifference returns true if the supplied subjects' nilness is
// different
func HasNilDifference(a, b interface{}) bool {
	if IsNil(a) || IsNil(b) {
		if (IsNil(a) && IsNotNil(b)) || (IsNil(b) && IsNotNil(a)) {
			return true
		}
	}
	return false
}

// IsNil checks the passed interface argument for Nil value.
// For interfaces, only 'i==nil' check is not sufficient.
// https://tour.golang.org/methods/12
// More details: https://mangatmodi.medium.com/go-check-nil-interface-the-right-way-d142776edef1
func IsNil(i interface{}) bool {
	if i == nil {
		return true
	}

	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
		return reflect.ValueOf(i).IsNil()
	}
	return false
}

func IsNotNil(i interface{}) bool {
	return !IsNil(i)
}
