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

// TraceExiter demarcates the end of a traced code block or function
type TraceExiter func(err error, additionalValues ...interface{})

// Tracer is responsible for tracing entrance and exit from blocks of code
type Tracer interface {
	// Trace logs an entry to a function or code block and returns a functor
	// that can be called to log the exit of the function or code block
	Trace(name string, additionalValues ...interface{}) TraceExiter
}
