// Code generated by mockery v2.53.0. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	manager "sigs.k8s.io/controller-runtime/pkg/manager"

	reconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"

	types "github.com/aws-controllers-k8s/runtime/pkg/types"

	v1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// AdoptedResourceReconciler is an autogenerated mock type for the AdoptedResourceReconciler type
type AdoptedResourceReconciler struct {
	mock.Mock
}

// BindControllerManager provides a mock function with given fields: _a0
func (_m *AdoptedResourceReconciler) BindControllerManager(_a0 manager.Manager) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for BindControllerManager")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(manager.Manager) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Reconcile provides a mock function with given fields: _a0, _a1
func (_m *AdoptedResourceReconciler) Reconcile(_a0 context.Context, _a1 reconcile.Request) (reconcile.Result, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for Reconcile")
	}

	var r0 reconcile.Result
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, reconcile.Request) (reconcile.Result, error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, reconcile.Request) reconcile.Result); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Get(0).(reconcile.Result)
	}

	if rf, ok := ret.Get(1).(func(context.Context, reconcile.Request) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SecretValueFromReference provides a mock function with given fields: _a0, _a1
func (_m *AdoptedResourceReconciler) SecretValueFromReference(_a0 context.Context, _a1 *v1alpha1.SecretKeyReference) (string, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for SecretValueFromReference")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *v1alpha1.SecretKeyReference) (string, error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *v1alpha1.SecretKeyReference) string); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, *v1alpha1.SecretKeyReference) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Sync provides a mock function with given fields: _a0, _a1, _a2, _a3
func (_m *AdoptedResourceReconciler) Sync(_a0 context.Context, _a1 types.AWSResourceDescriptor, _a2 types.AWSResourceManager, _a3 *v1alpha1.AdoptedResource) error {
	ret := _m.Called(_a0, _a1, _a2, _a3)

	if len(ret) == 0 {
		panic("no return value specified for Sync")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, types.AWSResourceDescriptor, types.AWSResourceManager, *v1alpha1.AdoptedResource) error); ok {
		r0 = rf(_a0, _a1, _a2, _a3)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// WriteToSecret provides a mock function with given fields: _a0, _a1, _a2, _a3, _a4
func (_m *AdoptedResourceReconciler) WriteToSecret(_a0 context.Context, _a1 string, _a2 string, _a3 string, _a4 string) error {
	ret := _m.Called(_a0, _a1, _a2, _a3, _a4)

	if len(ret) == 0 {
		panic("no return value specified for WriteToSecret")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, string) error); ok {
		r0 = rf(_a0, _a1, _a2, _a3, _a4)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewAdoptedResourceReconciler creates a new instance of AdoptedResourceReconciler. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewAdoptedResourceReconciler(t interface {
	mock.TestingT
	Cleanup(func())
}) *AdoptedResourceReconciler {
	mock := &AdoptedResourceReconciler{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
