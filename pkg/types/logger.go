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

// Logger is responsible for writing log messages
type Logger interface {
	Tracer
	// WithValues adapts the internal logger with a set of additional key/value
	// data
	WithValues(...interface{})
	// Debug writes a supplied log message about a resource that includes a set
	// of standard log values for the resource's kind, namespace, name, etc
	Debug(msg string, additionalValues ...interface{})
	// Info writes a supplied log message about a resource that includes a set
	// of standard log values for the resource's kind, namespace, name, etc
	Info(msg string, additionalValues ...interface{})
	// Enter logs an entry to a function or code block
	Enter(name string, additionalValues ...interface{})
	// Exit logs an exit from a function or code block
	Exit(name string, err error, additionalValues ...interface{})
}
