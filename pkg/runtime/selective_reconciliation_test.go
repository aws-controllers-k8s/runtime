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

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// uResource is a minimal AWSResource backed by an *unstructured.Unstructured,
// used to exercise the selective-reconciliation merge logic without depending
// on a generated resource type.
type uResource struct {
	obj        *unstructured.Unstructured
	conditions []*ackv1alpha1.Condition
}

// withLateInitializedCondition sets an ACK.LateInitialized condition on the
// resource, mirroring what the generated LateInitialize sets when a resource
// actually has late-init fields configured. The runtime uses the presence of
// this condition to distinguish a genuine late-init write-back (persist) from
// plain drift on an ignored field (suppress).
func (r *uResource) withLateInitializedCondition() *uResource {
	status := corev1.ConditionTrue
	r.conditions = append(r.conditions, &ackv1alpha1.Condition{
		Type:   ackv1alpha1.ConditionTypeLateInitialized,
		Status: status,
	})
	return r
}

func newUResource(annotations map[string]string, spec map[string]interface{}) *uResource {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetAPIVersion("test.services.k8s.aws/v1alpha1")
	u.SetKind("Thing")
	if annotations != nil {
		u.SetAnnotations(annotations)
	}
	if spec != nil {
		_ = unstructured.SetNestedField(u.Object, spec, "spec")
	}
	return &uResource{obj: u}
}

func (r *uResource) spec() map[string]interface{} {
	s, _, _ := unstructured.NestedMap(r.obj.Object, "spec")
	return s
}

func (r *uResource) Conditions() []*ackv1alpha1.Condition         { return r.conditions }
func (r *uResource) ReplaceConditions(c []*ackv1alpha1.Condition) { r.conditions = c }
func (r *uResource) Identifiers() acktypes.AWSResourceIdentifiers { return nil }
func (r *uResource) IsBeingDeleted() bool                         { return false }
func (r *uResource) RuntimeObject() rtclient.Object               { return r.obj }

// MetaObject returns a metadata-only view of the resource. It deliberately
// excludes the spec so that ackcompare.MetaV1ObjectEqual (which JSON-marshals
// the returned object) compares ONLY metadata, matching production behavior
// where MetaObject() returns the typed ObjectMeta rather than the whole object.
// Returning r.obj here would fold the spec into the metadata comparison and
// make equalMetadata spuriously false whenever the spec differs, masking the
// patch-gate logic the reconciler tests exercise. The returned object shares
// the same underlying metadata map as r.obj, so metadata reads stay in sync.
func (r *uResource) MetaObject() metav1.Object {
	meta := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if md, found, _ := unstructured.NestedFieldNoCopy(r.obj.Object, "metadata"); found {
		meta.Object["metadata"] = md
	}
	return meta
}
func (r *uResource) SetObjectMeta(meta metav1.ObjectMeta)             {}
func (r *uResource) SetIdentifiers(*ackv1alpha1.AWSIdentifiers) error { return nil }
func (r *uResource) SetStatus(acktypes.AWSResource)                   {}
func (r *uResource) PopulateResourceFromAnnotation(map[string]string) error {
	return nil
}
func (r *uResource) DeepCopy() acktypes.AWSResource {
	var conds []*ackv1alpha1.Condition
	if r.conditions != nil {
		conds = make([]*ackv1alpha1.Condition, len(r.conditions))
		for i, c := range r.conditions {
			cc := *c
			conds[i] = &cc
		}
	}
	return &uResource{obj: r.obj.DeepCopy(), conditions: conds}
}

// gatesEnabled returns a FeatureGates instance with the SelectiveReconciliation
// gate enabled, for tests that need the ignore-field-drift behavior active.
func gatesEnabled(t *testing.T) featuregate.FeatureGates {
	t.Helper()
	gates, err := featuregate.GetFeatureGatesWithOverrides(
		map[string]bool{featuregate.SelectiveReconciliation: true},
	)
	require.NoError(t, err)
	return gates
}

func TestApplyIgnoredFields_NoAnnotations(t *testing.T) {
	desired := newUResource(nil, map[string]interface{}{"description": "from-spec"})
	latest := newUResource(nil, map[string]interface{}{"description": "from-aws"})

	out, err := applyIgnoredFields(desired, latest, gatesEnabled(t))
	require.NoError(t, err)
	// Same instance returned untouched when there are no annotations.
	assert.Equal(t, "from-spec", out.(*uResource).spec()["description"])
}

func TestApplyIgnoredFields_GateDisabled(t *testing.T) {
	desired := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.description"},
		map[string]interface{}{"description": "from-spec"},
	)
	latest := newUResource(nil, map[string]interface{}{"description": "from-aws"})

	// Default gates have SelectiveReconciliation disabled.
	out, err := applyIgnoredFields(desired, latest, featuregate.GetDefaultFeatureGates())
	require.NoError(t, err)
	// Gate off -> desired returned unchanged.
	assert.Equal(t, "from-spec", out.(*uResource).spec()["description"])
}

func TestApplyIgnoredFields_IgnoreWholeField(t *testing.T) {
	desired := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.description"},
		map[string]interface{}{"description": "from-spec", "name": "keep"},
	)
	latest := newUResource(nil, map[string]interface{}{
		"description": "from-aws",
		"name":        "old",
	})

	out, err := applyIgnoredFields(desired, latest, gatesEnabled(t))
	require.NoError(t, err)
	spec := out.(*uResource).spec()
	// Ignored field takes the AWS value so it compares equal (no Update).
	assert.Equal(t, "from-aws", spec["description"])
	// Non-ignored field keeps the desired (spec) value.
	assert.Equal(t, "keep", spec["name"])
	// The original desired resource is NOT mutated.
	assert.Equal(t, "from-spec", desired.spec()["description"])
}

func TestApplyIgnoredFields_IgnoreFieldAbsentInLatest(t *testing.T) {
	desired := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.description"},
		map[string]interface{}{"description": "from-spec"},
	)
	latest := newUResource(nil, map[string]interface{}{})

	out, err := applyIgnoredFields(desired, latest, gatesEnabled(t))
	require.NoError(t, err)
	spec := out.(*uResource).spec()
	// Field absent in latest -> removed from desired copy so it compares equal.
	_, found := spec["description"]
	assert.False(t, found)
}

func TestRestoreIgnoredFields_RestoresDeclaredValue(t *testing.T) {
	desired := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.description"},
		map[string]interface{}{"description": "declared-X", "name": "keep"},
	)
	updated := newUResource(nil, map[string]interface{}{
		"description": "aws-value",
		"name":        "keep",
	})

	err := restoreIgnoredFields(updated, desired, []string{"spec.description"}, gatesEnabled(t))
	require.NoError(t, err)
	spec := updated.spec()
	// The declared value is restored into updated.
	assert.Equal(t, "declared-X", spec["description"])
	// Non-ignored field left intact.
	assert.Equal(t, "keep", spec["name"])
}

func TestRestoreIgnoredFields_DeclaredAbsentRemovesFromUpdated(t *testing.T) {
	desired := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.description"},
		map[string]interface{}{"name": "keep"},
	)
	updated := newUResource(nil, map[string]interface{}{
		"description": "aws-value",
		"name":        "keep",
	})

	err := restoreIgnoredFields(updated, desired, []string{"spec.description"}, gatesEnabled(t))
	require.NoError(t, err)
	spec := updated.spec()
	// Declared value absent -> path removed from updated.
	_, found := spec["description"]
	assert.False(t, found)
	assert.Equal(t, "keep", spec["name"])
}

func TestRestoreIgnoredFields_GateDisabled(t *testing.T) {
	desired := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.description"},
		map[string]interface{}{"description": "declared-X"},
	)
	updated := newUResource(nil, map[string]interface{}{"description": "aws-value"})

	err := restoreIgnoredFields(updated, desired, []string{"spec.description"}, featuregate.GetDefaultFeatureGates())
	require.NoError(t, err)
	// Gate off -> updated left unchanged.
	assert.Equal(t, "aws-value", updated.spec()["description"])
}

func TestRestoreIgnoredFields_DoesNotMutateDesired(t *testing.T) {
	desired := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.description"},
		map[string]interface{}{"description": "declared-X"},
	)
	updated := newUResource(nil, map[string]interface{}{"description": "aws-value"})

	err := restoreIgnoredFields(updated, desired, []string{"spec.description"}, gatesEnabled(t))
	require.NoError(t, err)
	// The declared (desired) resource is NOT mutated.
	assert.Equal(t, "declared-X", desired.spec()["description"])
}

func TestFilterIgnoredDeltaDifferences_RemovesIgnoredPaths(t *testing.T) {
	res := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.tags"},
		nil,
	)
	delta := ackcompare.NewDelta()
	delta.Add("Spec.Tags", "a", "b")
	delta.Add("Spec.Tags.foo", "a", "b")
	delta.Add("Spec.Name", "a", "b")

	FilterIgnoredDeltaDifferences(delta, res, gatesEnabled(t))

	require.Len(t, delta.Differences, 1)
	assert.True(t, delta.Differences[0].Path.Contains("Spec.Name"))
}

func TestFilterIgnoredDeltaDifferences_GateDisabled(t *testing.T) {
	res := newUResource(
		map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.tags"},
		nil,
	)
	delta := ackcompare.NewDelta()
	delta.Add("Spec.Tags", "a", "b")

	FilterIgnoredDeltaDifferences(delta, res, featuregate.GetDefaultFeatureGates())
	// Gate off -> delta untouched.
	assert.Len(t, delta.Differences, 1)
}

func TestFilterIgnoredDeltaDifferences_NoAnnotation(t *testing.T) {
	res := newUResource(nil, nil)
	delta := ackcompare.NewDelta()
	delta.Add("Spec.Tags", "a", "b")

	FilterIgnoredDeltaDifferences(delta, res, gatesEnabled(t))
	// No annotation -> delta untouched.
	assert.Len(t, delta.Differences, 1)
}

func TestGlobalFeatureGates_RoundTrip(t *testing.T) {
	orig := featuregate.GetGlobalFeatureGates()
	defer featuregate.SetGlobalFeatureGates(orig)

	featuregate.SetGlobalFeatureGates(gatesEnabled(t))
	assert.True(t, featuregate.GetGlobalFeatureGates().IsEnabled(featuregate.SelectiveReconciliation))
}

func TestPathHelpers(t *testing.T) {
	desired := newUResource(map[string]string{
		ackv1alpha1.AnnotationIgnoreFieldDrift: " spec.a , spec.b ,, ",
	}, nil)

	assert.Equal(t, []string{"spec.a", "spec.b"}, IgnoreFieldDriftPaths(desired))
	assert.True(t, HasSelectiveReconciliation(desired))
	assert.Equal(t, "Spec.Tags", toDeltaPath("spec.tags"))
}
