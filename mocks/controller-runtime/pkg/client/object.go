// Code generated by mockery v2.9.4. DO NOT EDIT.

package mocks

import (
	mock "github.com/stretchr/testify/mock"
	runtime "k8s.io/apimachinery/pkg/runtime"

	schema "k8s.io/apimachinery/pkg/runtime/schema"

	types "k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Object is an autogenerated mock type for the Object type
type Object struct {
	mock.Mock
}

// DeepCopyObject provides a mock function with given fields:
func (_m *Object) DeepCopyObject() runtime.Object {
	ret := _m.Called()

	var r0 runtime.Object
	if rf, ok := ret.Get(0).(func() runtime.Object); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runtime.Object)
		}
	}

	return r0
}

// GetAnnotations provides a mock function with given fields:
func (_m *Object) GetAnnotations() map[string]string {
	ret := _m.Called()

	var r0 map[string]string
	if rf, ok := ret.Get(0).(func() map[string]string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]string)
		}
	}

	return r0
}

// GetClusterName provides a mock function with given fields:
func (_m *Object) GetClusterName() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetCreationTimestamp provides a mock function with given fields:
func (_m *Object) GetCreationTimestamp() v1.Time {
	ret := _m.Called()

	var r0 v1.Time
	if rf, ok := ret.Get(0).(func() v1.Time); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(v1.Time)
	}

	return r0
}

// GetDeletionGracePeriodSeconds provides a mock function with given fields:
func (_m *Object) GetDeletionGracePeriodSeconds() *int64 {
	ret := _m.Called()

	var r0 *int64
	if rf, ok := ret.Get(0).(func() *int64); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*int64)
		}
	}

	return r0
}

// GetDeletionTimestamp provides a mock function with given fields:
func (_m *Object) GetDeletionTimestamp() *v1.Time {
	ret := _m.Called()

	var r0 *v1.Time
	if rf, ok := ret.Get(0).(func() *v1.Time); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Time)
		}
	}

	return r0
}

// GetFinalizers provides a mock function with given fields:
func (_m *Object) GetFinalizers() []string {
	ret := _m.Called()

	var r0 []string
	if rf, ok := ret.Get(0).(func() []string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// GetGenerateName provides a mock function with given fields:
func (_m *Object) GetGenerateName() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetGeneration provides a mock function with given fields:
func (_m *Object) GetGeneration() int64 {
	ret := _m.Called()

	var r0 int64
	if rf, ok := ret.Get(0).(func() int64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int64)
	}

	return r0
}

// GetLabels provides a mock function with given fields:
func (_m *Object) GetLabels() map[string]string {
	ret := _m.Called()

	var r0 map[string]string
	if rf, ok := ret.Get(0).(func() map[string]string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]string)
		}
	}

	return r0
}

// GetManagedFields provides a mock function with given fields:
func (_m *Object) GetManagedFields() []v1.ManagedFieldsEntry {
	ret := _m.Called()

	var r0 []v1.ManagedFieldsEntry
	if rf, ok := ret.Get(0).(func() []v1.ManagedFieldsEntry); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]v1.ManagedFieldsEntry)
		}
	}

	return r0
}

// GetName provides a mock function with given fields:
func (_m *Object) GetName() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetNamespace provides a mock function with given fields:
func (_m *Object) GetNamespace() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetObjectKind provides a mock function with given fields:
func (_m *Object) GetObjectKind() schema.ObjectKind {
	ret := _m.Called()

	var r0 schema.ObjectKind
	if rf, ok := ret.Get(0).(func() schema.ObjectKind); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(schema.ObjectKind)
		}
	}

	return r0
}

// GetOwnerReferences provides a mock function with given fields:
func (_m *Object) GetOwnerReferences() []v1.OwnerReference {
	ret := _m.Called()

	var r0 []v1.OwnerReference
	if rf, ok := ret.Get(0).(func() []v1.OwnerReference); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]v1.OwnerReference)
		}
	}

	return r0
}

// GetResourceVersion provides a mock function with given fields:
func (_m *Object) GetResourceVersion() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetSelfLink provides a mock function with given fields:
func (_m *Object) GetSelfLink() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetUID provides a mock function with given fields:
func (_m *Object) GetUID() types.UID {
	ret := _m.Called()

	var r0 types.UID
	if rf, ok := ret.Get(0).(func() types.UID); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(types.UID)
	}

	return r0
}

// SetAnnotations provides a mock function with given fields: annotations
func (_m *Object) SetAnnotations(annotations map[string]string) {
	_m.Called(annotations)
}

// SetClusterName provides a mock function with given fields: clusterName
func (_m *Object) SetClusterName(clusterName string) {
	_m.Called(clusterName)
}

// SetCreationTimestamp provides a mock function with given fields: timestamp
func (_m *Object) SetCreationTimestamp(timestamp v1.Time) {
	_m.Called(timestamp)
}

// SetDeletionGracePeriodSeconds provides a mock function with given fields: _a0
func (_m *Object) SetDeletionGracePeriodSeconds(_a0 *int64) {
	_m.Called(_a0)
}

// SetDeletionTimestamp provides a mock function with given fields: timestamp
func (_m *Object) SetDeletionTimestamp(timestamp *v1.Time) {
	_m.Called(timestamp)
}

// SetFinalizers provides a mock function with given fields: finalizers
func (_m *Object) SetFinalizers(finalizers []string) {
	_m.Called(finalizers)
}

// SetGenerateName provides a mock function with given fields: name
func (_m *Object) SetGenerateName(name string) {
	_m.Called(name)
}

// SetGeneration provides a mock function with given fields: generation
func (_m *Object) SetGeneration(generation int64) {
	_m.Called(generation)
}

// SetLabels provides a mock function with given fields: labels
func (_m *Object) SetLabels(labels map[string]string) {
	_m.Called(labels)
}

// SetManagedFields provides a mock function with given fields: managedFields
func (_m *Object) SetManagedFields(managedFields []v1.ManagedFieldsEntry) {
	_m.Called(managedFields)
}

// SetName provides a mock function with given fields: name
func (_m *Object) SetName(name string) {
	_m.Called(name)
}

// SetNamespace provides a mock function with given fields: namespace
func (_m *Object) SetNamespace(namespace string) {
	_m.Called(namespace)
}

// SetOwnerReferences provides a mock function with given fields: _a0
func (_m *Object) SetOwnerReferences(_a0 []v1.OwnerReference) {
	_m.Called(_a0)
}

// SetResourceVersion provides a mock function with given fields: version
func (_m *Object) SetResourceVersion(version string) {
	_m.Called(version)
}

// SetSelfLink provides a mock function with given fields: selfLink
func (_m *Object) SetSelfLink(selfLink string) {
	_m.Called(selfLink)
}

// SetUID provides a mock function with given fields: uid
func (_m *Object) SetUID(uid types.UID) {
	_m.Called(uid)
}
