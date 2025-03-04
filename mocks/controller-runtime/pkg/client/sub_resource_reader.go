// Code generated by mockery v2.53.0. DO NOT EDIT.

package mocks

import (
	context "context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	mock "github.com/stretchr/testify/mock"
)

// SubResourceReader is an autogenerated mock type for the SubResourceReader type
type SubResourceReader struct {
	mock.Mock
}

// Get provides a mock function with given fields: ctx, obj, subResource, opts
func (_m *SubResourceReader) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, obj, subResource)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, client.Object, client.Object, ...client.SubResourceGetOption) error); ok {
		r0 = rf(ctx, obj, subResource, opts...)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewSubResourceReader creates a new instance of SubResourceReader. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewSubResourceReader(t interface {
	mock.TestingT
	Cleanup(func())
}) *SubResourceReader {
	mock := &SubResourceReader{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
