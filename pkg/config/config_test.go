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

package config

import "testing"

func TestParseReconcileFlagArgument(t *testing.T) {
	tests := []struct {
		flagArgument   string
		expectedKey    string
		expectedVal    int
		expectedErr    bool
		expectedErrMsg string
	}{
		// Test valid flag arguments
		{"key=1", "key", 1, false, ""},
		{"key=123456", "key", 123456, false, ""},
		{"key=600", "key", 600, false, ""},
		{"k=1", "k", 1, false, ""},
		{"ke_y=123456", "ke_y", 123456, false, ""},

		// Test invalid flag arguments
		{"key", "", 0, true, "invalid flag argument format: expected key=value"},
		{"key=", "", 0, true, "missing value in flag argument"},
		{"=value", "", 0, true, "missing key in flag argument"},
		{"key=value1=value2", "", 0, true, "invalid flag argument format: expected key=value"},
		{"key=a", "", 0, true, "invalid value in flag argument: strconv.Atoi: parsing \"a\": invalid syntax"},
		{"key=-1", "", 0, true, "invalid value in flag argument: expected non-negative integer, got -1"},
		{"key=-123456", "", 0, true, "invalid value in flag argument: expected non-negative integer, got -123456"},
		{"key=1.1", "", 0, true, "invalid value in flag argument: strconv.Atoi: parsing \"1.1\": invalid syntax"},
	}
	for _, test := range tests {
		key, val, err := parseReconcileFlagArgument(test.flagArgument)
		if err != nil && !test.expectedErr {
			t.Errorf("unexpected error for flag argument '%s': %v", test.flagArgument, err)
		}
		if err == nil && test.expectedErr {
			t.Errorf("expected error for flag argument '%s', got nil", test.flagArgument)
		}
		if err != nil && err.Error() != test.expectedErrMsg {
			t.Errorf("unexpected error message for flag argument '%s': expected '%s', got '%v'", test.flagArgument, test.expectedErrMsg, err)
		}
		if key != test.expectedKey {
			t.Errorf("unexpected key for flag argument '%s': expected '%s', got '%s'", test.flagArgument, test.expectedKey, key)
		}
		if val != test.expectedVal {
			t.Errorf("unexpected value for flag argument '%s': expected %d, got %d", test.flagArgument, test.expectedVal, val)
		}
	}
}
