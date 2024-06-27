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

package featuregate

import (
	"testing"
)

func TestIsEnabled(t *testing.T) {
	gates := FeatureGates{
		"enabledFeature":  {Stage: Alpha, Enabled: true},
		"disabledFeature": {Stage: Beta, Enabled: false},
	}

	tests := []struct {
		name     string
		feature  string
		expected bool
	}{
		{"Enabled feature", "enabledFeature", true},
		{"Disabled feature", "disabledFeature", false},
		{"Non-existent feature", "nonExistentFeature", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gates.IsEnabled(tt.feature); got != tt.expected {
				t.Errorf("IsEnabled(%q) = %v, want %v", tt.feature, got, tt.expected)
			}
		})
	}
}

func TestGetFeature(t *testing.T) {
	gates := FeatureGates{
		"existingFeature": {Stage: GA, Enabled: true},
	}

	tests := []struct {
		name          string
		feature       string
		expectedFound bool
	}{
		{"Existing feature", "existingFeature", true},
		{"Non-existent feature", "nonExistentFeature", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feature, found := gates.GetFeature(tt.feature)
			if found != tt.expectedFound {
				t.Errorf("GetFeature(%q) found = %v, want %v", tt.feature, found, tt.expectedFound)
			}
			if found {
				if feature.Stage != GA || !feature.Enabled {
					t.Errorf("GetFeature(%q) = %+v, want {Stage: GA, Enabled: true}", tt.feature, feature)
				}
			}
		})
	}
}

func TestGetDefaultFeatureGates(t *testing.T) {
	// Temporarily replace defaultFeatureGates for this test
	oldDefaultFeatureGates := defaultACKFeatureGates
	defaultACKFeatureGates = FeatureGates{
		"feature1": {Stage: Alpha, Enabled: false},
		"feature2": {Stage: Beta, Enabled: true},
	}
	defer func() { defaultACKFeatureGates = oldDefaultFeatureGates }()

	gates := GetDefaultFeatureGates()

	if len(gates) != 2 {
		t.Errorf("GetDefaultFeatureGates() returned %d feature gates, want 2", len(gates))
	}

	if !gates.IsEnabled("feature2") {
		t.Errorf("feature2 should be enabled")
	}

	if gates.IsEnabled("feature1") {
		t.Errorf("feature1 should be disabled")
	}
}

func TestGetFeatureGatesWithOverrides(t *testing.T) {
	// Temporarily replace defaultFeatureGates for this test
	oldDefaultFeatureGates := defaultACKFeatureGates
	defaultACKFeatureGates = FeatureGates{
		"feature1": {Stage: Alpha, Enabled: false},
		"feature2": {Stage: Beta, Enabled: true},
		"feature3": {Stage: GA, Enabled: false},
	}
	defer func() { defaultACKFeatureGates = oldDefaultFeatureGates }()

	overrides := map[string]bool{
		"feature1": true,
		"feature2": false,
		"feature4": true, // This should be ignored as it's not in defaultFeatureGates
	}

	gates := GetFeatureGatesWithOverrides(overrides)

	tests := []struct {
		name     string
		feature  string
		expected bool
	}{
		{"Overridden to true", "feature1", true},
		{"Overridden to false", "feature2", false},
		{"Not overridden", "feature3", false},
		{"Non-existent feature", "feature4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gates.IsEnabled(tt.feature); got != tt.expected {
				t.Errorf("IsEnabled(%q) = %v, want %v", tt.feature, got, tt.expected)
			}
		})
	}

	if len(gates) != 3 {
		t.Errorf("GetFeatureGatesWithOverrides() returned %d feature gates, want 3", len(gates))
	}
}
