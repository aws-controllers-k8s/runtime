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

package annotation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	k8sobj "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/aws-controllers-k8s/runtime/pkg/annotation"
)

func TestGetNumLateInitializationAttempt(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(1, annotation.GetNumLateInitializationAttempt(nil))
	obj := &k8sobj.Unstructured{}
	obj.SetAnnotations(nil)
	assert.Equal(1, annotation.GetNumLateInitializationAttempt(obj))
	annotations := make(map[string]string)
	obj.SetAnnotations(annotations)
	assert.Equal(1, annotation.GetNumLateInitializationAttempt(obj))
	annotations[annotation.LateInitializationAttempt] = "abcd"
	obj.SetAnnotations(annotations)
	assert.Equal(1, annotation.GetNumLateInitializationAttempt(obj))
	annotations[annotation.LateInitializationAttempt] = "10"
	obj.SetAnnotations(annotations)
	assert.Equal(10, annotation.GetNumLateInitializationAttempt(obj))
}

func TestIncrementNumLateInitializationAttempt(t *testing.T) {
	assert := assert.New(t)
	// Handles nil parameter
	err := annotation.IncrementNumLateInitializationAttempt(nil)
	assert.NotNil(err)
	assert.Equal("metav1Obj parameter should not be nil", err.Error())
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	err = annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Nil(err)
	assert.Equal("1", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	err = annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Nil(err)
	assert.Equal("1", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	err = annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Nil(err)
	assert.Equal("100", obj.GetAnnotations()[annotation.LateInitializationAttempt])
}

func TestSetNumLateInitializationAttempt(t *testing.T) {
	assert := assert.New(t)
	// Handles nil parameter
	err := annotation.SetNumLateInitializationAttempt(nil, 10)
	assert.NotNil(err)
	assert.Equal("metav1Obj parameter should not be nil", err.Error())
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	err = annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Nil(err)
	assert.Equal("10", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	err = annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Nil(err)
	assert.Equal("10", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	err = annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Nil(err)
	assert.Equal("10", obj.GetAnnotations()[annotation.LateInitializationAttempt])
}

func TestRemoveLateInitializationAttempt(t *testing.T) {
	assert := assert.New(t)
	// Handles nil parameter
	annotation.RemoveLateInitializationAttempt(nil)
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	annotation.RemoveLateInitializationAttempt(obj)
	assert.Nil(obj.GetAnnotations())

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	annotation.RemoveLateInitializationAttempt(obj)
	_, ok := obj.GetAnnotations()[annotation.LateInitializationAttempt]
	assert.False(ok)

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	annotation.RemoveLateInitializationAttempt(obj)
	_, ok = obj.GetAnnotations()[annotation.LateInitializationAttempt]
	assert.False(ok)
}
