package compare_test

import (
	"testing"
	"time"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sobj "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/stretchr/testify/require"

	"github.com/aws-controllers-k8s/runtime/pkg/compare"
)

func TestMetaV1ObjectEqual_Nil(t *testing.T) {
	require := require.New(t)
	// both nil
	require.True(compare.MetaV1ObjectEqual(nil, nil))
	ob1 := &k8sobj.Unstructured{}
	// nil difference
	require.False(compare.MetaV1ObjectEqual(ob1, nil))
	require.False(compare.MetaV1ObjectEqual(nil, ob1))
}

func TestMetaV1ObjectEqual_Annotations(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob1.SetAnnotations(make(map[string]string))
	ob2 := &k8sobj.Unstructured{}
	ob2.SetAnnotations(make(map[string]string))
	// empty annotations
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))

	ob2Anns := ob2.GetAnnotations()
	ob2Anns["some"] = "Annotations"
	ob2.SetAnnotations(ob2Anns)
	// Unequal annotations
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob1Anns := ob1.GetAnnotations()
	ob1Anns["some"] = "Annotations"
	ob1.SetAnnotations(ob1Anns)
	// equal annotations
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_RemainingItemCount(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	var itemCount1 int64 = 10
	ob1.SetRemainingItemCount(&itemCount1)
	ob2 := &k8sobj.Unstructured{}

	// Unequal RemainingItemCount
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))

	// Equal RemainingItemCount
	ob2.SetRemainingItemCount(&itemCount1)
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_CreationTimestamp(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	time1 := k8smetav1.NewTime(time.Unix(5, 0))
	time2 := k8smetav1.NewTime(time1.Time)
	ob1.SetCreationTimestamp(time1)
	ob2 := &k8sobj.Unstructured{}
	ob2.SetCreationTimestamp(time2)
	// same timestamp
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))

	ob2.SetCreationTimestamp(k8smetav1.NewTime(time.Unix(10, 0)))
	// different timestamp
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_DeletionGracePeriodSeconds(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both nil
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	var time1, time2 int64
	time1 = 5
	time2 = 5
	ob1.SetDeletionGracePeriodSeconds(&time1)
	// only one value nil
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetDeletionGracePeriodSeconds(&time2)
	// both equal non nil
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	time2 = 6
	ob2.SetDeletionGracePeriodSeconds(&time2)
	// both unequal non nil
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_DeletionTimestamp(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both nil
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	time1 := k8smetav1.NewTime(time.Unix(5, 0))
	time2 := k8smetav1.NewTime(time.Unix(5, 0))
	ob1.SetDeletionTimestamp(&time1)
	// only one value nil
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetDeletionTimestamp(&time2)
	// both equal non nil
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	time2 = k8smetav1.NewTime(time.Unix(10, 0))
	ob2.SetDeletionTimestamp(&time2)
	// both unequal non nil
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_Finalizers(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both empty
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	finalizers1 := []string{"a"}
	ob1.SetFinalizers(finalizers1)
	// One non empty
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	finalizers2 := []string{"a"}
	ob2.SetFinalizers(finalizers2)
	// both non empty equal
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	finalizers2 = []string{"b"}
	ob2.SetFinalizers(finalizers2)
	// both non empty non equal
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_GenerateName(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both empty
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob1.SetGenerateName("a")
	// One non empty
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetGenerateName("a")
	// both non empty equal
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetGenerateName("b")
	// both non empty non equal
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_Labels(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob1.SetLabels(make(map[string]string))
	ob2 := &k8sobj.Unstructured{}
	ob2.SetLabels(make(map[string]string))
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))

	ob2Anns := ob2.GetLabels()
	ob2Anns["some"] = "Labels"
	ob2.SetLabels(ob2Anns)
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_ManagedField(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// empty managed fields
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	mf1 := k8smetav1.ManagedFieldsEntry{Manager: "manager"}
	mf1copy := k8smetav1.ManagedFieldsEntry{Manager: "manager"}

	ob1.SetManagedFields([]k8smetav1.ManagedFieldsEntry{mf1})
	ob2.SetManagedFields([]k8smetav1.ManagedFieldsEntry{mf1copy})
	// Same managed fields
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	mf2 := k8smetav1.ManagedFieldsEntry{Manager: "manager2"}
	// Unequal managed fields
	ob2.SetManagedFields([]k8smetav1.ManagedFieldsEntry{mf2})
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_Name(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both empty
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob1.SetName("a")
	// One non empty
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetName("a")
	// both non empty equal
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetName("b")
	// both non empty non equal
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_Namespace(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both empty
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob1.SetNamespace("a")
	// One non empty
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetNamespace("a")
	// both non empty equal
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetNamespace("b")
	// both non empty non equal
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_OwnerReferences(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// empty managed fields
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	or1 := k8smetav1.OwnerReference{Name: "name1"}
	or1copy := k8smetav1.OwnerReference{Name: "name1"}

	ob1.SetOwnerReferences([]k8smetav1.OwnerReference{or1})
	ob2.SetOwnerReferences([]k8smetav1.OwnerReference{or1copy})
	// Same managed fields
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	or2 := k8smetav1.OwnerReference{Name: "name2"}
	// Unequal managed fields
	ob2.SetOwnerReferences([]k8smetav1.OwnerReference{or2})
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_ResourceVersion(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both empty
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob1.SetResourceVersion("a")
	// One non empty
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetResourceVersion("a")
	// both non empty equal
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetResourceVersion("b")
	// both non empty non equal
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_SelfLink(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both empty
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob1.SetSelfLink("a")
	// One non empty
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetSelfLink("a")
	// both non empty equal
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetSelfLink("b")
	// both non empty non equal
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}

func TestMetaV1ObjectEqual_Uid(t *testing.T) {
	require := require.New(t)
	ob1 := &k8sobj.Unstructured{}
	ob2 := &k8sobj.Unstructured{}
	// both empty
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob1.SetUID("a")
	// One non empty
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetUID("a")
	// both non empty equal
	require.True(compare.MetaV1ObjectEqual(ob1, ob2))
	ob2.SetUID("b")
	// both non empty non equal
	require.False(compare.MetaV1ObjectEqual(ob1, ob2))
}
