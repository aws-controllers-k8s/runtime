// Code generated by mockery v2.53.3. DO NOT EDIT.

package mocks

import (
	mock "github.com/stretchr/testify/mock"
	client "sigs.k8s.io/controller-runtime/pkg/client"

	types "github.com/aws-controllers-k8s/runtime/pkg/types"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// AWSResource is an autogenerated mock type for the AWSResource type
type AWSResource struct {
	mock.Mock
}

// Conditions provides a mock function with no fields
func (_m *AWSResource) Conditions() []*v1alpha1.Condition {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Conditions")
	}

	var r0 []*v1alpha1.Condition
	if rf, ok := ret.Get(0).(func() []*v1alpha1.Condition); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*v1alpha1.Condition)
		}
	}

	return r0
}

// DeepCopy provides a mock function with no fields
func (_m *AWSResource) DeepCopy() types.AWSResource {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DeepCopy")
	}

	var r0 types.AWSResource
	if rf, ok := ret.Get(0).(func() types.AWSResource); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(types.AWSResource)
		}
	}

	return r0
}

// Identifiers provides a mock function with no fields
func (_m *AWSResource) Identifiers() types.AWSResourceIdentifiers {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Identifiers")
	}

	var r0 types.AWSResourceIdentifiers
	if rf, ok := ret.Get(0).(func() types.AWSResourceIdentifiers); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(types.AWSResourceIdentifiers)
		}
	}

	return r0
}

// IsBeingDeleted provides a mock function with no fields
func (_m *AWSResource) IsBeingDeleted() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for IsBeingDeleted")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// MetaObject provides a mock function with no fields
func (_m *AWSResource) MetaObject() v1.Object {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for MetaObject")
	}

	var r0 v1.Object
	if rf, ok := ret.Get(0).(func() v1.Object); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1.Object)
		}
	}

	return r0
}

// PopulateResourceFromAnnotation provides a mock function with given fields: fields
func (_m *AWSResource) PopulateResourceFromAnnotation(fields map[string]string) error {
	ret := _m.Called(fields)

	if len(ret) == 0 {
		panic("no return value specified for PopulateResourceFromAnnotation")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(map[string]string) error); ok {
		r0 = rf(fields)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ReplaceConditions provides a mock function with given fields: _a0
func (_m *AWSResource) ReplaceConditions(_a0 []*v1alpha1.Condition) {
	_m.Called(_a0)
}

// RuntimeObject provides a mock function with no fields
func (_m *AWSResource) RuntimeObject() client.Object {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for RuntimeObject")
	}

	var r0 client.Object
	if rf, ok := ret.Get(0).(func() client.Object); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(client.Object)
		}
	}

	return r0
}

// SetIdentifiers provides a mock function with given fields: _a0
func (_m *AWSResource) SetIdentifiers(_a0 *v1alpha1.AWSIdentifiers) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for SetIdentifiers")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1alpha1.AWSIdentifiers) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetObjectMeta provides a mock function with given fields: meta
func (_m *AWSResource) SetObjectMeta(meta v1.ObjectMeta) {
	_m.Called(meta)
}

// SetStatus provides a mock function with given fields: _a0
func (_m *AWSResource) SetStatus(_a0 types.AWSResource) {
	_m.Called(_a0)
}

// NewAWSResource creates a new instance of AWSResource. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewAWSResource(t interface {
	mock.TestingT
	Cleanup(func())
}) *AWSResource {
	mock := &AWSResource{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
