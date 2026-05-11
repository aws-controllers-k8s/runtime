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
	"testing"
)

func TestDocumentEqual(t *testing.T) {
	cases := []struct {
		name    string
		a       string
		b       string
		want    bool
		wantErr bool
	}{
		{
			name: "IdenticalStrings",
			a:    `{"key": "value"}`,
			b:    `{"key": "value"}`,
			want: true,
		},
		{
			name: "JSONWhitespaceDifference",
			a:    `{"key":"value","count":5}`,
			b: `{
				"key": "value",
				"count": 5
			}`,
			want: true,
		},
		{
			name: "JSONKeyOrderDifference",
			a:    `{"a": 1, "b": 2}`,
			b:    `{"b": 2, "a": 1}`,
			want: true,
		},
		{
			name: "JSONDifferentValues",
			a:    `{"key": "value1"}`,
			b:    `{"key": "value2"}`,
			want: false,
		},
		{
			name: "YAMLEquivalent",
			a: `key: value
count: 5`,
			b: `count: 5
key: value`,
			want: true,
		},
		{
			name: "YAMLvsJSON",
			a:    `{"key": "value", "count": 5}`,
			b: `key: value
count: 5`,
			want: true,
		},
		{
			name: "JSONvsYAMLNested",
			a:    `{"parent": {"child": "value"}}`,
			b: `parent:
  child: value`,
			want: true,
		},
		{
			name: "TrailingNewline",
			a:    `{"key": "value"}`,
			b:    "{\"key\": \"value\"}\n",
			want: true,
		},
		{
			name: "ArrayOrderMatters",
			a:    `{"items": ["a", "b"]}`,
			b:    `{"items": ["b", "a"]}`,
			want: false,
		},
		{
			name: "NestedObjects",
			a:    `{"outer": {"inner": {"deep": true}}}`,
			b: `outer:
  inner:
    deep: true`,
			want: true,
		},
		{
			name: "NumberTypes",
			a:    `{"count": 5}`,
			b:    `count: 5`,
			want: true,
		},
		{
			name: "BooleanTypes",
			a:    `{"enabled": true}`,
			b:    `enabled: true`,
			want: true,
		},
		{
			name: "EmptyStrings",
			a:    "",
			b:    "",
			want: true,
		},
		{
			name: "NullJSON",
			a:    "null",
			b:    "null",
			want: true,
		},
		{
			name: "ComplexAddonConfig",
			a: `{
				"notificationOrigin": ["PRODUCT"],
				"retryPolicy": {
					"minDelayTarget": 20,
					"maxDelayTarget": 20
				}
			}`,
			b:    `{"notificationOrigin":["PRODUCT"],"retryPolicy":{"minDelayTarget":20,"maxDelayTarget":20}}`,
			want: true,
		},
		{
			name: "TrailingNewline_Issue2869",
			a:    "{\n  \"secrets-store-csi-driver\": {\n    \"syncSecret\": {\n      \"enabled\": true\n    }\n  }\n}\n",
			b:    "{\n  \"secrets-store-csi-driver\": {\n    \"syncSecret\": {\n      \"enabled\": true\n    }\n  }\n}",
			want: true,
		},
		{
			name: "PrettyVsCompact_Issue2877",
			a:    "{\n  \"notificationOrigin\": [\n    \"PRODUCT\"\n  ],\n  \"productType\": [\n    {\n      \"anything-but\": \"simple\"\n    }\n  ]\n}\n",
			b:    `{"notificationOrigin":["PRODUCT"],"productType":[{"anything-but":"simple"}]}`,
			want: true,
		},
		{
			name: "NestedDifferentKeyOrder",
			a:    `{"outer":{"z":26,"a":1},"list":[1,2,3]}`,
			b:    `{"list":[1,2,3],"outer":{"a":1,"z":26}}`,
			want: true,
		},
		{
			name: "DifferentStructure",
			a:    `{"key":"value"}`,
			b:    `{"key":{"nested":"value"}}`,
			want: false,
		},
		{
			name: "ExtraKey",
			a:    `{"a":1}`,
			b:    `{"a":1,"b":2}`,
			want: false,
		},
		{
			name: "BothEmptyObjects",
			a:    `{}`,
			b:    `{}`,
			want: true,
		},
		{
			name: "EmptyVsNonEmpty",
			a:    `{}`,
			b:    `{"key":"value"}`,
			want: false,
		},
		{
			name: "NullValues",
			a:    `{"key":null}`,
			b:    `{"key":null}`,
			want: true,
		},
		{
			name: "NullVsMissing",
			a:    `{"key":null}`,
			b:    `{}`,
			want: false,
		},
		{
			name: "ComplexNestedStructure",
			a: `{
				"secrets-store-csi-driver": {
					"syncSecret": {
						"enabled": true
					},
					"providers": ["aws", "azure"]
				}
			}`,
			b:    `{"secrets-store-csi-driver":{"syncSecret":{"enabled":true},"providers":["aws","azure"]}}`,
			want: true,
		},
		{
			name:    "InvalidFirstDocument",
			a:       `{invalid json`,
			b:       `{"key": "value"}`,
			wantErr: true,
		},
		{
			name:    "InvalidSecondDocument",
			a:       `{"key": "value"}`,
			b:       `{invalid json`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DocumentEqual(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("DocumentEqual(%q, %q) expected error, got nil", tc.a, tc.b)
				}
				return
			}
			if err != nil {
				t.Errorf("DocumentEqual(%q, %q) unexpected error: %v", tc.a, tc.b, err)
				return
			}
			if got != tc.want {
				t.Errorf("DocumentEqual(%q, %q) = %t, want %t", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
