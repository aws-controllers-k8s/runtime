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

import (
	"math"
	"time"
)

// Exponential type helps in implementing exponential backoff strategy.
type Exponential struct {
	// Initial holds the initial delay.
	Initial time.Duration
	// Factor holds the factor that the delay time will be multiplied
	// by on each iteration. If this is zero, a factor of two will be used.
	Factor float64
	// MaxDelay holds the maximum delay between the start.
	// If this is zero, there is no maximum delay.
	MaxDelay time.Duration
}

// GetBackoff returns the exponentialBackoff duration for the LateInitializationConfig
// based on the 'numAttempts' parameter
func (l *Exponential) GetBackoff(numAttempt int) time.Duration {
	// return Initial as default
	if numAttempt <= 0 {
		return l.Initial
	}
	// Default base = 2
	var base float64 = 2
	if l.Factor != 0 {
		base = l.Factor
	}
	additionalDelaySeconds := math.Pow(base, float64(numAttempt-1))
	netBackoff := time.Duration(l.Initial.Seconds()+additionalDelaySeconds) * time.Second
	// If the user provided MaxDelay, do not exceed that limit
	if l.MaxDelay != 0 && l.MaxDelay > l.Initial {
		netBackoff = time.Duration(math.Min(netBackoff.Seconds(), l.MaxDelay.Seconds())) * time.Second
	}
	return netBackoff
}
