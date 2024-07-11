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

import (
	"reflect"
	"strings"
	"testing"
)

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
		{"key=-1", "", 0, true, "invalid value in flag argument: value must be greater than 0"},
		{"key=-123456", "", 0, true, "invalid value in flag argument: value must be greater than 0"},
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

const (
	dns1123SubdomainErrorMsg string = "a lowercase RFC 1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"
)

func TestParseNamespace(t *testing.T) {
	tests := []struct {
		name                 string
		watchNamespaceString string
		expectedNamespaces   []string
		expectedErr          bool
		expectedErrMsg       string
	}{
		{"empty namespace", "", nil, false, ""},
		{"default namespace", "default", []string{"default"}, false, ""},
		{"two valid namespaces", "default,foo", []string{"default", "foo"}, false, ""},
		{"three valid namespaces", "default,foo,bar", []string{"default", "foo", "bar"}, false, ""},
		{"multiple empty namespace", ",,,,,", nil, true, "invalid namespace: empty namespace"},
		{"duplicate namespace", "foo,bar,bar", nil, true, "duplicate namespace 'bar'"},
		{"valid namespaces and one empty namespace", "default,foo,", nil, true, "invalid namespace: empty namespace"},
		{"last namespace is invalid", "default,foo,---", nil, true, dns1123SubdomainErrorMsg},
		{"non RFC 1123 label compliant namespace - dot", "foo.bar", nil, true, "must not contain dots"},
		{"non RFC 1123 label compliant namespace - underscore", "foo_bar", nil, true, dns1123SubdomainErrorMsg},
	}
	for _, test := range tests {
		namespaces, err := parseWatchNamespaceString(test.watchNamespaceString)
		if err != nil && !test.expectedErr {
			t.Errorf("unexpected error for namespace '%s': %v", test.watchNamespaceString, err)
		}
		if err == nil && test.expectedErr {
			t.Errorf("expected error for namespace '%s', got nil", test.watchNamespaceString)
		}
		if err != nil && !strings.Contains(err.Error(), test.expectedErrMsg) {
			t.Errorf("unexpected error message for namespace '%s': expected '%s', got '%v'", test.watchNamespaceString, test.expectedErrMsg, err)
		}
		if len(namespaces) != len(test.expectedNamespaces) {
			t.Errorf("unexpected number of namespaces for namespace '%s': expected %d, got %d", test.watchNamespaceString, len(test.expectedNamespaces), len(namespaces))
		}
		for i, ns := range namespaces {
			if ns != test.expectedNamespaces[i] {
				t.Errorf("unexpected namespace for namespace '%s': expected '%s', got '%s'", test.watchNamespaceString, test.expectedNamespaces[i], ns)
			}
		}
	}
}

func TestParseFeatureGates(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]bool
		wantErr bool
	}{
		{
			name:  "Empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "Single feature enabled",
			input: "Feature1=true",
			want:  map[string]bool{"Feature1": true},
		},
		{
			name:  "Single feature disabled",
			input: "Feature1=false",
			want:  map[string]bool{"Feature1": false},
		},
		{
			name:  "Multiple features",
			input: "Feature1=true,Feature2=false,Feature3=true",
			want: map[string]bool{
				"Feature1": true,
				"Feature2": false,
				"Feature3": true,
			},
		},
		{
			name:  "Whitespace in input",
			input: " Feature1 = true , Feature2 = false ",
			want: map[string]bool{
				"Feature1": true,
				"Feature2": false,
			},
		},
		{
			name:    "Invalid format",
			input:   "Feature1:true",
			wantErr: true,
		},
		{
			name:    "Invalid boolean value",
			input:   "Feature1=yes",
			wantErr: true,
		},
		{
			name:    "Missing value",
			input:   "Feature1=",
			wantErr: true,
		},
		{
			name:    "Missing key",
			input:   "=true",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFeatureGates(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFeatureGates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFeatureGates() = %v, want %v", got, tt.want)
			}
		})
	}
}
