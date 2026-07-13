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
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	"github.com/aws-controllers-k8s/runtime/pkg/runtime/iamroleselector"
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
func (r *uResource) RuntimeObject() client.Object                 { return r.obj }

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

	filterIgnoredDeltaDifferences(delta, res, gatesEnabled(t))

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

	filterIgnoredDeltaDifferences(delta, res, featuregate.GetDefaultFeatureGates())
	// Gate off -> delta untouched.
	assert.Len(t, delta.Differences, 1)
}

func TestFilterIgnoredDeltaDifferences_NoAnnotation(t *testing.T) {
	res := newUResource(nil, nil)
	delta := ackcompare.NewDelta()
	delta.Add("Spec.Tags", "a", "b")

	filterIgnoredDeltaDifferences(delta, res, gatesEnabled(t))
	// No annotation -> delta untouched.
	assert.Len(t, delta.Differences, 1)
}

func TestPathHelpers(t *testing.T) {
	desired := newUResource(map[string]string{
		ackv1alpha1.AnnotationIgnoreFieldDrift: " spec.a , spec.b ,, ",
	}, nil)

	assert.Equal(t, []string{"spec.a", "spec.b"}, IgnoreFieldDriftPaths(desired))
	assert.True(t, HasSelectiveReconciliation(desired))
	assert.Equal(t, "Spec.Tags", toDeltaPath("spec.tags"))
}

func TestIsValidFieldPath(t *testing.T) {
	testCases := []struct {
		path  string
		valid bool
	}{
		// Well-formed dotted field paths.
		{"spec.tags", true},
		{"spec.assumeRolePolicyDocument", true},
		{"spec.a.b.c", true},
		{"spec.field_with_underscore", true},
		{"spec", true},
		// Malformed: illegal characters.
		{"spec/tags", false},
		{"spec.tags!", false},
		{"spec.tags-1", false},
		{"spec.tags 1", false},
		{"spec.tags[0]", false}, // array index: sub-element ignore is out of v1 scope
		// Malformed: empty segments.
		{"spec..tags", false},
		{".spec", false},
		{"spec.", false},
		{"", false},
		// Malformed: segment starting with a non-letter.
		{"spec.1tag", false},
		{"1spec.tags", false},
	}
	for _, tc := range testCases {
		assert.Equalf(t, tc.valid, isValidFieldPath(tc.path),
			"isValidFieldPath(%q)", tc.path)
	}
}

func TestMalformedIgnorePaths(t *testing.T) {
	// Mix of well-formed and malformed paths; only the malformed ones are
	// returned, sorted.
	desired := newUResource(map[string]string{
		ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.tags, spec/bad, spec.ok, spec..empty",
	}, nil)
	assert.Equal(t, []string{"spec..empty", "spec/bad"}, malformedIgnorePaths(desired))

	// All well-formed -> nil.
	clean := newUResource(map[string]string{
		ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.tags, spec.description",
	}, nil)
	assert.Nil(t, malformedIgnorePaths(clean))

	// No annotation -> nil.
	assert.Nil(t, malformedIgnorePaths(newUResource(nil, nil)))
}

// iamPolicyDelta mimics the generated per-resource Delta for a resource whose
// spec.policy field is is_iam_policy: it compares that field SEMANTICALLY via
// IAMPolicyDocumentEqual (like the generated code) and every other scalar field
// byte-for-byte, so driftedIgnoredPaths is exercised with the same comparator
// the real generated delta would use.
func iamPolicyDelta(a, b acktypes.AWSResource) *ackcompare.Delta {
	delta := ackcompare.NewDelta()
	sa := a.(*uResource).spec()
	sb := b.(*uResource).spec()
	for k := range sa {
		if k == "policy" {
			if eq, err := ackcompare.IAMPolicyDocumentEqual(sa[k].(string), sb[k].(string)); err == nil && eq {
				continue // semantically equal -> no difference
			}
		}
		if sa[k] != sb[k] {
			delta.Add("Spec."+capFirst(k), sa[k], sb[k])
		}
	}
	return delta
}

// TestDriftedIgnoredPaths_SemanticFields verifies that drift detection for the
// observability log uses the generated per-field comparator (here
// IAMPolicyDocumentEqual for an is_iam_policy field) rather than a byte compare.
// AWS canonicalizes a policy document on read (collapses single-element arrays,
// reorders keys, reindents), so a byte compare would report perpetual, spurious
// drift; the fix reports drift only when the field is genuinely different.
func TestDriftedIgnoredPaths_SemanticFields(t *testing.T) {
	const declared = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":["ec2.amazonaws.com"]},"Action":["sts:AssumeRole"]}]}`
	// Same policy AWS-canonicalized (byte-different, semantically equal).
	const canonicalized = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	// A genuinely different policy: lambda added as an allowed principal.
	const drifted = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":["ec2.amazonaws.com","lambda.amazonaws.com"]},"Action":"sts:AssumeRole"}]}`

	testCases := []struct {
		name        string
		latestValue string
		wantDrift   []string
	}{
		{"canonicalized only is not drift", canonicalized, nil},
		{"semantic change is drift", drifted, []string{"spec.policy"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec, _, rd, _, _ := ignoreDriftReconcilerMocks(t, true)
			rd.On("Delta", mock.Anything, mock.Anything).Return(iamPolicyDelta)

			desired := newUResource(
				map[string]string{ackv1alpha1.AnnotationIgnoreFieldDrift: "spec.policy"},
				map[string]interface{}{"policy": declared},
			)
			latest := newUResource(nil, map[string]interface{}{"policy": tc.latestValue})

			assert.Equal(t, tc.wantDrift, rec.driftedIgnoredPaths(desired, latest))
		})
	}
}

// -----------------------------------------------------------------------------
// Reconciler-level behavior tests for ignore-field-drift.
//
// These tests drive the reconciler end to end (rec.Sync / lateInitializeResource
// / the write-back patch) rather than the helper functions in isolation, so they
// verify the *composed* behavior: create, drift suppression, anti-clobber, the
// retain write-back, late-init persistence, and gate-off. They use `uResource`
// (the unstructured-backed AWSResource defined above) so real Spec values and
// annotations flow through the unstructured converters used by
// applyIgnoredFields / restoreIgnoredFields; the mocked AWSResourceDescriptor.Delta
// computes a real spec-field-level delta so the reconciler's Update-vs-no-Update
// branching is driven by the actual (possibly drift-suppressed) spec values.
//
// The behaviors under test, as a function of (annotation present?, Spec value,
// late-init configured?). "Ignored" means the field is named in the
// services.k8s.aws/ignore-field-drift annotation AND the gate is enabled.
//
//   annotation | Spec | late-init | behavior verified                          | test
//   -----------|------|-----------|--------------------------------------------|--------------------------------------
//   absent     | X    | no        | drift reconciled back to X (Update fires)  | Unannotated_DriftReconciled
//   absent     | nil  | no        | no drift, no Update                         | Unannotated_NilSpecNoUpdate
//   absent     | nil  | yes       | late-init writes AWS default D into Spec   | Unannotated_LateInitWritesValue
//   present    | X    | no        | create still sends declared X (no suppress)| Ignored_CreateStillSendsDeclaredValue
//   present    | X    | no        | ignored-only drift suppresses the Update   | Ignored_DriftOnlySuppressesUpdate
//   present    | X    | no        | Update on another field: send AWS value    | Ignored_UpdateAntiClobberAndRetain
//              |      |           |  (anti-clobber), patch retains declared X  |
//   present    | Y    | yes       | edited declared value Y retained, not E/X  | Ignored_DeclaredEditRetained
//   present    | nil  | no        | drift suppressed, Spec stays nil           | Ignored_NilSpecDriftSuppressed
//   present    | nil  | yes       | late-init still writes D; later drift kept | Ignored_LateInitNotOverridden
//   present    | nil  | yes       | late-init value D persisted to stored CR   | Ignored_LateInitValuePersisted (regression)
//   present*   | X    | no        | gate OFF: annotation has no effect         | GateOff_AnnotationHasNoEffect
//
// The two regression cases (Ignored_DeclaredEditRetained,
// Ignored_LateInitValuePersisted) fail against the pre-fix code and pass after;
// see their per-test comments.
// -----------------------------------------------------------------------------

const driftField = "description"

// specDelta produces a real Delta by comparing the top-level scalar entries of
// the "spec" maps of two uResources. Only the keys we use in these tests are
// compared, which is sufficient to drive the reconciler's Update branching.
func specDelta(a, b acktypes.AWSResource) *ackcompare.Delta {
	delta := ackcompare.NewDelta()
	ua, aok := a.(*uResource)
	ub, bok := b.(*uResource)
	if !aok || !bok {
		return delta
	}
	sa := ua.spec()
	sb := ub.spec()
	keys := map[string]struct{}{}
	for k := range sa {
		keys[k] = struct{}{}
	}
	for k := range sb {
		keys[k] = struct{}{}
	}
	for k := range keys {
		va := sa[k]
		vb := sb[k]
		if va != vb {
			// Capitalize first segment to match generated Delta path form.
			path := "Spec." + capFirst(k)
			delta.Add(path, va, vb)
		}
	}
	return delta
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// ignoreDriftReconcilerMocks builds a reconciler wired with mocked
// collaborators and the SelectiveReconciliation feature gate set to
// `gateEnabled`. The returned descriptor mock's Delta computes a real
// spec-level delta via specDelta so the reconciler branches realistically.
func ignoreDriftReconcilerMocks(
	t *testing.T,
	gateEnabled bool,
) (
	*resourceReconciler,
	*ackmocks.AWSResourceManager,
	*ackmocks.AWSResourceDescriptor,
	*ctrlrtclientmock.Client,
	acktypes.ServiceControllerMetadata,
) {
	t.Helper()

	zapOptions := ctrlrtzap.Options{Development: true, Level: zapcore.InfoLevel}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))

	cfg := ackcfg.Config{
		FeatureGates: featuregate.FeatureGates{
			featuregate.ReadOnlyResources:       {Enabled: true},
			featuregate.ResourceAdoption:        {Enabled: true},
			featuregate.SelectiveReconciliation: {Enabled: gateEnabled},
		},
		ResourceTagKeys: []string{},
	}
	metrics := ackmetrics.NewMetrics("bookstore")

	sc := &ackmocks.ServiceController{}
	scmd := acktypes.ServiceControllerMetadata{}
	sc.On("GetMetadata").Return(scmd)

	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		k8srtschema.GroupVersionKind{
			Group: "bookstore.services.k8s.aws", Kind: "fakeBook",
		},
	)
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	kc := &ctrlrtclientmock.Client{}

	rec := &resourceReconciler{
		reconciler: reconciler{
			sc:        sc,
			kc:        kc,
			log:       fakeLogger.WithName("ackrt"),
			cfg:       cfg,
			metrics:   metrics,
			carmCache: ackrtcache.Caches{},
			irsCache:  &iamroleselector.Cache{},
		},
		rmf: rmf,
		rd:  rd,
	}

	rm := &ackmocks.AWSResourceManager{}
	return rec, rm, rd, kc, scmd
}

// newDriftRes builds a uResource with bookstore-like metadata so the reconciler
// machinery (conditions, GVK) operates normally. When ignore is true, the
// ignore-field-drift annotation is set for spec.description.
func newDriftRes(ignore bool, spec map[string]interface{}) *uResource {
	annotations := map[string]string{}
	if ignore {
		annotations[ackv1alpha1.AnnotationIgnoreFieldDrift] = "spec." + driftField
	}
	r := newUResource(annotations, spec)
	r.obj.SetNamespace("default")
	r.obj.SetName("mybook")
	r.obj.SetGeneration(1)
	// Mark managed: the reconciler's update path requires a finalizer.
	r.obj.SetFinalizers([]string{"finalizers.bookstore.services.k8s.aws/Book"})
	return r
}

// specOf returns the spec map of an AWSResource (assumed *uResource).
func specOf(r acktypes.AWSResource) map[string]interface{} {
	return r.(*uResource).spec()
}

// wireDescriptorCommon wires the descriptor methods used by the reconciler's
// update / patch path. IsManaged returns true (resource carries a finalizer).
// Delta computes a real spec-level delta. MarkAdopted is a no-op.
func wireDescriptorCommon(rd *ackmocks.AWSResourceDescriptor) {
	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("MarkAdopted", mock.Anything).Return()
	rd.On("Delta", mock.Anything, mock.Anything).Return(
		func(a, b acktypes.AWSResource) *ackcompare.Delta {
			return specDelta(a, b)
		},
	)
}

// wireManagerCommon wires the resource-manager methods that are invoked
// regardless of create/update flow. ResolveReferences and EnsureTags are
// pass-throughs. ClearResolvedReferences / FilterSystemTags are no-ops.
func wireManagerCommon(
	rm *ackmocks.AWSResourceManager,
	scmd acktypes.ServiceControllerMetadata,
) {
	rm.On("ResolveReferences", mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ client.Reader, r acktypes.AWSResource) acktypes.AWSResource { return r },
		false,
		nil,
	)
	rm.On("EnsureTags", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	rm.On("ClearResolvedReferences", mock.Anything).Return(
		func(r acktypes.AWSResource) acktypes.AWSResource { return r },
	)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(true, nil)
}

// =============================================================================
// Unannotated, Spec=X, no late-init.
// Expectation: normal behavior. AWS has drifted to E; the reconciler detects
// the delta and calls Update to reconcile the field back to X.
// =============================================================================
func TestIgnoreFieldDrift_Unannotated_DriftReconciled(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true /* gate on, but no annotation */)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(false, map[string]interface{}{driftField: "X"})
	latest := newDriftRes(false, map[string]interface{}{driftField: "E"})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)

	var updateDesired acktypes.AWSResource
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, d, _ acktypes.AWSResource, _ *ackcompare.Delta) acktypes.AWSResource {
			updateDesired = d
			return latest
		}, nil,
	)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Update IS called (drift reconciled) and carries X (the declared value).
	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.NotNil(updateDesired)
	assert.Equal(t, "X", specOf(updateDesired)[driftField])
}

// =============================================================================
// Unannotated, Spec=nil, no late-init.
// Expectation: normal, unchanged behavior. No drift (latest also nil) -> no
// Update.
// =============================================================================
func TestIgnoreFieldDrift_Unannotated_NilSpecNoUpdate(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(false, map[string]interface{}{"name": "keep"})
	latest := newDriftRes(false, map[string]interface{}{"name": "keep"})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// No delta -> no Update.
	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// =============================================================================
// Unannotated, Spec=nil, late-init writes D.
// Expectation: late-init writes D into the spec; normal behavior. We assert
// LateInitialize runs and its result (carrying D) is patched back.
// =============================================================================
func TestIgnoreFieldDrift_Unannotated_LateInitWritesValue(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(false, map[string]interface{}{"name": "keep"})
	// latest (from ReadOne) has no description; AWS hasn't drifted.
	latest := newDriftRes(false, map[string]interface{}{"name": "keep"})
	// late-init result carries D in the description field.
	lateInited := newDriftRes(false, map[string]interface{}{"name": "keep", driftField: "D"})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(lateInited, nil)

	var patched acktypes.AWSResource
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		// The runtime object being patched is the late-inited resource's object.
	}).Return(nil)
	_ = patched

	out, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	rm.AssertCalled(t, "LateInitialize", mock.Anything, mock.Anything)
	// The returned (latest) resource is the late-inited one carrying D.
	assert.Equal(t, "D", specOf(out)[driftField])
}

// =============================================================================
// Ignored (create): annotation present, Spec=X, no late-init.
// Expectation: the resource passed to rm.Create carries X (the field is still
// created from Spec — annotation only suppresses *drift*, not creation).
// =============================================================================
func TestIgnoreFieldDrift_Ignored_CreateStillSendsDeclaredValue(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(true, map[string]interface{}{driftField: "X"})
	created := newDriftRes(true, map[string]interface{}{driftField: "X"})

	// ReadOne returns NotFound first (triggers create), then returns the
	// created resource for the post-create ReadOne.
	rm.On("ReadOne", mock.Anything, desired).Return(nil, ackerr.NotFound).Once()
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(created, nil)

	var createInput acktypes.AWSResource
	rm.On("Create", mock.Anything, mock.Anything).Return(
		func(_ context.Context, d acktypes.AWSResource) acktypes.AWSResource {
			createInput = d
			return created
		}, nil,
	)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(created, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	rm.AssertCalled(t, "Create", mock.Anything, mock.Anything)
	require.NotNil(createInput)
	// Create input carries X for the ignored field (baseline creation).
	assert.Equal(t, "X", specOf(createInput)[driftField])
}

// =============================================================================
// Ignored (suppress): annotation present, Spec=X, AWS drifted to E, ONLY the
// ignored field differs.
// Expectation: drift suppressed -> rm.Update is NOT called.
// =============================================================================
func TestIgnoreFieldDrift_Ignored_DriftOnlySuppressesUpdate(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(true, map[string]interface{}{driftField: "X"})
	latest := newDriftRes(true, map[string]interface{}{driftField: "E"})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Only the ignored field drifted -> applyIgnoredFields merges E into the
	// reconcile copy so the delta is empty -> Update NOT called.
	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// =============================================================================
// Ignored (anti-clobber + retain) — THE CENTRAL PROOF.
// annotation present, Spec=X (ignored field), AWS drifted to E, AND a separate
// NON-ignored field ("name") also differs so an Update is forced.
// Expectations:
//   - Update IS called (the non-ignored field forces it).
//   - The resource passed to Update carries the AWS value E for the ignored
//     field (anti-clobber: we do NOT clobber the AWS-observed drift).
//   - The resource patched back to k8s retains X for the ignored field
//     (retain: the user's declared value is never overwritten in the CR spec).
//
// =============================================================================
func TestIgnoreFieldDrift_Ignored_UpdateAntiClobberAndRetain(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	// desired: ignored field=X, name=desired-name
	desired := newDriftRes(true, map[string]interface{}{
		driftField: "X",
		"name":     "desired-name",
	})
	// latest (AWS): ignored field drifted to E, name=aws-name (non-ignored diff)
	latest := newDriftRes(true, map[string]interface{}{
		driftField: "E",
		"name":     "aws-name",
	})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)

	var updateInput acktypes.AWSResource
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, d, _ acktypes.AWSResource, _ *ackcompare.Delta) acktypes.AWSResource {
			updateInput = d
			// rm.Update returns the resource as it would observe it post-update.
			// Simulate AWS normalizing the non-ignored field to "aws-name" so the
			// write-back patch fires on a non-ignored field. The ignored field
			// still carries the AWS-observed value E in the returned resource;
			// restoreIgnoredFields must rewrite it to the declared X before patch.
			out := d.DeepCopy()
			_ = unstructured.SetNestedField(
				out.(*uResource).obj.Object, "aws-name", "spec", "name",
			)
			return out
		}, nil,
	)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(
		func(_ context.Context, r acktypes.AWSResource) acktypes.AWSResource { return r }, nil,
	)

	// Capture the resource that is patched back to k8s. The reconciler patches
	// latestCleaned.RuntimeObject(), which (for uResource) is the underlying
	// *unstructured.Unstructured; we read the spec from the captured object.
	var patchedSpec map[string]interface{}
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		if u, ok := args.Get(1).(*unstructured.Unstructured); ok {
			if s, found, _ := unstructured.NestedMap(u.Object, "spec"); found {
				patchedSpec = s
			}
		}
	}).Return(nil)

	out, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Update IS called because the non-ignored "name" field differs.
	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.NotNil(updateInput)

	// ANTI-CLOBBER: the Update request carries the AWS-observed value E for the
	// ignored field (we did NOT overwrite the drift with X).
	assert.Equal(t, "E", specOf(updateInput)[driftField],
		"anti-clobber: Update input must carry the AWS value E for the ignored field")
	// The non-ignored field carries the declared value.
	assert.Equal(t, "desired-name", specOf(updateInput)["name"])

	// RETAIN: the resource returned from Sync (and patched back) retains X for
	// the ignored field — the user's declared value is never overwritten.
	assert.Equal(t, "X", specOf(out)[driftField],
		"retain: post-update resource must retain the declared value X for the ignored field")
	// RETAIN, verified at the k8s patch boundary: a write-back patch fires
	// (the non-ignored field was normalized by AWS to "aws-name"), and the spec
	// patched back to the API server carries X (NOT the AWS value E) for the
	// ignored field — restoreIgnoredFields rewrote it before the patch.
	require.NotNil(patchedSpec,
		"expected a write-back patch to fire for the non-ignored field")
	assert.Equal(t, "X", patchedSpec[driftField],
		"retain: patched-back spec must carry the declared value X for the ignored field")
	assert.Equal(t, "aws-name", patchedSpec["name"],
		"the non-ignored field is reconciled to the AWS-normalized value")
}

// =============================================================================
// Ignored (declared-edit retained) — REGRESSION GUARD for the e2e
// `test_tags_drift_ignored` failure.
//
// annotation present for the ignored field; the user has EDITED a DECLARED-SET
// ignored field from a prior value X to Y (so the stored desired now carries
// Y); AWS still has the old value E for it. CRUCIALLY the resource ALSO has a
// (separate) late-init field, so LateInitialize sets the RESOURCE-level
// ACK.LateInitialized condition — exactly the production IAM Role case where
// spec.path is late-init and spec.tags is the ignored, user-edited field.
//
// A separate NON-ignored field ("name") also differs so an Update is forced.
// After Update, lateInitializeResource runs: `latest`/`lateInited` carry the
// AWS value E on the ignored path (read returned it / late-init was a no-op for
// the path) and the LateInitialized condition is set.
//
// The prior fix used the RESOURCE-level LateInitialized condition as the persist
// discriminator and applied it to ALL ignored paths. For the user-edited
// ignored path that produces: rebaseIgnoredFieldsForPersist rebases the base to
// Y (declared≠lateInited) and ignoredFieldNeedsPersist sees base=Y vs target=E
// differ -> fires a patch whose MergeFrom(base=Y).Data(target=E) body SETS
// spec.<ignored>=E, clobbering the user's edit Y back to the AWS value. That is
// the regression.
//
// Correct behavior (retain): a DECLARED-PRESENT ignored path is owned by the
// normal retain path. The persist logic must fire ONLY for declared-ABSENT
// paths late-init filled, so the merge patch body must NEVER set the ignored
// field to E (AWS) or X (prior); the stored value stays Y.
//
// This FAILS against the pre-fix code (patch body sets spec.<ignored>=E) and
// PASSES after the fix.
// =============================================================================
func TestIgnoreFieldDrift_Ignored_DeclaredEditRetained(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)

	// The descriptor's Delta returns the RAW spec delta, mirroring the
	// generated newResourceDelta. The reconciler's filteredDelta wrapper
	// strips the ignored-field difference from every delta-driven decision,
	// so the patch on the late-init path is NOT spuriously driven by the
	// (filtered) delta, isolating the persist-gate / rebase logic under test.
	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("MarkAdopted", mock.Anything).Return()
	rd.On("Delta", mock.Anything, mock.Anything).Return(
		func(a, b acktypes.AWSResource) *ackcompare.Delta {
			return specDelta(a, b)
		},
	)
	wireManagerCommon(rm, scmd)

	// desired (stored CR): the user has EDITED the ignored field to Y (it was
	// previously X). A non-ignored field also carries the declared value.
	desired := newDriftRes(true, map[string]interface{}{
		driftField: "Y",
		"name":     "desired-name",
	})
	// latest (AWS): ignored field still has the old value E, name=aws-name
	// (a non-ignored diff that forces an Update).
	latest := newDriftRes(true, map[string]interface{}{
		driftField: "E",
		"name":     "aws-name",
	})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)

	var updateInput acktypes.AWSResource
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, d, _ acktypes.AWSResource, _ *ackcompare.Delta) acktypes.AWSResource {
			updateInput = d
			// rm.Update returns the resource as it would observe it post-update:
			// the ignored field still carries the AWS-observed value E (the edit
			// to the ignored field is NOT pushed to AWS by design).
			out := d.DeepCopy()
			_ = unstructured.SetNestedField(
				out.(*uResource).obj.Object, "E", "spec", driftField,
			)
			_ = unstructured.SetNestedField(
				out.(*uResource).obj.Object, "aws-name", "spec", "name",
			)
			return out
		}, nil,
	)
	// LateInitialize is a no-op for the ignored path but sets the RESOURCE-level
	// ACK.LateInitialized condition (the resource has a separate late-init field,
	// e.g. spec.path on an IAM Role). The returned object carries E on the
	// ignored path, mirroring the AWS-observed read value.
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(
		func(_ context.Context, r acktypes.AWSResource) acktypes.AWSResource {
			out := r.DeepCopy().(*uResource)
			_ = unstructured.SetNestedField(out.obj.Object, "E", "spec", driftField)
			return out.withLateInitializedCondition()
		}, nil,
	)

	// Capture the MERGE PATCH BODY (the diff actually sent to the API server),
	// not the object being patched. Only the body reveals what is WRITTEN to the
	// stored CR for the ignored field.
	var patchBodies []map[string]interface{}
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		obj, _ := args.Get(1).(client.Object)
		patch, _ := args.Get(2).(client.Patch)
		if obj == nil || patch == nil {
			return
		}
		data, derr := patch.Data(obj)
		if derr != nil {
			return
		}
		var body map[string]interface{}
		if json.Unmarshal(data, &body) == nil {
			patchBodies = append(patchBodies, body)
		}
	}).Return(nil)

	out, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Update IS called because the non-ignored "name" field differs.
	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.NotNil(updateInput)

	_ = out

	// PATCH BOUNDARY (the central proof): no merge patch body may ever WRITE the
	// ignored field. The patch base on every write-back is the declared desired,
	// which already carries the user's edit Y, so the ignored field must produce
	// no diff and must be absent from every patch body. In particular the body
	// must NEVER set the ignored field to the AWS value E (the regression) or to
	// the prior value X; the stored CR therefore retains Y.
	//
	// This asserts on the merge patch BODY rather than specOf(out): the in-memory
	// object returned from Sync carries the AWS-observed late-init value E on the
	// ignored path (the rebase fix deliberately leaves the target object
	// untouched and only adjusts the patch BASE), but what is PERSISTED to the
	// stored CR is the patch body, which never writes the ignored field — so the
	// CR read-back the e2e checks retains Y.
	require.NotEmpty(patchBodies, "expected a write-back patch to fire for the non-ignored field")
	for _, body := range patchBodies {
		v, found, _ := unstructured.NestedString(body, "spec", driftField)
		assert.False(t, found,
			"declared-edit retain: the merge patch body must NOT write the ignored field "+
				"(got value %q); the declared edit Y is retained, never clobbered to E or X", v)
	}
}

// =============================================================================
// Ignored, Spec=nil, no late-init, AWS has a value (drift).
// Expectation: create sends nothing for the ignored field; drift suppressed;
// spec stays nil. Here we exercise the update path (resource already exists)
// with the ignored field absent in desired and present (D) in latest.
// =============================================================================
func TestIgnoreFieldDrift_Ignored_NilSpecDriftSuppressed(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	// desired: ignored field absent (nil).
	desired := newDriftRes(true, map[string]interface{}{"name": "keep"})
	// latest (AWS): ignored field present with value D.
	latest := newDriftRes(true, map[string]interface{}{"name": "keep", driftField: "D"})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)

	// Capture every spec patched back to k8s so we can prove the AWS value D is
	// never written to the CR spec for the ignored field.
	var patchedSpecs []map[string]interface{}
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		if u, ok := args.Get(1).(*unstructured.Unstructured); ok {
			if s, found, _ := unstructured.NestedMap(u.Object, "spec"); found {
				patchedSpecs = append(patchedSpecs, s)
			}
		}
	}).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Only the ignored field differs (desired absent vs latest=D). Drift is
	// suppressed -> Update NOT called.
	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	// The AWS value D is never patched back to the CR spec for the ignored
	// field — the declared (absent/nil) state is retained in etcd.
	for _, s := range patchedSpecs {
		_, found := s[driftField]
		assert.False(t, found, "AWS value D must never be patched back to the CR spec")
	}
}

// =============================================================================
// Ignored, Spec=nil, late-init writes D.
// Expectation: late-init still writes D to spec (late-init is NOT overridden by
// the annotation); later drift is suppressed; the spec retains D.
// =============================================================================
func TestIgnoreFieldDrift_Ignored_LateInitNotOverridden(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	// This faithfully reproduces the PRODUCTION late-init-wins scenario that the
	// e2e exercises and that the previous regression test did NOT capture:
	//
	//   - The DECLARED CR (`desired`, what is stored in etcd) leaves the ignored
	//     field unset.
	//   - The AWS-observed `latest` (from ReadOne) ALREADY carries the
	//     service-defaulted value D for the ignored field (the field is returned
	//     by the Read API, as 150/170 late-init fields are). Consequently
	//     LateInitialize is a no-op for the path: `lateInited` also carries D.
	//   - LateInitialize sets the ACK.LateInitialized condition, marking D as a
	//     late-initialized (service-defaulted) value rather than plain drift.
	//
	// With `latest` as the patch base (pre-fix), base==target on the ignored
	// path, the MergeFrom diff is empty, and D is NEVER persisted to the stored
	// (unset) CR. The fix rebases the ignored late-init path onto the declared
	// (unset) value so the MergeFrom patch carries spec.description=D.

	// desired (stored CR): ignored field absent (nil).
	desired := newDriftRes(true, map[string]interface{}{"name": "keep"})
	// latest (ReadOne): ignored field ALREADY carries the service default D.
	latest := newDriftRes(true, map[string]interface{}{"name": "keep", driftField: "D"})
	// late-init result: no-op for the path (D already present), but late-init
	// is configured so the LateInitialized condition is set.
	lateInited := newDriftRes(true, map[string]interface{}{"name": "keep", driftField: "D"}).
		withLateInitializedCondition()

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(lateInited, nil)

	// Capture the MERGE PATCH BODY (the diff actually sent to the API server),
	// not the object being patched. The object passed to Patch is the
	// late-inited resource, which always carries D in its spec; only the patch
	// body reveals whether spec.description=D is actually WRITTEN to the stored
	// CR. The body is computed from the patch base (which the fix rebases onto
	// the declared, unset value) versus the target object.
	var patchBodies []map[string]interface{}
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		obj, _ := args.Get(1).(client.Object)
		patch, _ := args.Get(2).(client.Patch)
		if obj == nil || patch == nil {
			return
		}
		data, derr := patch.Data(obj)
		if derr != nil {
			return
		}
		var body map[string]interface{}
		if json.Unmarshal(data, &body) == nil {
			patchBodies = append(patchBodies, body)
		}
	}).Return(nil)

	out, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Update NOT called: only the ignored field differs (and it's suppressed).
	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	rm.AssertCalled(t, "LateInitialize", mock.Anything, mock.Anything)

	// PATCH BOUNDARY: a write-back patch must SET spec.description=D, persisting
	// the late-init value to the previously-unset declared CR. This is the
	// assertion that FAILS before the fix (the patch body does not carry D
	// because the base==target on the ignored path) and PASSES after.
	persisted := false
	for _, body := range patchBodies {
		if s, found, _ := unstructured.NestedString(body, "spec", driftField); found && s == "D" {
			persisted = true
		}
	}
	assert.True(t, persisted,
		"late-init value D must be persisted to the stored CR spec for the ignored field "+
			"(the merge patch body must SET spec.description=D)")

	// The resource returned from Sync carries D as well.
	assert.Equal(t, "D", specOf(out)[driftField],
		"late-init value D must survive (late-init is not overridden)")
}

// =============================================================================
// Regression: late-init write-back of an IGNORED field must be persisted to the
// CR spec, even though the generated Delta filters out the ignored-field diff.
//
// This test mirrors PRODUCTION more closely than the other tests here on TWO
// axes the previous version missed:
//
//  1. The reconciler's filteredDelta wrapper (which applies
//     filterIgnoredDeltaDifferences on top of the descriptor's raw Delta)
//     strips the ignored-field difference from every delta-driven decision.
//  2. The AWS-observed `latest` (from ReadOne) ALREADY carries the
//     service-defaulted value D for the ignored field — the realistic case for
//     a late-init field that is returned by the Read API. This is the crucial
//     STORED-CR-vs-AWS-latest distinction: the DECLARED CR (`desired`) is
//     unset, while `latest` (the late-init patch base) is not. The previous
//     test wired `latest` unset, so the diff base==latest already carried the
//     value and the bug was masked.
//
// Scenario:
//
//   - annotation present for spec.description (the ignored field)
//   - desired (stored CR): ignored field unset
//   - ReadOne (latest): ignored field ALREADY = D
//   - LateInitialize: no-op for the path, but sets ACK.LateInitialized
//
// With `latest` as the late-init patch base, the base and target agree on the
// ignored path AND the filtered delta reports "no difference", so pre-fix
// nothing is patched and D never reaches the stored (unset) CR. The fix rebases
// the ignored late-init path onto the declared (unset) value, so the MergeFrom
// patch body SETS spec.description=D.
//
// This test FAILS against the pre-fix code and PASSES after the fix.
// =============================================================================
func TestIgnoreFieldDrift_Ignored_LateInitValuePersisted(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)

	// The descriptor's Delta returns the RAW spec delta, mirroring the
	// generated newResourceDelta. The reconciler's filteredDelta wrapper
	// strips differences on the ignored field from every delta-driven
	// decision.
	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("MarkAdopted", mock.Anything).Return()
	rd.On("Delta", mock.Anything, mock.Anything).Return(
		func(a, b acktypes.AWSResource) *ackcompare.Delta {
			return specDelta(a, b)
		},
	)
	wireManagerCommon(rm, scmd)

	// desired (stored CR): ignored field unset.
	desired := newDriftRes(true, map[string]interface{}{"name": "keep"})
	// latest (ReadOne): ignored field ALREADY carries the service default D.
	latest := newDriftRes(true, map[string]interface{}{"name": "keep", driftField: "D"})
	// late-init result: no-op for the path, but late-init is configured so the
	// LateInitialized condition is set.
	lateInited := newDriftRes(true, map[string]interface{}{"name": "keep", driftField: "D"}).
		withLateInitializedCondition()

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(lateInited, nil)

	// Capture the MERGE PATCH BODY (the diff sent to the API server), not the
	// object being patched (which always carries D). Only the body reveals
	// whether spec.description=D is actually written to the stored CR.
	var patchBodies []map[string]interface{}
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		obj, _ := args.Get(1).(client.Object)
		patch, _ := args.Get(2).(client.Patch)
		if obj == nil || patch == nil {
			return
		}
		data, derr := patch.Data(obj)
		if derr != nil {
			return
		}
		var body map[string]interface{}
		if json.Unmarshal(data, &body) == nil {
			patchBodies = append(patchBodies, body)
		}
	}).Return(nil)

	out, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Update NOT called: only the ignored field would differ, and it's filtered.
	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	rm.AssertCalled(t, "LateInitialize", mock.Anything, mock.Anything)

	// The late-init value D must be persisted to the CR spec at the patch
	// boundary, despite the filtered delta reporting "no difference" AND despite
	// `latest` already carrying D.
	persisted := false
	for _, body := range patchBodies {
		if s, found, _ := unstructured.NestedString(body, "spec", driftField); found && s == "D" {
			persisted = true
		}
	}
	assert.True(t, persisted,
		"late-init value D for the ignored field must be persisted to the CR spec "+
			"(the merge patch body must SET spec.description=D)")
	// And the resource returned from Sync carries D as well.
	assert.Equal(t, "D", specOf(out)[driftField],
		"resource returned from Sync must carry the late-init value D")
}

// =============================================================================
// Gate off: annotation present but SelectiveReconciliation disabled.
// Expectation: the annotation is ignored entirely -> behaves like row 1, i.e.
// drift on the (would-be ignored) field is reconciled, so rm.Update IS called
// and carries the declared value X.
// =============================================================================
func TestIgnoreFieldDrift_GateOff_AnnotationHasNoEffect(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, false /* gate OFF */)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(true, map[string]interface{}{driftField: "X"})
	latest := newDriftRes(true, map[string]interface{}{driftField: "E"})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)

	var updateInput acktypes.AWSResource
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, d, _ acktypes.AWSResource, _ *ackcompare.Delta) acktypes.AWSResource {
			updateInput = d
			return latest
		}, nil,
	)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	// Gate off: drift is reconciled -> Update IS called, carrying declared X.
	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.NotNil(updateInput)
	assert.Equal(t, "X", specOf(updateInput)[driftField])
}

// =============================================================================
// Ignored (custom sdkUpdate that BUILDS ITS OWN SDK REQUEST) — verifies the
// "customUpdate could clobber" concern is a non-issue for the standard pattern.
//
// This models what a real custom `sdkUpdate` does: it does NOT just hand back
// the resource it was given — it constructs an SDK request payload field-by-
// field from the `desired` argument it received, calls the API, and returns a
// result. We capture the value that would be sent "to AWS" for the ignored
// field and assert it is the OBSERVED value E (a no-op re-send), never the
// user's declared X (which would clobber the external change).
//
// A non-ignored field ("name") also drifts, so an Update is genuinely forced
// (the ignored-only case is covered separately by DriftOnlySuppressesUpdate).
// =============================================================================
func TestIgnoreFieldDrift_Ignored_CustomUpdateBuildsRequestFromDesired(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(true, map[string]interface{}{
		driftField: "X",            // ignored field, user-declared
		"name":     "desired-name", // non-ignored field, forces the Update
	})
	latest := newDriftRes(true, map[string]interface{}{
		driftField: "E",        // AWS drifted the ignored field out-of-band
		"name":     "aws-name", // non-ignored field also differs
	})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)

	// sentToAWS captures the request payload a realistic custom sdkUpdate would
	// build. The KEY LINE is `payload[driftField] = specOf(d)[driftField]`: the
	// custom method sources the ignored field's value from the `desired` (d)
	// argument it was handed -- exactly the pattern every audited controller
	// uses -- NOT from a fresh read or the API response.
	var sentToAWS map[string]interface{}
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, d, _ acktypes.AWSResource, _ *ackcompare.Delta) acktypes.AWSResource {
			payload := map[string]interface{}{}
			for k, v := range specOf(d) { // build request from the passed-in desired
				payload[k] = v
			}
			sentToAWS = payload
			return d.DeepCopy()
		}, nil,
	)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(
		func(_ context.Context, r acktypes.AWSResource) acktypes.AWSResource { return r }, nil,
	)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	require.NotNil(sentToAWS, "custom update should have run (a non-ignored field drifted)")
	// THE PROOF: the value the custom update sends to AWS for the ignored field
	// is the OBSERVED value E, not the declared X -> re-sending E is a no-op, so
	// the external change is NOT clobbered.
	assert.Equal(t, "E", sentToAWS[driftField],
		"custom update built from `desired` sends the observed value for the "+
			"ignored field (no-op); it must NOT send the declared value (clobber)")
	// The non-ignored field is sent as declared (normal reconciliation).
	assert.Equal(t, "desired-name", sentToAWS["name"])
}

// =============================================================================
// Ignored (custom sdkUpdate sourcing from `latest`) — verifies that even the
// pattern that looks risky (reading the field from the observed `latest`
// instead of `desired`) is still safe, because for an ignored field the merge
// makes desired == latest. This is why the fleet audit classified controllers
// that read from `latest` (e.g. lambda's "resend current code") as safe.
//
// It also pins the boundary: the ONLY value that would clobber is one that is
// NEITHER the declared X NOR the observed E (a synthesized/stale value) -- a
// pattern no audited controller exhibits.
// =============================================================================
func TestIgnoreFieldDrift_Ignored_CustomUpdateFromLatestIsAlsoSafe(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, true)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)

	desired := newDriftRes(true, map[string]interface{}{
		driftField: "X",
		"name":     "desired-name",
	})
	latest := newDriftRes(true, map[string]interface{}{
		driftField: "E",
		"name":     "aws-name",
	})

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)

	// This custom update sources the ignored field from `latest` (the observed
	// state) rather than `desired`. It sends E -- which equals what AWS already
	// has -- so it is still a no-op, no clobber.
	var sentToAWS map[string]interface{}
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, d, l acktypes.AWSResource, _ *ackcompare.Delta) acktypes.AWSResource {
			sentToAWS = map[string]interface{}{
				driftField: specOf(l)[driftField], // sourced from LATEST, not desired
				"name":     specOf(d)["name"],
			}
			return d.DeepCopy()
		}, nil,
	)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(
		func(_ context.Context, r acktypes.AWSResource) acktypes.AWSResource { return r }, nil,
	)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(ctx, rm, desired)
	require.NoError(err)

	require.NotNil(sentToAWS)
	// Reading from `latest` sends the observed value E == what AWS holds -> no-op.
	assert.Equal(t, "E", sentToAWS[driftField],
		"sourcing the ignored field from `latest` sends the observed value, "+
			"which equals what AWS already has -- still a no-op, no clobber")
}
