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
	assert.Nil(annotation.IncrementNumLateInitializationAttempt(nil))
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	res := annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Equal("1", res.GetAnnotations()[annotation.LateInitializationAttempt])

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	res = annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Equal("1", res.GetAnnotations()[annotation.LateInitializationAttempt])

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	res = annotation.IncrementNumLateInitializationAttempt(obj)
	assert.Equal("100", res.GetAnnotations()[annotation.LateInitializationAttempt])
}

func TestSetNumLateInitializationAttempt(t *testing.T) {
	assert := assert.New(t)
	assert.Nil(annotation.SetNumLateInitializationAttempt(nil, 10))
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	res := annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Equal("10", res.GetAnnotations()[annotation.LateInitializationAttempt])

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	res = annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Equal("10", res.GetAnnotations()[annotation.LateInitializationAttempt])

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	res = annotation.SetNumLateInitializationAttempt(obj, 10)
	assert.Equal("10", res.GetAnnotations()[annotation.LateInitializationAttempt])
}

func TestRemoveLateInitializationAttempt(t *testing.T) {
	assert := assert.New(t)
	assert.Nil(annotation.RemoveLateInitializationAttempt(nil))
	obj := &k8sobj.Unstructured{}
	// nil annotations
	obj.SetAnnotations(nil)
	res := annotation.RemoveLateInitializationAttempt(obj)
	assert.Nil(res.GetAnnotations())

	//empty annotations
	obj.SetAnnotations(make(map[string]string))
	res = annotation.RemoveLateInitializationAttempt(obj)
	_, ok := res.GetAnnotations()[annotation.LateInitializationAttempt]
	assert.False(ok)

	obj.SetAnnotations(map[string]string{annotation.LateInitializationAttempt: "99"})
	res = annotation.RemoveLateInitializationAttempt(obj)
	_, ok = res.GetAnnotations()[annotation.LateInitializationAttempt]
	assert.False(ok)
}
