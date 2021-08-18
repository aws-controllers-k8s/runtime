package annotation_test

import (
	"testing"

	assert "github.com/stretchr/testify/assert"
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
	annotation.IncrementNumLateInitializationAttempt(nil)
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Equal("1", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Equal("1", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Equal("100", obj.GetAnnotations()[annotation.LateInitializationAttempt])
}

func TestSetNumLateInitializationAttempt(t *testing.T) {
	assert := assert.New(t)
	// Handles nil parameter
	annotation.SetNumLateInitializationAttempt(nil, 10)
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Equal("10", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Equal("10", obj.GetAnnotations()[annotation.LateInitializationAttempt])

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	annotation.SetNumLateInitializationAttempt(obj, 10)
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
