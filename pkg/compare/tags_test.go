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

package compare_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws-controllers-k8s/runtime/pkg/compare"
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
)

func TestGetTagsDifference(t *testing.T) {
	tests := []struct {
		name      string
		from      acktags.Tags
		to        acktags.Tags
		wantAdded acktags.Tags
		wantUnch  acktags.Tags
		wantRemov acktags.Tags
	}{
		{
			name:      "both empty",
			from:      acktags.Tags{},
			to:        acktags.Tags{},
			wantAdded: acktags.Tags{},
			wantUnch:  acktags.Tags{},
			wantRemov: acktags.Tags{},
		},
		{
			name: "all unchanged",
			from: acktags.Tags{"a": "1", "b": "2"},
			to:   acktags.Tags{"a": "1", "b": "2"},
			wantAdded: acktags.Tags{},
			wantUnch:  acktags.Tags{"a": "1", "b": "2"},
			wantRemov: acktags.Tags{},
		},
		{
			name: "tags added",
			from: acktags.Tags{},
			to:   acktags.Tags{"a": "1", "b": "2"},
			wantAdded: acktags.Tags{"a": "1", "b": "2"},
			wantUnch:  acktags.Tags{},
			wantRemov: acktags.Tags{},
		},
		{
			name: "tags removed",
			from: acktags.Tags{"a": "1", "b": "2"},
			to:   acktags.Tags{},
			wantAdded: acktags.Tags{},
			wantUnch:  acktags.Tags{},
			wantRemov: acktags.Tags{"a": "1", "b": "2"},
		},
		{
			name: "value change only in added not removed",
			from: acktags.Tags{"env": "prod"},
			to:   acktags.Tags{"env": "staging"},
			wantAdded: acktags.Tags{"env": "staging"},
			wantUnch:  acktags.Tags{},
			wantRemov: acktags.Tags{},
		},
		{
			name: "mixed add remove unchanged and value change",
			from: acktags.Tags{"keep": "same", "remove": "old", "update": "old"},
			to:   acktags.Tags{"keep": "same", "new": "val", "update": "new"},
			wantAdded: acktags.Tags{"new": "val", "update": "new"},
			wantUnch:  acktags.Tags{"keep": "same"},
			wantRemov: acktags.Tags{"remove": "old"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			added, unchanged, removed := compare.GetTagsDifference(tt.from, tt.to)

			require.Equal(tt.wantAdded, added)
			require.Equal(tt.wantUnch, unchanged)
			require.Equal(tt.wantRemov, removed)
		})
	}
}
