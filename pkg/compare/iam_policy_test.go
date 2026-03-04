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

func TestIAMPolicyDocumentEqual(t *testing.T) {
	cases := []struct {
		name    string
		a       string
		b       string
		want    bool
		wantErr bool
	}{
		{
			name: "IdenticalStrings",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			b:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			want: true,
		},
		{
			name: "DifferentWhitespace",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			b: `{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Action": "s3:GetObject",
						"Resource": "*"
					}
				]
			}`,
			want: true,
		},
		{
			name: "DifferentStatementOrder",
			a: `{
				"Version": "2012-10-17",
				"Statement": [
					{"Sid": "1", "Effect": "Allow", "Action": "s3:GetObject", "Resource": "*"},
					{"Sid": "2", "Effect": "Deny", "Action": "s3:DeleteObject", "Resource": "*"}
				]
			}`,
			b: `{
				"Version": "2012-10-17",
				"Statement": [
					{"Sid": "2", "Effect": "Deny", "Action": "s3:DeleteObject", "Resource": "*"},
					{"Sid": "1", "Effect": "Allow", "Action": "s3:GetObject", "Resource": "*"}
				]
			}`,
			want: true,
		},
		{
			name: "ActionAsStringVsArray",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			b:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":"*"}]}`,
			want: true,
		},
		{
			name: "ResourceAsStringVsArray",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"arn:aws:s3:::bucket"}]}`,
			b:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":["arn:aws:s3:::bucket"]}]}`,
			want: true,
		},
		{
			name: "DifferentActions",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			b:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:PutObject","Resource":"*"}]}`,
			want: false,
		},
		{
			name: "DifferentVersion",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`,
			b:    `{"Version":"2008-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`,
			want: false,
		},
		{
			name: "DifferentStatementCount",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`,
			b: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:*","Resource":"*"},
				{"Effect":"Deny","Action":"s3:DeleteObject","Resource":"*"}
			]}`,
			want: false,
		},
		{
			name:    "InvalidJSONFirst",
			a:       `{invalid json`,
			b:       `{"Version":"2012-10-17","Statement":[]}`,
			want:    false,
			wantErr: true,
		},
		{
			name:    "InvalidJSONSecond",
			a:       `{"Version":"2012-10-17","Statement":[]}`,
			b:       `{invalid json`,
			want:    false,
			wantErr: true,
		},
		{
			name: "WithConditions",
			a: `{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Action": "s3:*",
					"Resource": "*",
					"Condition": {
						"StringEquals": {
							"aws:PrincipalOrgID": "o-123456"
						}
					}
				}]
			}`,
			b: `{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Action": "s3:*",
					"Resource": "*",
					"Condition": {
						"StringEquals": {
							"aws:PrincipalOrgID": "o-123456"
						}
					}
				}]
			}`,
			want: true,
		},
		{
			name: "DifferentConditions",
			a: `{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Action": "s3:*",
					"Resource": "*",
					"Condition": {
						"StringEquals": {
							"aws:PrincipalOrgID": "o-111111"
						}
					}
				}]
			}`,
			b: `{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Action": "s3:*",
					"Resource": "*",
					"Condition": {
						"StringEquals": {
							"aws:PrincipalOrgID": "o-222222"
						}
					}
				}]
			}`,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IAMPolicyDocumentEqual(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
			}
		})
	}
}
