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
	"errors"
	"fmt"
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
	Type         *string                                    `json:"type_,omitempty"`
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
	// Go field names, matching delta paths ("Logging" is "loggingConfiguration" in JSON).
	assert.Equal(t, []string{
		"CreatedAfter", // has its own JSON encoding: a leaf
		"Enabled",
		"Env",          // map keys are not recorded
		"InlinedField", // embedded struct fields are inlined
		"Logging",      // structs are descended into...
		"Logging.Level",
		"Name",
		"SubnetIDs",
		"SubnetRefs", // ...lists are not
	}, declaredSpecPaths(res))
}

func TestDeclaredSpecPaths_Unstructured(t *testing.T) {
	res := newUResource(nil, map[string]interface{}{
		"name":                 "a",
		"loggingConfiguration": map[string]interface{}{"level": "ERROR"},
	})
	assert.Equal(t, []string{"loggingConfiguration", "loggingConfiguration.level", "name"}, declaredSpecPaths(res))
}

// --- declaredFieldsStore -----------------------------------------------------

func TestDeclaredFieldsStore(t *testing.T) {
	s := newDeclaredFieldsStore()
	key := k8stypes.NamespacedName{Namespace: "ns", Name: "a"}
	_, ok := s.get(key, "uid-1")
	assert.False(t, ok, "no entry yet")

	s.set(key, "uid-1", []string{"name", "loggingConfiguration", "loggingConfiguration.level"})
	assert.Equal(t, []string{"loggingConfiguration", "loggingConfiguration.level", "name"}, s.recorded(key, "uid-1"))

	_, ok = s.get(key, "uid-2")
	assert.False(t, ok, "an entry left by an earlier object with the same name is not used")

	// Paths are interned once per kind; replacing an entry drops the paths
	// that are no longer set.
	other := k8stypes.NamespacedName{Namespace: "ns", Name: "b"}
	s.set(other, "uid-3", []string{"name", "description"})
	s.set(key, "uid-1", []string{"name"})
	assert.Equal(t, []string{"name"}, s.recorded(key, "uid-1"))
	assert.Equal(t, []string{"description", "name"}, s.recorded(other, "uid-3"))
	assert.Len(t, s.paths, 4)

	s.forget(key)
	_, ok = s.get(key, "uid-1")
	assert.False(t, ok, "forgotten")

	// More than 64 distinct paths spill into a second word.
	many := make([]string, 70)
	for i := range many {
		many[i] = fmt.Sprintf("f%02d", i)
	}
	s.set(key, "uid-1", many)
	assert.Equal(t, many, s.recorded(key, "uid-1"))
	assert.Equal(t, []string{"description", "name"}, s.recorded(other, "uid-3"), "older, shorter bitsets still read correctly")

	var nilStore *declaredFieldsStore
	nilStore.set(key, "uid-1", []string{"name"})
	nilStore.forget(key)
	_, ok = nilStore.get(key, "uid-1")
	assert.False(t, ok, "a nil store records nothing")
}

func TestDeclaredFieldsEntry(t *testing.T) {
	rec := &resourceReconciler{declared: newDeclaredFieldsStore()}
	res := &tResource{obj: &dfCR{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "a", UID: "uid-1"},
		Spec:       dfSpec{Name: strp("a")},
	}}
	_, ok := rec.declaredFieldsEntry(res)
	assert.False(t, ok, "no entry")
	// An entry left by an earlier object with the same name is not used.
	rec.declared.set(k8stypes.NamespacedName{Namespace: "ns", Name: "a"}, "uid-old", []string{"name", "description"})
	_, ok = rec.declaredFieldsEntry(res)
	assert.False(t, ok, "entry from another UID")
	rec.seedDeclaredFields(res)
	got, ok := rec.declaredFieldsEntry(res)
	require.True(t, ok)
	assert.Equal(t, map[string]struct{}{"Name": {}}, got, "seeded from the current spec")
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
	dropped := filterUnsetUnmanagedDifferences(d, record, []string{"Spec.Tags"}, nil).dropped
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
	dropped := filterUnsetUnmanagedDifferences(d, map[string]struct{}{"loggingConfiguration": {}, "loggingConfiguration.level": {}}, nil, nil).dropped
	assert.Empty(t, dropped)
	assert.Len(t, d.Differences, 1)
}

// --- reconciler wiring -------------------------------------------------------

// unsetFieldsDescriptor is a mocked descriptor that also implements
// AWSResourceDescriptorWithUnsetFields, as one generated with
// `unset_fields_unmanaged: true` does.
type unsetFieldsDescriptor struct {
	*ackmocks.AWSResourceDescriptor
	enabled bool
	paths   []string
}

func (d *unsetFieldsDescriptor) UnsetFieldsUnmanaged() bool     { return d.enabled }
func (d *unsetFieldsDescriptor) UnsetMeansEmptyPaths() []string { return d.paths }

// unsetFieldsReconciler returns a reconciler whose descriptor was generated
// with (enabled) or without the setting. Without it, the descriptor does not
// implement the interface at all, like one from today's code-generator.
func unsetFieldsReconciler(t *testing.T, enabled bool) (*resourceReconciler, *ackmocks.AWSResourceManager, *ackmocks.AWSResourceDescriptor, *ctrlrtclientmock.Client) {
	t.Helper()
	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, false)
	rec.declared = newDeclaredFieldsStore()
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)
	if enabled {
		rec.rd = &unsetFieldsDescriptor{AWSResourceDescriptor: rd, enabled: true, paths: []string{"Spec.Tags"}}
	}
	return rec, rm, rd, kc
}

// unsetKey is the store key of the resources built by newUnsetRes.
var unsetKey = k8stypes.NamespacedName{Namespace: "default", Name: "mybook"}

// newUnsetRes builds a managed uResource named default/mybook with UID uid-1.
func newUnsetRes(spec map[string]interface{}) *uResource {
	r := newDriftRes(false, spec)
	r.obj.SetUID("uid-1")
	return r
}

// syncN runs n Syncs with AWS reporting `latest` each time.
func syncN(t *testing.T, n int, rec *resourceReconciler, rm *ackmocks.AWSResourceManager, kc *ctrlrtclientmock.Client, desired, latest acktypes.AWSResource) {
	t.Helper()
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	for i := 0; i < n; i++ {
		_, err := rec.Sync(context.TODO(), rm, desired)
		require.NoError(t, err)
	}
}

// syncOnce runs one Sync with AWS reporting `latest`.
func syncOnce(t *testing.T, rec *resourceReconciler, rm *ackmocks.AWSResourceManager, kc *ctrlrtclientmock.Client, desired, latest acktypes.AWSResource) {
	t.Helper()
	syncN(t, 1, rec, rm, kc, desired, latest)
}

func TestUnsetFieldsUnmanaged_ServerDefaultDoesNotDriveUpdate(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	rec.declared.set(unsetKey, "uid-1", []string{"name"})
	syncOnce(t, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}))

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	kc.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything)
}

// TestUnsetFieldsUnmanaged_NoEntryKeepsTodaysBehaviorOnce: after a restart
// the store is empty, so the first sync can't tell a server default from a
// removal. It sends the Update as today and seeds the entry; the next sync
// filters.
func TestUnsetFieldsUnmanaged_NoEntryKeepsTodaysBehaviorOnce(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	syncN(t, 3, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}))

	rm.AssertNumberOfCalls(t, "Update", 1)
	assert.Equal(t, []string{"name"}, rec.declared.recorded(unsetKey, "uid-1"))
}

// A no-op Update that AWS rejects (some services refuse an update that
// changes nothing) must not keep the CR without an entry forever: the entry
// is seeded before the Update, so the next sync filters.
func TestUnsetFieldsUnmanaged_NoEntryFailedUpdateStillSeeds(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	desired := newUnsetRes(map[string]interface{}{"name": "a"})
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"})
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("nothing to update"))
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.Error(t, err)
	assert.Equal(t, []string{"name"}, rec.declared.recorded(unsetKey, "uid-1"))

	_, err = rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)
	rm.AssertNumberOfCalls(t, "Update", 1)
}

func TestUnsetFieldsUnmanaged_NotEnabledKeepsTodaysBehavior(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, false)
	syncOnce(t, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}))

	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	assert.Nil(t, rec.declared.recorded(unsetKey, "uid-1"), "nothing recorded when not enabled")
}

func TestUnsetFieldsUnmanaged_RemovedFieldStillDrivesUpdate(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	rec.declared.set(unsetKey, "uid-1", []string{"name", "description"})
	syncOnce(t, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "old value"}))

	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	// AWS still reported the removed value before the Update, so the removal
	// stays pending in the record and keeps showing in the delta next time,
	// exactly as without the gate.
	assert.Equal(t, []string{"description", "name"}, rec.declared.recorded(unsetKey, "uid-1"))
}

func TestUnsetFieldsUnmanaged_AppliedRemovalLeavesRecord(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	rec.declared.set(unsetKey, "uid-1", []string{"name", "description"})
	// AWS no longer reports description: the removal has been applied.
	syncOnce(t, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a"}))

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	assert.Equal(t, []string{"name"}, rec.declared.recorded(unsetKey, "uid-1"))
}

// TestUnsetFieldsUnmanaged_RemovalAfterRestartIsSentOnce: a removal made
// while the controller was not running reaches the first sync after the
// restart, which keeps today's behavior and sends the Update. After that the
// entry is seeded from the spec, which no longer has the field.
func TestUnsetFieldsUnmanaged_RemovalAfterRestartIsSentOnce(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	syncN(t, 3, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "old value"}))

	rm.AssertNumberOfCalls(t, "Update", 1)
	assert.Equal(t, []string{"name"}, rec.declared.recorded(unsetKey, "uid-1"))
}

func TestNextDeclaredPaths(t *testing.T) {
	previous := map[string]struct{}{"Name": {}, "KMSKeyID": {}, "Description": {}}
	declared := &tResource{obj: &dfCR{Spec: dfSpec{Name: strp("a"), Enabled: boolp(true)}}}
	observed := &tResource{obj: &dfCR{Spec: dfSpec{
		Name:     strp("a"),
		Enabled:  boolp(true),
		KMSKeyID: strp("still in AWS"), // removed by the user, still reported: pending
		Logging:  &dfLogging{},         // never declared: not added
		// description: removed and no longer reported: dropped
	}}}
	assert.Equal(t, []string{"Enabled", "KMSKeyID", "Name"}, nextDeclaredPaths(declared, observed, previous))
	// Without an observation (create path) or an entry, the record is what is set now.
	assert.Equal(t, []string{"Enabled", "Name"}, nextDeclaredPaths(declared, nil, previous))
	assert.Equal(t, []string{"Enabled", "Name"}, nextDeclaredPaths(declared, observed, nil))
}

func TestUnsetFieldsUnmanaged_RecreatedObjectIsNotTrusted(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	// Left by an earlier object with the same name that declared description.
	rec.declared.set(unsetKey, "uid-other", []string{"name", "description"})
	syncN(t, 2, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}))

	// Treated as no entry: today's behavior once, then filtered.
	rm.AssertNumberOfCalls(t, "Update", 1)
	assert.Equal(t, []string{"name"}, rec.declared.recorded(unsetKey, "uid-1"), "replaced")
}

func TestUnsetFieldsUnmanaged_SteadyStateMakesNoWrite(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	rec.declared.set(unsetKey, "uid-1", []string{"name"})
	syncOnce(t, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}))

	rm.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	kc.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything)
}

// deletingRes is a uResource that is being deleted.
type deletingRes struct{ *uResource }

func (r *deletingRes) IsBeingDeleted() bool { return true }

func TestUnsetFieldsUnmanaged_DeletionForgetsEntry(t *testing.T) {
	rec, rm, _, _, _ := ignoreDriftReconcilerMocks(t, false)
	rec.declared = newDeclaredFieldsStore()
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("IsManaged", mock.Anything).Return(false)
	rec.rd = rd
	rec.declared.set(unsetKey, "uid-1", []string{"name"})

	_, err := rec.reconcile(context.TODO(), rm, &deletingRes{newUnsetRes(map[string]interface{}{"name": "a"})})
	require.NoError(t, err)
	_, ok := rec.declared.get(unsetKey, "uid-1")
	assert.False(t, ok)
}

func TestFilterUnsetUnmanaged_DescriptorOptOut(t *testing.T) {
	var nilStr *string
	var nilList []*string
	d := deltaOf(
		[3]interface{}{"Spec.Policy", nilStr, strp("added outside ACK")},
		[3]interface{}{"Spec.Tags", nilList, []*string{strp("t")}},
		[3]interface{}{"Spec.Description", nilStr, strp("server default")},
	)
	// Policy is opted out, and Tags is kept because the generated list always
	// includes the tag field.
	dropped := filterUnsetUnmanagedDifferences(d, map[string]struct{}{}, []string{"Spec.Policy", "Spec.Tags"}, nil).dropped
	assert.Equal(t, []string{"Spec.Description"}, dropped)
	assert.Equal(t, []string{"Spec.Policy", "Spec.Tags"}, pathsOf(d))
}

func TestUnsetFieldsUnmanaged_OptedOutFieldStillDrivesUpdate(t *testing.T) {
	rec, rm, rdMock, kc := unsetFieldsReconciler(t, true)
	rec.rd = &unsetFieldsDescriptor{AWSResourceDescriptor: rdMock, enabled: true, paths: []string{"Spec.Description"}}
	rec.declared.set(unsetKey, "uid-1", []string{"name"})
	desired := newUnsetRes(map[string]interface{}{"name": "a"})
	latest := newUnsetRes(map[string]interface{}{"name": "a", "description": "added outside ACK"})
	rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil)
	rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.Sync(context.TODO(), rm, desired)
	require.NoError(t, err)

	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// A descriptor that implements the interface but reports false (the setting
// is generated per controller, so this is not expected, but must be safe).
func TestUnsetFieldsUnmanaged_DescriptorReportsFalse(t *testing.T) {
	rec, rm, rdMock, kc := unsetFieldsReconciler(t, false)
	rec.rd = &unsetFieldsDescriptor{AWSResourceDescriptor: rdMock, enabled: false}
	syncOnce(t, rec, rm, kc,
		newUnsetRes(map[string]interface{}{"name": "a"}),
		newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"}))
	rm.AssertCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// A field whose JSON name differs from its Go name by more than case (ACK
// names a "type" field Type with JSON "type_") must still match its delta
// path, or removing it would be treated as never set.
func TestFilterUnsetUnmanaged_GoNameDiffersFromJSON(t *testing.T) {
	before := &tResource{obj: &dfCR{Spec: dfSpec{Name: strp("a"), Type: strp("EXPRESS")}}}
	record := map[string]struct{}{}
	for _, p := range declaredSpecPaths(before) {
		record[p] = struct{}{}
	}
	assert.Contains(t, record, "Type")

	var nilStr *string
	d := deltaOf([3]interface{}{"Spec.Type", nilStr, strp("EXPRESS")})
	assert.Empty(t, filterUnsetUnmanagedDifferences(d, record, nil, nil).dropped, "the removal is kept")
	assert.Len(t, d.Differences, 1)
}

// --- AWS values seen on unset fields ----------------------------------------

func TestFilterUnsetUnmanaged_ComparesAgainstPreviousValue(t *testing.T) {
	var nilStr *string
	diffs := func(desc, kms string) *ackcompare.Delta {
		return deltaOf(
			[3]interface{}{"Spec.Description", nilStr, strp(desc)},
			[3]interface{}{"Spec.KMSKeyID", nilStr, strp(kms)},
		)
	}

	// First filtered sync: every difference on an unset field is dropped
	// and its value recorded.
	d := diffs("server default", "aws/managed")
	first := filterUnsetUnmanagedDifferences(d, map[string]struct{}{}, nil, nil)
	assert.ElementsMatch(t, []string{"Spec.Description", "Spec.KMSKeyID"}, first.dropped)
	assert.Empty(t, first.changed)
	assert.Empty(t, d.Differences)

	// AWS changed one value: that difference is kept, the other dropped.
	d = diffs("changed outside ACK", "aws/managed")
	second := filterUnsetUnmanagedDifferences(d, map[string]struct{}{}, nil, first.seen)
	assert.Equal(t, []string{"Spec.KMSKeyID"}, second.dropped)
	assert.Equal(t, []string{"Spec.Description"}, second.changed)
	assert.Equal(t, []string{"Spec.Description"}, pathsOf(d))

	// AWS still reports the changed value (nil could not undo it): accepted.
	d = diffs("changed outside ACK", "aws/managed")
	third := filterUnsetUnmanagedDifferences(d, map[string]struct{}{}, nil, second.seen)
	assert.Empty(t, third.changed)
	assert.Empty(t, d.Differences)
}

func TestFilterUnsetUnmanaged_ValueAppearingIsAChange(t *testing.T) {
	var nilStr *string
	// First filtered sync: AWS reported no value for Policy, so there was no
	// difference and nothing to record.
	first := filterUnsetUnmanagedDifferences(ackcompare.NewDelta(), map[string]struct{}{}, nil, nil)
	require.NotNil(t, first.seen)

	d := deltaOf([3]interface{}{"Spec.Policy", nilStr, strp("added outside ACK")})
	second := filterUnsetUnmanagedDifferences(d, map[string]struct{}{}, nil, first.seen)
	assert.Equal(t, []string{"Spec.Policy"}, second.changed)
	assert.Len(t, d.Differences, 1, "kept, so the hook can delete it")
}

func TestDeclaredFieldsStore_SeenValues(t *testing.T) {
	s := newDeclaredFieldsStore()
	key := k8stypes.NamespacedName{Namespace: "ns", Name: "a"}
	s.setSeenValues(key, "uid-1", map[string]uint64{"Spec.Description": 1})
	assert.Nil(t, s.seenValues(key, "uid-1"), "no entry: nothing stored")

	s.set(key, "uid-1", []string{"Name"})
	assert.Nil(t, s.seenValues(key, "uid-1"), "nil until the first filtered sync")
	s.setSeenValues(key, "uid-1", map[string]uint64{})
	assert.NotNil(t, s.seenValues(key, "uid-1"), "an empty result still marks the first sync as done")

	s.setSeenValues(key, "uid-1", map[string]uint64{"Spec.Description": 7})
	s.set(key, "uid-1", []string{"Name", "Enabled"})
	assert.Equal(t, map[string]uint64{"Spec.Description": 7}, s.seenValues(key, "uid-1"), "kept when the record is replaced")
	assert.Equal(t, []string{"Enabled", "Name"}, s.recorded(key, "uid-1"), "interned delta paths are not recorded as declared")

	s.set(key, "uid-2", []string{"Name"})
	assert.Nil(t, s.seenValues(key, "uid-2"), "a new object with the same name starts over")
}

// syncSeq runs one Sync per latest, with AWS reporting each in turn.
func syncSeq(t *testing.T, rec *resourceReconciler, rm *ackmocks.AWSResourceManager, kc *ctrlrtclientmock.Client, desired acktypes.AWSResource, latests ...acktypes.AWSResource) {
	t.Helper()
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	for _, latest := range latests {
		rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil).Once()
		rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(latest, nil).Once()
		rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil).Once()
		_, err := rec.Sync(context.TODO(), rm, desired)
		require.NoError(t, err)
	}
}

// An AWS-side change to a field the user never set drives one Update, as
// today. When nil can't undo it, the new value is accepted afterwards.
func TestUnsetFieldsUnmanaged_AWSChangeDrivesOneUpdate(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	rec.declared.set(unsetKey, "uid-1", []string{"name"})
	desired := newUnsetRes(map[string]interface{}{"name": "a"})
	def := newUnsetRes(map[string]interface{}{"name": "a", "description": "server default"})
	changed := newUnsetRes(map[string]interface{}{"name": "a", "description": "changed outside ACK"})

	syncSeq(t, rec, rm, kc, desired, def, def, changed, changed, changed)
	rm.AssertNumberOfCalls(t, "Update", 1)
}

// A value that appears on a field AWS previously reported as unset (a policy
// added outside ACK) is a change too, so the hook gets to remove it.
func TestUnsetFieldsUnmanaged_AppearingValueDrivesOneUpdate(t *testing.T) {
	rec, rm, _, kc := unsetFieldsReconciler(t, true)
	rec.declared.set(unsetKey, "uid-1", []string{"name"})
	desired := newUnsetRes(map[string]interface{}{"name": "a"})
	none := newUnsetRes(map[string]interface{}{"name": "a"})
	added := newUnsetRes(map[string]interface{}{"name": "a", "description": "added outside ACK"})

	// The Update removed it, so AWS reports nothing afterwards.
	syncSeq(t, rec, rm, kc, desired, none, added, none, none)
	rm.AssertNumberOfCalls(t, "Update", 1)
}
