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

// Package featuregate provides a simple mechanism for managing feature gates
// in ACK controllers. It allows for default gates to be defined and
// optionally overridden.
package featuregate

import "fmt"

const (
	// ResourceAdoption is a feature gate for enabling forced adoption of resources
	// by annotation
	ResourceAdoption = "ResourceAdoption"

	// ReadOnlyResources is a feature gate for enabling ReadOnly resources annotation.
	ReadOnlyResources = "ReadOnlyResources"

	// TeamLevelCARM is a feature gate for enabling CARM for team-level resources.
	TeamLevelCARM = "TeamLevelCARM"

	// ServiceLevelCARM is a feature gate for enabling CARM for service-level resources.
	ServiceLevelCARM = "ServiceLevelCARM"
)

// defaultACKFeatureGates is a map of feature names to Feature structs
// representing the default feature gates for ACK controllers.
var defaultACKFeatureGates = FeatureGates{
	ResourceAdoption:  {Stage: Beta, Enabled: true},
	ReadOnlyResources: {Stage: Beta, Enabled: true},
	TeamLevelCARM:     {Stage: Alpha, Enabled: false},
	ServiceLevelCARM:  {Stage: Alpha, Enabled: false},
}

// FeatureStage represents the development stage of a feature.
type FeatureStage string

const (
	// Alpha represents a feature in early testing, potentially unstable.
	// Alpha features may be removed or changed at any time and are disabled
	// by default.
	Alpha FeatureStage = "alpha"

	// Beta represents a feature in advanced testing, more stable than alpha.
	// Beta features are enabled by default.
	Beta FeatureStage = "beta"

	// GA represents a feature that is generally available and stable.
	GA FeatureStage = "ga"
)

// Feature represents a single feature gate with its properties.
type Feature struct {
	// Stage indicates the current development stage of the feature.
	Stage FeatureStage

	// Enabled determines if the feature is enabled.
	Enabled bool
}

// FeatureGates is a map representing a set of feature gates.
type FeatureGates map[string]Feature

// IsEnabled checks if a feature with the given name is enabled.
// It returns true if the feature exists and is enabled, false
// otherwise.
func (fg FeatureGates) IsEnabled(name string) bool {
	feature, ok := fg[name]
	return ok && feature.Enabled
}

// GetFeature retrieves a feature by its name.
// It returns the Feature struct and a boolean indicating whether the
// feature was found.
func (fg FeatureGates) GetFeature(name string) (Feature, bool) {
	feature, ok := fg[name]
	return feature, ok
}

// GetFeatureNames returns a slice of feature names in the FeatureGates
// instance.
func (fg FeatureGates) GetFeatureNames() []string {
	names := make([]string, 0, len(fg))
	for name := range fg {
		names = append(names, name)
	}
	return names
}

// GetDefaultFeatureGates returns a new FeatureGates instance initialized with the default feature set.
// This function should be used when no overrides are needed.
func GetDefaultFeatureGates() FeatureGates {
	gates := make(FeatureGates)
	for name, feature := range defaultACKFeatureGates {
		gates[name] = feature
	}
	return gates
}

// GetFeatureGatesWithOverrides returns a new FeatureGates instance with the default features,
// but with the provided overrides applied. This allows for runtime configuration of feature gates.
func GetFeatureGatesWithOverrides(featureGateOverrides map[string]bool) (FeatureGates, error) {
	gates := GetDefaultFeatureGates()
	for name, enabled := range featureGateOverrides {
		if feature, ok := gates[name]; ok {
			feature.Enabled = enabled
			gates[name] = feature
		} else {
			return nil, fmt.Errorf("unknown feature gate: %v", name)
		}
	}
	return gates, nil
}
