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

package types

import "math"

// LateInitializationRetryConfig type contains the runtime configuration for late initialization of any field.
type LateInitializationRetryConfig struct {
	// Minimum backoff duration in seconds to retry late initialization for a field
	MinBackoffSeconds int
	// Maximum backoff duration in seconds to retry late initialization for a field
	MaxBackoffSeconds int
}

// GetExponentialBackoffSeconds returns the exponentialBackoff duration for the LateInitializationConfig
// based on the 'numAttempts' parameter
func (l *LateInitializationRetryConfig) GetExponentialBackoffSeconds(numAttempt int) int {
	// return MinBackoffSeconds as default
	if numAttempt <= 0 {
		return l.MinBackoffSeconds
	}

	exponentialDelay := math.Pow(2, float64(numAttempt-1))
	netExponentialBackoff := float64(l.MinBackoffSeconds) + exponentialDelay
	// If the user provided MaxBackoffSeconds, do not exceed that limit
	if l.MaxBackoffSeconds != 0 && l.MaxBackoffSeconds > l.MinBackoffSeconds {
		netExponentialBackoff = math.Min(float64(l.MaxBackoffSeconds), netExponentialBackoff)
	}
	return int(netExponentialBackoff)
}
