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
	"errors"
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
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// repeatedUpdatesDescriptor is a mocked descriptor that also implements
// AWSResourceDescriptorWithSkipRepeatedUpdates, as one generated with
// `skip_repeated_updates: true` does.
type repeatedUpdatesDescriptor struct {
	*ackmocks.AWSResourceDescriptor
	enabled bool
}

func (d *repeatedUpdatesDescriptor) SkipRepeatedUpdates() bool { return d.enabled }

// repeatedUpdatesReconciler returns a reconciler whose descriptor was
// generated with (enabled) or without the setting, and whose resource manager
// reports the resource synced or not. Without the setting the descriptor does
// not implement the interface at all, like one from today's code-generator.
func repeatedUpdatesReconciler(t *testing.T, enabled, synced bool) (*resourceReconciler, *ackmocks.AWSResourceManager, *ctrlrtclientmock.Client) {
	t.Helper()
	rec, rm, rd, kc, _ := ignoreDriftReconcilerMocks(t, false)
	rec.updates = newUpdateRecordStore()
	wireDescriptorCommon(rd)
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
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(synced, nil)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	if enabled {
		rec.rd = &repeatedUpdatesDescriptor{AWSResourceDescriptor: rd, enabled: true}
	}
	return rec, rm, kc
}

// rKey is the store key of the resources built by newRURes.
var rKey = k8stypes.NamespacedName{Namespace: "default", Name: "mybook"}

// newRURes builds a managed uResource named default/mybook with UID uid-1.
func newRURes(spec map[string]interface{}) *uResource {
	r := newDriftRes(false, spec)
	r.obj.SetUID("uid-1")
	return r
}

// step is one sync: the CR as stored, what AWS reports, and the Update error.
type step struct {
	desired   acktypes.AWSResource
	latest    acktypes.AWSResource
	updateErr error
}

// runSteps runs one Sync per step and returns how many Updates were sent.
func runSteps(t *testing.T, rec *resourceReconciler, rm *ackmocks.AWSResourceManager, steps ...step) int {
	t.Helper()
	updates := 0
	for i, s := range steps {
		// Fresh objects per sync, as a real ReadOne returns: conditions set
		// on one sync's result must not leak into the next.
		latest := s.latest.DeepCopy()
		rm.On("ReadOne", mock.Anything, mock.Anything).Return(latest, nil).Once()
		rm.On("LateInitialize", mock.Anything, mock.Anything).Return(latest, nil).Maybe().Once()
		rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(mock.Arguments) { updates++ }).
			Return(latest, s.updateErr).Maybe().Once()
		_, err := rec.Sync(context.TODO(), rm, s.desired.DeepCopy())
		if s.updateErr != nil {
			require.Error(t, err, "step %d", i)
		} else {
			require.NoError(t, err, "step %d", i)
		}
		// Drop expectations this step didn't use, so the next step's apply.
		rm.ExpectedCalls = keepCommon(rm.ExpectedCalls)
	}
	return updates
}

// keepCommon drops the per-step expectations (ReadOne, Update, LateInitialize).
func keepCommon(calls []*mock.Call) []*mock.Call {
	out := calls[:0]
	for _, c := range calls {
		switch c.Method {
		case "ReadOne", "Update", "LateInitialize":
			continue
		}
		out = append(out, c)
	}
	return out
}

func repeat(n int, s step) []step {
	out := make([]step, n)
	for i := range out {
		out[i] = s
	}
	return out
}

var (
	ruDesired  = newRURes(map[string]interface{}{"name": "a"})
	ruDefault  = newRURes(map[string]interface{}{"name": "a", "description": "server default"})
	ruChanged  = newRURes(map[string]interface{}{"name": "a", "description": "changed outside ACK"})
	ruDesired2 = newRURes(map[string]interface{}{"name": "b"})
)

// A perpetual delta (here a server default on an unset field) is sent once,
// then skipped while nothing changes.
func TestSkipRepeatedUpdates_PerpetualDeltaSentOnce(t *testing.T) {
	rec, rm, _ := repeatedUpdatesReconciler(t, true, true)
	n := runSteps(t, rec, rm, repeat(4, step{desired: ruDesired, latest: ruDefault})...)
	assert.Equal(t, 1, n)
}

func TestSkipRepeatedUpdates_AWSChangeSendsUpdate(t *testing.T) {
	rec, rm, _ := repeatedUpdatesReconciler(t, true, true)
	n := runSteps(t, rec, rm,
		step{desired: ruDesired, latest: ruDefault},
		step{desired: ruDesired, latest: ruDefault},
		step{desired: ruDesired, latest: ruChanged},
		step{desired: ruDesired, latest: ruChanged},
		step{desired: ruDesired, latest: ruChanged},
	)
	assert.Equal(t, 2, n, "the first sync, then one for the change")
}

func TestSkipRepeatedUpdates_SpecChangeSendsUpdate(t *testing.T) {
	rec, rm, _ := repeatedUpdatesReconciler(t, true, true)
	n := runSteps(t, rec, rm,
		step{desired: ruDesired, latest: ruDefault},
		step{desired: ruDesired, latest: ruDefault},
		step{desired: ruDesired2, latest: ruDefault},
		step{desired: ruDesired2, latest: ruDefault},
	)
	assert.Equal(t, 2, n, "the first sync, then one for the edit")
}

// An Update that fails leaves the sync not Synced, so the next sync sends it
// again: a user's change keeps retrying until it succeeds, as today.
func TestSkipRepeatedUpdates_FailedUpdateIsRetried(t *testing.T) {
	rec, rm, _ := repeatedUpdatesReconciler(t, true, true)
	fail := step{desired: ruDesired, latest: ruDefault, updateErr: errors.New("throttled")}
	n := runSteps(t, rec, rm, fail, fail, fail, step{desired: ruDesired, latest: ruDefault}, step{desired: ruDesired, latest: ruDefault})
	assert.Equal(t, 4, n, "every failed attempt, then the one that succeeds")
	_, ok := rec.updates.committed[rKey]
	assert.True(t, ok)
}

// A resource the manager does not report synced from what AWS reports now
// (for example an update still in progress) is never skipped.
func TestSkipRepeatedUpdates_NotSyncedSendsUpdate(t *testing.T) {
	rec, rm, _ := repeatedUpdatesReconciler(t, true, false)
	spec, _ := hashSpec(ruDesired)
	aws, _ := hashSpec(ruDefault)
	rec.updates.committed[rKey] = updateRecord{uid: "uid-1", spec: spec, aws: aws}
	n := runSteps(t, rec, rm, repeat(3, step{desired: ruDesired, latest: ruDefault})...)
	assert.Equal(t, 3, n)
	_, ok := rec.updates.committed[rKey]
	assert.False(t, ok, "a sync that does not end Synced drops the record")
}

func TestSkipRepeatedUpdates_NotEnabled(t *testing.T) {
	rec, rm, _ := repeatedUpdatesReconciler(t, false, true)
	n := runSteps(t, rec, rm, repeat(3, step{desired: ruDesired, latest: ruDefault})...)
	assert.Equal(t, 3, n)
	assert.Empty(t, rec.updates.committed, "nothing recorded when not enabled")
}

// A record left by an earlier object with the same name is not used.
func TestSkipRepeatedUpdates_RecreatedObjectIsNotTrusted(t *testing.T) {
	rec, rm, _ := repeatedUpdatesReconciler(t, true, true)
	spec, _ := hashSpec(ruDesired)
	aws, _ := hashSpec(ruDefault)
	rec.updates.committed[rKey] = updateRecord{uid: "uid-old", spec: spec, aws: aws}
	n := runSteps(t, rec, rm, repeat(2, step{desired: ruDesired, latest: ruDefault})...)
	assert.Equal(t, 1, n)
	assert.Equal(t, k8stypes.UID("uid-1"), rec.updates.committed[rKey].uid)
}

// deletingRes is a uResource that is being deleted.
type deletingRes struct{ *uResource }

func (r *deletingRes) IsBeingDeleted() bool { return true }

func TestSkipRepeatedUpdates_DeletionForgetsRecord(t *testing.T) {
	rec, rm, _, _, _ := ignoreDriftReconcilerMocks(t, false)
	rec.updates = newUpdateRecordStore()
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("IsManaged", mock.Anything).Return(false)
	rec.rd = rd
	rec.updates.committed[rKey] = updateRecord{uid: "uid-1"}

	_, err := rec.reconcile(context.TODO(), rm, &deletingRes{newRURes(map[string]interface{}{"name": "a"})})
	require.NoError(t, err)
	assert.Empty(t, rec.updates.committed)
}

func TestUpdateRecordStore(t *testing.T) {
	s := newUpdateRecordStore()
	key := k8stypes.NamespacedName{Namespace: "ns", Name: "a"}

	_, ok := s.begin(key, updateRecord{uid: "u1", spec: 1, aws: 2})
	assert.False(t, ok, "no record yet")
	s.finish(key, true)
	prev, ok := s.begin(key, updateRecord{uid: "u1", spec: 3, aws: 4})
	require.True(t, ok)
	assert.Equal(t, updateRecord{uid: "u1", spec: 1, aws: 2}, prev, "begin returns the committed record")

	s.finish(key, false)
	_, ok = s.begin(key, updateRecord{uid: "u1"})
	assert.False(t, ok, "a sync that did not end Synced drops the record")
	s.finish(key, true)
	_, ok = s.begin(key, updateRecord{uid: "u2"})
	assert.False(t, ok, "a record from another UID is not used")

	s.forget(key)
	assert.Empty(t, s.committed)
	assert.Empty(t, s.pending)

	s.finish(key, true)
	assert.Empty(t, s.committed, "finish without begin commits nothing")

	var nilStore *updateRecordStore
	nilStore.finish(key, true)
	nilStore.forget(key)
	_, ok = nilStore.begin(key, updateRecord{})
	assert.False(t, ok, "a nil store records nothing")
}

// --- hashSpec on a typed resource -------------------------------------------

type ruSpec struct {
	Name *string            `json:"name,omitempty"`
	Tags map[string]*string `json:"tags,omitempty"`
}

type ruCR struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ruSpec `json:"spec,omitempty"`
}

func (in *ruCR) DeepCopyObject() k8sruntime.Object { c := *in; return &c }

type ruTyped struct{ obj *ruCR }

func (r *ruTyped) Conditions() []*ackv1alpha1.Condition                   { return nil }
func (r *ruTyped) ReplaceConditions([]*ackv1alpha1.Condition)             {}
func (r *ruTyped) Identifiers() acktypes.AWSResourceIdentifiers           { return nil }
func (r *ruTyped) IsBeingDeleted() bool                                   { return false }
func (r *ruTyped) RuntimeObject() client.Object                           { return r.obj }
func (r *ruTyped) MetaObject() metav1.Object                              { return &r.obj.ObjectMeta }
func (r *ruTyped) SetObjectMeta(meta metav1.ObjectMeta)                   { r.obj.ObjectMeta = meta }
func (r *ruTyped) SetIdentifiers(*ackv1alpha1.AWSIdentifiers) error       { return nil }
func (r *ruTyped) SetStatus(acktypes.AWSResource)                         {}
func (r *ruTyped) PopulateResourceFromAnnotation(map[string]string) error { return nil }
func (r *ruTyped) DeepCopy() acktypes.AWSResource {
	return &ruTyped{obj: r.obj.DeepCopyObject().(*ruCR)}
}

func TestHashSpec(t *testing.T) {
	s := func(v string) *string { return &v }
	a := &ruTyped{obj: &ruCR{Spec: ruSpec{Name: s("a"), Tags: map[string]*string{"x": s("1"), "y": s("2")}}}}
	b := &ruTyped{obj: &ruCR{Spec: ruSpec{Name: s("a"), Tags: map[string]*string{"y": s("2"), "x": s("1")}}}}
	c := &ruTyped{obj: &ruCR{Spec: ruSpec{Name: s("b")}}}
	// Metadata is not part of the hash.
	b.obj.Annotations = map[string]string{"k": "v"}

	ha, ok := hashSpec(a)
	require.True(t, ok)
	hb, _ := hashSpec(b)
	hc, _ := hashSpec(c)
	assert.Equal(t, ha, hb, "same spec, same hash")
	assert.NotEqual(t, ha, hc)

	hu1, ok := hashSpec(ruDefault)
	require.True(t, ok)
	hu2, _ := hashSpec(ruChanged)
	assert.NotEqual(t, hu1, hu2, "unstructured specs are hashed too")

	_, ok = hashSpec(nil)
	assert.False(t, ok)
}
