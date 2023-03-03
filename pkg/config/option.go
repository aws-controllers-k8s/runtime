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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Option is a struct used to help validating the controller
// configuration
type Option struct {
	// gvks is an array containing the resources gvks (Camel-cased names)
	// managed by the controller
	gvks []schema.GroupVersionKind
}

// WithGVKs instructs the configuration to validate against a set of
// supplied resource kinds and their respective groups.
func WithGVKs(gvks []schema.GroupVersionKind) Option {
	return Option{gvks: gvks}
}

// mergeOptions merges all Option structs into a single Option
// and sets any defaults to missing values
func mergeOptions(opts []Option) Option {
	merged := Option{}
	for _, opt := range opts {
		if len(opt.gvks) > 0 {
			merged.gvks = opt.gvks
		}
	}
	// TODO: set some default values...
	return merged
}
