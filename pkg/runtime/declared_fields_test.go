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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// --- typed test resource ----------------------------------------------------

type dfLogging struct {
	Level                *string   `json:"level,omitempty"`
	IncludeExecutionData *bool     `json:"includeExecutionData,omitempty"`
	Destinations         []*string `json:"destinations,omitempty"`
}

type dfInline struct {
	InlinedField *string `json:"inlinedField,omitempty"`
}

type dfSpec struct {
	dfInline     `json:",inline"`
	Name         *string                                    `json:"name,omitempty"`
	Description  *string                                    `json:"description,omitempty"`
	KMSKeyID     *string                                    `json:"kmsKeyID,omitempty"`
	Enabled      *bool                                      `json:"enabled,omitempty"`
	Logging      *dfLogging                                 `json:"loggingConfiguration,omitempty"`
	SubnetIDs    []*string                                  `json:"subnetIDs,omitempty"`
	SubnetRefs   []*ackv1alpha1.AWSResourceReferenceWrapper `json:"subnetRefs,omitempty"`
	Env          map[string]*string                         `json:"env,omitempty"`
	CreatedAfter *metav1.Time                               `json:"createdAfter,omitempty"`
	Ignored      *string                                    `json:"-"`
	Policy       *string                                    `json:"policy,omitempty"`
}

type dfCR struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              dfSpec `json:"spec,omitempty"`
}

func (in *dfCR) DeepCopyObject() k8sruntime.Object {
	out := &dfCR{}
	b, _ := json.Marshal(in)
	_ = json.Unmarshal(b, out)
	return out
}

// tResource is a minimal AWSResource backed by a typed object, so the
// reflection walk in declaredSpecPaths sees real Go types.
type tResource struct{ obj *dfCR }

func (r *tResource) Conditions() []*ackv1alpha1.Condition                   { return nil }
func (r *tResource) ReplaceConditions([]*ackv1alpha1.Condition)             {}
func (r *tResource) Identifiers() acktypes.AWSResourceIdentifiers           { return nil }
func (r *tResource) IsBeingDeleted() bool                                   { return false }
func (r *tResource) RuntimeObject() client.Object                           { return r.obj }
func (r *tResource) MetaObject() metav1.Object                              { return &r.obj.ObjectMeta }
func (r *tResource) SetObjectMeta(meta metav1.ObjectMeta)                   { r.obj.ObjectMeta = meta }
func (r *tResource) SetIdentifiers(*ackv1alpha1.AWSIdentifiers) error       { return nil }
func (r *tResource) SetStatus(acktypes.AWSResource)                         {}
func (r *tResource) PopulateResourceFromAnnotation(map[string]string) error { return nil }
func (r *tResource) DeepCopy() acktypes.AWSResource {
	return &tResource{obj: r.obj.DeepCopyObject().(*dfCR)}
}

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

// --- declaredSpecPaths -------------------------------------------------------

func TestDeclaredSpecPaths_Typed(t *testing.T) {
	res := &tResource{obj: &dfCR{Spec: dfSpec{
		dfInline:     dfInline{InlinedField: strp("x")},
		Name:         strp("a"),
		Enabled:      boolp(false), // an explicit false is set
		Logging:      &dfLogging{Level: strp("ERROR")},
		SubnetIDs:    []*string{}, // an explicit empty list is set
		SubnetRefs:   []*ackv1alpha1.AWSResourceReferenceWrapper{{From: &ackv1alpha1.AWSResourceReference{Name: strp("s")}}},
		Env:          map[string]*string{"FOO": strp("bar")},
		CreatedAfter: &metav1.Time{},
		Ignored:      strp("never recorded"),
	}}}
	assert.Equal(t, []string{
		"createdAfter", // has its own JSON encoding: a leaf
		"enabled",
		"env",                  // map keys are not recorded
		"inlinedField",         // embedded struct fields are inlined
		"loggingConfiguration", // structs are descended into...
		"loggingConfiguration.level",
		"name",
		"subnetIDs",
		"subnetRefs", // ...lists are not
	}, declaredSpecPaths(res))
}

func TestDeclaredSpecPaths_Unstructured(t *testing.T) {
	res := newUResource(nil, map[string]interface{}{
		"name":                 "a",
		"loggingConfiguration": map[string]interface{}{"level": "ERROR"},
	})
	assert.Equal(t, []string{"loggingConfiguration", "loggingConfiguration.level", "name"}, declaredSpecPaths(res))
}

// --- record encoding ---------------------------------------------------------

func TestDeclaredFieldsRecord_RoundTripAndDeterministic(t *testing.T) {
	paths := []string{"name", "loggingConfiguration", "loggingConfiguration.level"}
	v1, err := encodeDeclaredFieldsRecord("uid-1", paths)
	require.NoError(t, err)
	v2, err := encodeDeclaredFieldsRecord("uid-1", []string{"loggingConfiguration.level", "name", "loggingConfiguration"})
	require.NoError(t, err)
	assert.Equal(t, v1, v2, "encoding must not depend on input order")
	assert.Equal(t, `{"uid":"uid-1","fields":{"loggingConfiguration":{"level":{}},"name":{}}}`, v1)

	res := &tResource{obj: &dfCR{ObjectMeta: metav1.ObjectMeta{
		UID:         "uid-1",
		Annotations: map[string]string{ackv1alpha1.AnnotationDeclaredFields: v1},
	}}}
	got, ok := readDeclaredFieldsRecord(res)
	require.True(t, ok)
	assert.Equal(t, map[string]struct{}{
		"name": {}, "loggingConfiguration": {}, "loggingConfiguration.level": {},
	}, got)
}

func TestReadDeclaredFieldsRecord_Unusable(t *testing.T) {
	valid, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name"})
	for name, tc := range map[string]struct {
		uid         k8stypes.UID
		annotations map[string]string
	}{
		"missing":     {uid: "uid-1"},
		"garbage":     {uid: "uid-1", annotations: map[string]string{ackv1alpha1.AnnotationDeclaredFields: "{not json"}},
		"foreign uid": {uid: "uid-2", annotations: map[string]string{ackv1alpha1.AnnotationDeclaredFields: valid}},
	} {
		t.Run(name, func(t *testing.T) {
			res := &tResource{obj: &dfCR{ObjectMeta: metav1.ObjectMeta{UID: tc.uid, Annotations: tc.annotations}}}
			_, ok := readDeclaredFieldsRecord(res)
			assert.False(t, ok)
		})
	}
}

func TestDeclaredFieldsForFilter_FallsBackToCurrentSpec(t *testing.T) {
	// A record from another object (for example a copied manifest) is ignored
	// and the fields set right now are used instead.
	foreign, _ := encodeDeclaredFieldsRecord("uid-other", []string{"name", "description"})
	res := &tResource{obj: &dfCR{
		ObjectMeta: metav1.ObjectMeta{UID: "uid-1", Annotations: map[string]string{ackv1alpha1.AnnotationDeclaredFields: foreign}},
		Spec:       dfSpec{Name: strp("a")},
	}}
	assert.Equal(t, map[string]struct{}{"name": {}}, declaredFieldsForFilter(res))
}

// --- filterUnsetUnmanagedDifferences -----------------------------------------

func deltaOf(diffs ...[3]interface{}) *ackcompare.Delta {
	d := ackcompare.NewDelta()
	for _, x := range diffs {
		d.Add(x[0].(string), x[1], x[2])
	}
	return d
}

func pathsOf(d *ackcompare.Delta) []string {
	out := []string{}
	for _, diff := range d.Differences {
		out = append(out, diff.Path.String())
	}
	return out
}

func TestFilterUnsetUnmanaged(t *testing.T) {
	var nilStr *string
	var nilLogging *dfLogging
	var nilList []*string
	var nilMap map[string]*string
	record := map[string]struct{}{
		"name":                       {},
		"kmsKeyID":                   {}, // declared before, removed now
		"loggingConfiguration":       {},
		"loggingConfiguration.level": {},
	}
	d := deltaOf(
		// set on the desired side: always kept
		[3]interface{}{"Spec.Name", strp("a"), strp("b")},
		// explicit empty list is a value the user set: kept
		[3]interface{}{"Spec.SubnetIDs", []*string{}, []*string{strp("s-1")}},
		// never set, AWS reports a default: dropped
		[3]interface{}{"Spec.Description", nilStr, strp("server default")},
		[3]interface{}{"Spec.TracingConfiguration", nilLogging, &dfLogging{}},
		[3]interface{}{"Spec.SecurityGroupIDs", nilList, []*string{strp("sg-1")}},
		[3]interface{}{"Spec.Env", nilMap, map[string]*string{"A": strp("1")}},
		// child of a declared struct that the user never set: dropped
		[3]interface{}{"Spec.LoggingConfiguration.IncludeExecutionData", (*bool)(nil), boolp(false)},
		// set before, unset now (removal), matched case-insensitively: kept
		[3]interface{}{"Spec.KMSKeyID", nilStr, strp("key")},
		// tags keep "unset means empty": kept
		[3]interface{}{"Spec.Tags", nilList, []*string{strp("console-added")}},
		// outside Spec: kept
		[3]interface{}{"Status.Something", nilStr, strp("x")},
	)
	dropped := filterUnsetUnmanagedDifferences(d, record, nil)
	assert.ElementsMatch(t, []string{
		"Spec.Description",
		"Spec.TracingConfiguration",
		"Spec.SecurityGroupIDs",
		"Spec.Env",
		"Spec.LoggingConfiguration.IncludeExecutionData",
	}, dropped)
	assert.Equal(t, []string{
		"Spec.Name", "Spec.SubnetIDs", "Spec.KMSKeyID", "Spec.Tags", "Status.Something",
	}, pathsOf(d))
}

func TestFilterUnsetUnmanaged_RemovedParentKeepsDiff(t *testing.T) {
	// The user removed the whole struct: the parent path is in the record, so
	// the difference is kept and removal behaves as it does without the gate.
	d := deltaOf([3]interface{}{"Spec.LoggingConfiguration", (*dfLogging)(nil), &dfLogging{Level: strp("ERROR")}})
	dropped := filterUnsetUnmanagedDifferences(d, map[string]struct{}{"loggingConfiguration": {}, "loggingConfiguration.level": {}}, nil)
	assert.Empty(t, dropped)
	assert.Len(t, d.Differences, 1)
}

// --- reconciler wiring -------------------------------------------------------

func unsetFieldsReconciler(t *testing.T, gateEnabled bool) (*resourceReconciler, *ackmocks.AWSResourceManager, *ackmocks.AWSResourceDescriptor, *ctrlrtclientmock.Client) {
	t.Helper()
	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, false)
	rec.cfg.FeatureGates[featuregate.UnsetFieldsUnmanaged] = featuregate.Feature{Enabled: gateEnabled}
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)
	return rec, rm, rd, kc
}

// newUnsetRes builds a managed uResource with a UID and optional record.
func newUnsetRes(spec map[string]interface{}, record string) *uResource {
	r := newDriftRes(false, spec)
	r.obj.SetUID("uid-1")
	if record != "" {
		r.obj.SetAnnotations(map[string]string{ackv1alpha1.AnnotationDeclaredFields: record})
	}
	return r
}

// patchedAnnotation returns the declared-fields annotation of the object
// passed to the last kc.Patch call, or "" when Patch was not called.
func patchedAnnotation(kc *ctrlrtclientmock.Client) string {
	for i := len(kc.Calls) - 1; i >= 0; i-- {
		if kc.Calls[i].Method == "Patch" {
			if obj, ok := kc.Calls[i].Arguments.Get(1).(client.Object); ok {
				return obj.GetAnnotations()[ackv1alpha1.AnnotationDeclaredFields]
			}
		}
	}
	return ""
}

func TestUnsetFieldsUnmanaged_ServerDefaultDoesNotDriveUpdate(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	desired := newUnsetRes(map[string]interface{}{"name": "a"}, "")
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}, "")
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	want, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name"})
	assert.Equal(t, want, patchedAnnotation(kc), "the record is written on the first sync")
}

func TestUnsetFieldsUnmanaged_GateOffKeepsTodaysBehavior(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, false)
	desired := newUnsetRes(map[string]interface{}{"name": "a"}, "")
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}, "")
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	assert.Empty(t, patchedAnnotation(kc), "no record without the gate")
}

func TestUnsetFieldsUnmanaged_RemovedFieldStillDrivesUpdate(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	record, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name", "description"})
	desired := newUnsetRes(map[string]interface{}{"name": "a"}, record)
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "old value"}, record)
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	// AWS still reported the removed value before the Update, so the removal
	// stays pending in the record and keeps showing in the delta next time,
	// exactly as without the gate. The stored record already matches, so no
	// record patch is needed.
	want, _ := encodeDeclaredFieldsRecord("uid-1", []string{"description", "name"})
	assert.Equal(t, want, record)
	assert.NotEqual(t, `{"uid":"uid-1","fields":{"name":{}}}`, patchedAnnotation(kc))
}

func TestUnsetFieldsUnmanaged_AppliedRemovalLeavesRecord(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	record, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name", "description"})
	desired := newUnsetRes(map[string]interface{}{"name": "a"}, record)
	// AWS no longer reports description: the removal has been applied.
	latest := newUnsetRes(map[string]interface{}{"name": "a"}, record)
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	want, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name"})
	assert.Equal(t, want, patchedAnnotation(kc))
}

func TestNextDeclaredPaths(t *testing.T) {
	record, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name", "kmsKeyID", "description"})
	declared := &tResource{obj: &dfCR{
		ObjectMeta: metav1.ObjectMeta{UID: "uid-1", Annotations: map[string]string{ackv1alpha1.AnnotationDeclaredFields: record}},
		Spec:       dfSpec{Name: strp("a"), Enabled: boolp(true)},
	}}
	observed := &tResource{obj: &dfCR{Spec: dfSpec{
		Name:     strp("a"),
		Enabled:  boolp(true),
		KMSKeyID: strp("still in AWS"), // removed by the user, still reported: pending
		Logging:  &dfLogging{},         // never declared: not added
		// description: removed and no longer reported: dropped
	}}}
	assert.Equal(t, []string{"enabled", "kmsKeyID", "name"}, nextDeclaredPaths(declared, observed))
	// Without an observation (create path) the record is what is set now.
	assert.Equal(t, []string{"enabled", "name"}, nextDeclaredPaths(declared, nil))
}

func TestUnsetFieldsUnmanaged_ForeignRecordIsNotTrusted(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	// Copied from another object: it claims description was declared.
	foreign, _ := encodeDeclaredFieldsRecord("uid-other", []string{"name", "description"})
	desired := newUnsetRes(map[string]interface{}{"name": "a"}, foreign)
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}, foreign)
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	want, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name"})
	assert.Equal(t, want, patchedAnnotation(kc), "the foreign record is replaced")
}

func TestUnsetFieldsUnmanaged_SteadyStateMakesNoWrite(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	record, _ := encodeDeclaredFieldsRecord("uid-1", []string{"name"})
	desired := newUnsetRes(map[string]interface{}{"name": "a"}, record)
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}, record)
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	kc.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything)
}

// optOutDescriptor is a mocked descriptor that also implements
// AWSResourceDescriptorWithUnsetMeansEmpty, as a generated one does.
type optOutDescriptor struct {
	*ackmocks.AWSResourceDescriptor
	paths []string
}

func (d *optOutDescriptor) UnsetMeansEmptyPaths() []string { return d.paths }

func TestFilterUnsetUnmanaged_DescriptorOptOut(t *testing.T) {
	var nilStr *string
	var nilList []*string
	d := deltaOf(
		[3]interface{}{"Spec.Policy", nilStr, strp("added outside ACK")},
		[3]interface{}{"Spec.Tags", nilList, []*string{strp("t")}},
		[3]interface{}{"Spec.Description", nilStr, strp("server default")},
	)
	// The descriptor's list is authoritative: Policy is opted out, and Tags
	// is kept because the generated list includes the tag field.
	dropped := filterUnsetUnmanagedDifferences(d, map[string]struct{}{}, []string{"Spec.Policy", "Spec.Tags"})
	assert.Equal(t, []string{"Spec.Description"}, dropped)
	assert.Equal(t, []string{"Spec.Policy", "Spec.Tags"}, pathsOf(d))
}

func TestUnsetFieldsUnmanaged_OptedOutFieldStillDrivesUpdate(t *testing.T) {
	rec, rm, rdMock, kc := unsetFieldsReconciler(t, true)
	rec.rd = &optOutDescriptor{AWSResourceDescriptor: rdMock, paths: []string{"Spec.Description"}}
	desired := newUnsetRes(map[string]interface{}{"name": "a"}, "")
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "added outside ACK"}, "")
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
