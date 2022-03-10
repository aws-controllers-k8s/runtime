// Code generated by mockery v2.9.4. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	manager "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgtypes "k8s.io/apimachinery/pkg/types"

	reconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"

	types "github.com/aws-controllers-k8s/runtime/pkg/types"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// FieldExportReconciler is an autogenerated mock type for the FieldExportReconciler type
type FieldExportReconciler struct {
	mock.Mock
}

// BindControllerManagerForAWSResource provides a mock function with given fields: _a0
func (_m *FieldExportReconciler) BindControllerManagerForAWSResource(_a0 manager.Manager) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(manager.Manager) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// BindControllerManagerForFieldExport provides a mock function with given fields: _a0
func (_m *FieldExportReconciler) BindControllerManagerForFieldExport(_a0 manager.Manager) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(manager.Manager) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetFieldExportsForResource provides a mock function with given fields: _a0, _a1, _a2
func (_m *FieldExportReconciler) GetFieldExportsForResource(_a0 context.Context, _a1 v1.GroupKind, _a2 pkgtypes.NamespacedName) ([]v1alpha1.FieldExport, error) {
	ret := _m.Called(_a0, _a1, _a2)

	var r0 []v1alpha1.FieldExport
	if rf, ok := ret.Get(0).(func(context.Context, v1.GroupKind, pkgtypes.NamespacedName) []v1alpha1.FieldExport); ok {
		r0 = rf(_a0, _a1, _a2)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]v1alpha1.FieldExport)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1.GroupKind, pkgtypes.NamespacedName) error); ok {
		r1 = rf(_a0, _a1, _a2)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Reconcile provides a mock function with given fields: _a0, _a1
func (_m *FieldExportReconciler) Reconcile(_a0 context.Context, _a1 reconcile.Request) (reconcile.Result, error) {
	ret := _m.Called(_a0, _a1)

	var r0 reconcile.Result
	if rf, ok := ret.Get(0).(func(context.Context, reconcile.Request) reconcile.Result); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Get(0).(reconcile.Result)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, reconcile.Request) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SecretValueFromReference provides a mock function with given fields: _a0, _a1
func (_m *FieldExportReconciler) SecretValueFromReference(_a0 context.Context, _a1 *v1alpha1.SecretKeyReference) (string, error) {
	ret := _m.Called(_a0, _a1)

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context, *v1alpha1.SecretKeyReference) string); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *v1alpha1.SecretKeyReference) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Sync provides a mock function with given fields: _a0, _a1, _a2
func (_m *FieldExportReconciler) Sync(_a0 context.Context, _a1 types.AWSResource, _a2 v1alpha1.FieldExport) (v1alpha1.FieldExport, error) {
	ret := _m.Called(_a0, _a1, _a2)

	var r0 v1alpha1.FieldExport
	if rf, ok := ret.Get(0).(func(context.Context, types.AWSResource, v1alpha1.FieldExport) v1alpha1.FieldExport); ok {
		r0 = rf(_a0, _a1, _a2)
	} else {
		r0 = ret.Get(0).(v1alpha1.FieldExport)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, types.AWSResource, v1alpha1.FieldExport) error); ok {
		r1 = rf(_a0, _a1, _a2)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
