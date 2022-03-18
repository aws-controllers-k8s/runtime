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

package log

import (
	"strings"

	"github.com/go-logr/logr"

	"github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// ResourceLogger is a wrapper around a logr.Logger that writes log messages
// about resources involved in a controller loop. It implements
// `pkg/types.Logger`
type ResourceLogger struct {
	log        logr.Logger
	res        acktypes.AWSResource
	blockDepth int
}

// WithValues adapts the internal logger with a set of additional values
func (rl *ResourceLogger) WithValues(
	values ...interface{},
) {
	rl.log = rl.log.WithValues(values...)
}

// Debug writes a supplied log message about a resource that includes a set of
// standard log values for the resource's kind, namespace, name, etc
func (rl *ResourceLogger) Debug(
	msg string,
	additionalValues ...interface{},
) {
	vals := expandResourceFields(rl.res, additionalValues...)
	rl.log.V(1).Info(msg, vals...)
}

// Info writes a supplied log message about a resource that includes a
// set of standard log values for the resource's kind, namespace, name, etc
func (rl *ResourceLogger) Info(
	msg string,
	additionalValues ...interface{},
) {
	vals := expandResourceFields(rl.res, additionalValues...)
	rl.log.V(0).Info(msg, vals...)
}

// Enter logs an entry to a function or code block
func (rl *ResourceLogger) Enter(
	name string, // name of the function or code block we're entering
	additionalValues ...interface{},
) {
	if rl.log.V(1).Enabled() {
		rl.blockDepth++
		depth := strings.Repeat(">", rl.blockDepth)
		msg := depth + " " + name
		vals := expandResourceFields(rl.res, additionalValues...)
		rl.log.V(1).Info(msg, vals...)
	}
}

// Exit logs an exit from a function or code block
func (rl *ResourceLogger) Exit(
	name string, // name of the function or code block we're exiting
	err error,
	additionalValues ...interface{},
) {
	if rl.log.V(1).Enabled() {
		depth := strings.Repeat("<", rl.blockDepth)
		msg := depth + " " + name
		if err != nil {
			additionalValues = append(additionalValues, "error")
			additionalValues = append(additionalValues, err)
		}
		vals := expandResourceFields(rl.res, additionalValues...)
		rl.log.V(1).Info(msg, vals...)
		rl.blockDepth--
	}
}

// Trace logs an entry to a function or code block and returns a functor
// that can be called to log the exit of the function or code block
func (rl *ResourceLogger) Trace(
	name string,
	additionalValues ...interface{},
) acktypes.TraceExiter {
	rl.Enter(name, additionalValues...)
	f := func(err error, args ...interface{}) {
		rl.Exit(name, err, args...)
	}
	return f
}

// NewResourceLogger returns a resourceLogger that can write log messages about
// a resource.
func NewResourceLogger(
	log logr.Logger,
	res acktypes.AWSResource,
	additionalValues ...interface{},
) *ResourceLogger {
	return &ResourceLogger{
		log:        log.WithValues(additionalValues...),
		res:        res,
		blockDepth: 0,
	}
}

// AdaptResource returns a logger with log values set for the resource's kind,
// namespace, name, etc
func AdaptResource(
	log logr.Logger,
	res acktypes.AWSResource,
	additionalValues ...interface{},
) logr.Logger {
	vals := expandResourceFields(res, additionalValues...)
	return log.WithValues(vals...)
}

// DebugResource writes a supplied log message about a resource that includes a
// set of standard log values for the resource's kind, namespace, name, etc
func DebugResource(
	log logr.Logger,
	res acktypes.AWSResource,
	msg string,
	additionalValues ...interface{},
) {
	AdaptResource(log, res, additionalValues...).V(1).Info(msg)
}

// InfoResource writes a supplied log message about a resource that includes a
// set of standard log values for the resource's kind, namespace, name, etc
func InfoResource(
	log logr.Logger,
	res acktypes.AWSResource,
	msg string,
	additionalValues ...interface{},
) {
	AdaptResource(log, res, additionalValues...).V(0).Info(msg)
}

// expandResourceFields returns the key/value pairs for a resource that should
// be used as structured data in log messages about the resource
func expandResourceFields(
	res acktypes.AWSResource,
	additionalValues ...interface{},
) []interface{} {
	metaObj := res.MetaObject()
	generation := metaObj.GetGeneration()
	vals := []interface{}{
		"generation", generation,
	}
	if len(additionalValues) > 0 {
		vals = append(vals, additionalValues...)
	}
	return vals
}

// AdaptAdoptedResource returns a logger with log values set for the adopted
// resource's kind, namespace, name, etc
func AdaptAdoptedResource(
	log logr.Logger,
	res *v1alpha1.AdoptedResource,
	additionalValues ...interface{},
) logr.Logger {
	vals := expandAdoptedResourceFields(res, additionalValues...)
	return log.WithValues(vals...)
}

// DebugAdoptedResource writes a supplied log message about a adopted resource that
// includes a set of standard log values for the resource's kind, namespace, name, etc
func DebugAdoptedResource(
	log logr.Logger,
	res *v1alpha1.AdoptedResource,
	msg string,
	additionalValues ...interface{},
) {
	AdaptAdoptedResource(log, res, additionalValues...).V(1).Info(msg)
}

// InfoAdoptedResource writes a supplied log message about a adopted resource that
// includes a set of standard log values for the resource's kind, namespace, name, etc
func InfoAdoptedResource(
	log logr.Logger,
	res *v1alpha1.AdoptedResource,
	msg string,
	additionalValues ...interface{},
) {
	AdaptAdoptedResource(log, res, additionalValues...).V(0).Info(msg)
}

// expandAdoptedResourceFields returns the key/value pairs for an adopted
// resource that should be used as structured data in log messages about the
// adopted resource
func expandAdoptedResourceFields(
	res *v1alpha1.AdoptedResource,
	additionalValues ...interface{},
) []interface{} {
	ns := res.Namespace
	resName := res.Name
	generation := res.Generation
	group := res.Spec.Kubernetes.Group
	kind := res.Spec.Kubernetes.Kind
	vals := []interface{}{
		"target_group", group,
		"target_kind", kind,
		"namespace", ns,
		"name", resName,
		"generation", generation,
	}
	if len(additionalValues) > 0 {
		vals = append(vals, additionalValues...)
	}
	return vals
}

// AdaptFieldExport returns a logger with log values set for the adopted
// resource's kind, namespace, name, etc
func AdaptFieldExport(
	log logr.Logger,
	res *v1alpha1.FieldExport,
	additionalValues ...interface{},
) logr.Logger {
	vals := expandFieldExportFields(res, additionalValues...)
	return log.WithValues(vals...)
}

// DebugFieldExport writes a supplied log message about a field export that
// includes a set of standard log values for the resource's kind, namespace, name, etc
func DebugFieldExport(
	log logr.Logger,
	res *v1alpha1.FieldExport,
	msg string,
	additionalValues ...interface{},
) {
	AdaptFieldExport(log, res, additionalValues...).V(1).Info(msg)
}

// InfoFieldExport writes a supplied log message about a field export that
// includes a set of standard log values for the resource's kind, namespace, name, etc
func InfoFieldExport(
	log logr.Logger,
	res *v1alpha1.FieldExport,
	msg string,
	additionalValues ...interface{},
) {
	AdaptFieldExport(log, res, additionalValues...).V(0).Info(msg)
}

// expandFieldExportFields returns the key/value pairs for an adopted
// resource that should be used as structured data in log messages about the
// field export
func expandFieldExportFields(
	res *v1alpha1.FieldExport,
	additionalValues ...interface{},
) []interface{} {
	ns := res.Namespace
	resName := res.Name
	generation := res.Generation
	vals := []interface{}{
		"source_name", res.Spec.From.Resource.Name,
		"source_kind", res.Spec.From.Resource.Kind,
		"source_path", res.Spec.From.Path,
		"target_name", res.Spec.To.Name,
		"target_namespace", res.Spec.To.Namespace,
		"target_kind", res.Spec.To.Kind,
		"namespace", ns,
		"name", resName,
		"generation", generation,
	}
	if len(additionalValues) > 0 {
		vals = append(vals, additionalValues...)
	}
	return vals
}
