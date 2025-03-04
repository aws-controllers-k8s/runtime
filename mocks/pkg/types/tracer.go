// Code generated by mockery v2.53.0. DO NOT EDIT.

package mocks

import (
	types "github.com/aws-controllers-k8s/runtime/pkg/types"
	mock "github.com/stretchr/testify/mock"
)

// Tracer is an autogenerated mock type for the Tracer type
type Tracer struct {
	mock.Mock
}

// Trace provides a mock function with given fields: name, additionalValues
func (_m *Tracer) Trace(name string, additionalValues ...interface{}) types.TraceExiter {
	var _ca []interface{}
	_ca = append(_ca, name)
	_ca = append(_ca, additionalValues...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Trace")
	}

	var r0 types.TraceExiter
	if rf, ok := ret.Get(0).(func(string, ...interface{}) types.TraceExiter); ok {
		r0 = rf(name, additionalValues...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(types.TraceExiter)
		}
	}

	return r0
}

// NewTracer creates a new instance of Tracer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewTracer(t interface {
	mock.TestingT
	Cleanup(func())
}) *Tracer {
	mock := &Tracer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
