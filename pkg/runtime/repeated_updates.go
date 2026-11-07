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
	"hash/fnv"
	"reflect"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// A controller generated with `skip_repeated_updates: true` in its
// generator.yaml skips an Update that two consecutive syncs have shown to
// change nothing.
//
// The reconciler keeps, in memory, at most one record per CR. A sync that
// ends with ACK.ResourceSynced and Ready both True, sent an Update, and read
// AWS again right after it with the same result as the read before it,
// writes a record of two hashes: one of what the Update is built from (the
// resolved desired spec, after the ignore-field-drift merge, plus the CR's
// labels and annotations), and one of what AWS reported (the spec and the
// status, without conditions). The record is confirmed when the next sync
// does the same with the same two hashes. A sync that skips because of a
// confirmed record keeps it; every other sync drops it, so a confirmation
// always comes from two consecutive syncs. On a later sync, a non-empty delta
// does not drive an Update only when:
//
//   - the CR has a confirmed record,
//   - both hashes are unchanged,
//   - the spec references no Secret, whose value can change unseen,
//   - no hook marked the fresh read not synced, Terminal or Recoverable, and
//   - rm.IsSynced reports the fresh read synced.
//
// Then the Update would be built from the same inputs, against the same AWS
// state, as an Update that succeeded and left AWS reading the same. Sending
// it again would change nothing either. This is the perpetual delta: a server
// default on an unset field, a value AWS rewrites, a list AWS reorders.
//
// Why two syncs. A read taken before an Update can't tell an Update that
// changed nothing from one that changed AWS while another writer put the old
// state back before the next sync: both leave the next read equal to the
// read before the Update. The read right after the Update tells them apart,
// unless the service answers it from before the Update's own change, which
// some read APIs do for a few seconds. The second sync's Update is sent a
// resync after the first, when the first Update's change is visible, so its
// read is stale only if the second Update itself changed something, which
// happens only when another writer restored the earlier state in between. A
// one-off restore is then undone by that second Update; only a writer that
// restores the same state after every Update, on a service whose reads lag,
// can still produce a wrong record.
//
// In every other case, including whenever the reconciler cannot tell, the
// Update is sent as today. The record is not persisted, so it is empty after
// a controller restart or a leader change: the first syncs of each CR then
// behave as today.

// updateRecord holds the hashes from one sync. confirmed is set when a
// second consecutive sync with the same hashes also showed its Update to
// change nothing; only a confirmed record lets a sync skip.
type updateRecord struct {
	uid       types.UID
	spec      uint64
	aws       uint64
	confirmed bool
}

// updateRecordStore is the reconciler's record, one per kind. pending holds
// the record the sync in progress will commit if it ends Synced and Ready,
// committed the CR's current record. Controller-runtime never reconciles the
// same object concurrently, so one pending entry per object is enough.
type updateRecordStore struct {
	mu        sync.Mutex
	committed map[types.NamespacedName]updateRecord
	pending   map[types.NamespacedName]updateRecord
}

func newUpdateRecordStore() *updateRecordStore {
	return &updateRecordStore{
		committed: map[types.NamespacedName]updateRecord{},
		pending:   map[types.NamespacedName]updateRecord{},
	}
}

// lookup returns the CR's committed record. ok is false when there is none,
// or it belongs to an earlier object with the same name. A nil store has no
// records.
func (s *updateRecordStore) lookup(key types.NamespacedName, uid types.UID) (updateRecord, bool) {
	if s == nil {
		return updateRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.committed[key]
	if !ok || prev.uid != uid {
		return updateRecord{}, false
	}
	return prev, true
}

// propose sets the record the sync in progress commits if it ends Synced and
// Ready.
func (s *updateRecordStore) propose(key types.NamespacedName, rec updateRecord) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[key] = rec
}

// finish ends a sync: it commits the proposed record when the sync ended
// Synced and Ready, and otherwise, or when nothing was proposed, drops the
// committed record so the next sync behaves as today.
func (s *updateRecordStore) finish(key types.NamespacedName, synced bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[key]
	delete(s.pending, key)
	if synced && ok {
		s.committed[key] = p
		return
	}
	delete(s.committed, key)
}

// forget drops the CR's record. Called when the CR is deleted or gone, and
// when a sync panics.
func (s *updateRecordStore) forget(key types.NamespacedName) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.committed, key)
	delete(s.pending, key)
}

// skipRepeatedUpdates reports whether the controller was generated with
// `skip_repeated_updates: true`.
func (r *resourceReconciler) skipRepeatedUpdates() bool {
	d, ok := r.rd.(acktypes.AWSResourceDescriptorWithSkipRepeatedUpdates)
	return ok && d.SkipRepeatedUpdates()
}

// syncObservation is what one sync would record: the CR's key and the hashes
// of its inputs. ok is false when the sync can never be recorded.
type syncObservation struct {
	key types.NamespacedName
	rec updateRecord
	ok  bool
}

// observeSync hashes this sync's inputs. desired is the resource the delta is
// computed from; latest is what AWS reports, before any Update. Call it before
// the delta is computed: compare hooks may modify both in place.
func (r *resourceReconciler) observeSync(desired, latest acktypes.AWSResource) syncObservation {
	key, uid, ok := resourceKey(desired)
	if !ok {
		return syncObservation{}
	}
	// A Secret's value is read at Update time and is in neither the spec nor
	// what AWS returns, so a rotation can't be seen. Never record.
	if spec, ok := resourceSpec(desired); !ok || hasSecretReference(reflect.ValueOf(spec)) {
		return syncObservation{}
	}
	desiredHash, ok1 := hashDesired(desired)
	observedHash, ok2 := hashObserved(latest)
	if !ok1 || !ok2 {
		return syncObservation{}
	}
	return syncObservation{key: key, rec: updateRecord{uid: uid, spec: desiredHash, aws: observedHash}, ok: true}
}

// isRepeatedUpdate reports whether an Update for this sync would repeat one
// already shown to change nothing. When it does, the record is kept.
func (r *resourceReconciler) isRepeatedUpdate(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	obs syncObservation,
	latest acktypes.AWSResource,
) bool {
	if !obs.ok {
		return false
	}
	prev, ok := r.updates.lookup(obs.key, obs.rec.uid)
	if !ok || !prev.confirmed || prev.spec != obs.rec.spec || prev.aws != obs.rec.aws {
		return false
	}
	// A hook may have marked the read itself not synced (for example a
	// change still propagating).
	if c := ackcondition.Synced(latest); c != nil && c.Status != corev1.ConditionTrue {
		return false
	}
	if conditionTrue(ackcondition.Terminal(latest)) || conditionTrue(ackcondition.Recoverable(latest)) {
		return false
	}
	if synced, err := rm.IsSynced(ctx, latest); err != nil || !synced {
		return false
	}
	r.updates.propose(obs.key, prev)
	return true
}

// recordIfNoOp reads AWS again after a successful Update and proposes the
// sync's record only when the read equals the one taken before the Update,
// so the Update changed nothing the controller reads. The record is confirmed
// when the CR's current record, from the previous sync, has the same hashes. readInput is the object
// the read before the Update was given, unmodified. updated is what the
// Update returned; a result that can't end the sync Synced is not read
// again, since nothing would be recorded anyway.
func (r *resourceReconciler) recordIfNoOp(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	obs syncObservation,
	readInput acktypes.AWSResource,
	updated acktypes.AWSResource,
) {
	if !obs.ok || ackcompare.IsNil(readInput) || ackcompare.IsNil(updated) {
		return
	}
	if c := ackcondition.Synced(updated); c != nil && c.Status != corev1.ConditionTrue {
		return
	}
	if synced, err := rm.IsSynced(ctx, updated); err != nil || !synced {
		return
	}
	rlog := ackrtlog.FromContext(ctx)
	rlog.Enter("rm.ReadOne (after Update)")
	after, err := rm.ReadOne(ctx, readInput)
	rlog.Exit("rm.ReadOne (after Update)", err)
	if err != nil {
		return
	}
	afterHash, ok := hashObserved(after)
	if !ok || afterHash != obs.rec.aws {
		rlog.Debug("not recording the Update as repeatable: AWS reads differently after it")
		return
	}
	rec := obs.rec
	if prev, ok := r.updates.lookup(obs.key, rec.uid); ok && prev.spec == rec.spec && prev.aws == rec.aws {
		rec.confirmed = true
	}
	r.updates.propose(obs.key, rec)
}

// finishSync ends the sync of desired; latest and err are what Sync returns,
// after ensureConditions has set its conditions.
func (r *resourceReconciler) finishSync(desired, latest acktypes.AWSResource, err error) {
	if !r.skipRepeatedUpdates() {
		return
	}
	key, _, ok := resourceKey(desired)
	if !ok {
		return
	}
	synced := err == nil && ackcompare.IsNotNil(latest) &&
		conditionTrue(ackcondition.Synced(latest)) && conditionTrue(ackcondition.Ready(latest))
	r.updates.finish(key, synced)
}

// abandonSync drops the CR's record when a sync panics. A recovered panic
// leaves Sync's error unset and its conditions set from the read before the
// Update, so finishSync would take it for success.
func (r *resourceReconciler) abandonSync(desired acktypes.AWSResource) {
	if key, _, ok := resourceKey(desired); ok {
		r.updates.forget(key)
	}
}

// forgetUpdateRecord drops the CR's record.
func (r *resourceReconciler) forgetUpdateRecord(key types.NamespacedName) {
	r.updates.forget(key)
}

func conditionTrue(c *ackv1alpha1.Condition) bool {
	return c != nil && c.Status == corev1.ConditionTrue
}

// resourceKey returns the namespaced name and UID of a resource.
func resourceKey(res acktypes.AWSResource) (types.NamespacedName, types.UID, bool) {
	if ackcompare.IsNil(res) || res.MetaObject() == nil {
		return types.NamespacedName{}, "", false
	}
	mo := res.MetaObject()
	return types.NamespacedName{Namespace: mo.GetNamespace(), Name: mo.GetName()}, mo.GetUID(), true
}

// hashDesired hashes what the Update is built from: the spec, plus the labels
// and annotations, which some hooks read and which change without changing
// the spec.
func hashDesired(res acktypes.AWSResource) (uint64, bool) {
	spec, ok := resourceSpec(res)
	if !ok {
		return 0, false
	}
	mo := res.MetaObject()
	return hashJSON(spec, mo.GetLabels(), mo.GetAnnotations())
}

// hashObserved hashes what AWS reported: the spec and the status, without
// conditions. Compare and update hooks also read the status.
func hashObserved(res acktypes.AWSResource) (uint64, bool) {
	spec, ok := resourceSpec(res)
	if !ok {
		return 0, false
	}
	status, ok := fieldOf(res, "Status", "status")
	if !ok {
		return 0, false
	}
	b, err := json.Marshal(status)
	if err != nil {
		return 0, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, false
	}
	delete(m, "conditions")
	return hashJSON(spec, m)
}

// hashJSON returns a hash of the JSON encoding of parts. Only equality
// matters, so nothing hashed is stored. encoding/json is deterministic for
// structs and sorts map keys.
func hashJSON(parts ...interface{}) (uint64, bool) {
	b, err := json.Marshal(parts)
	if err != nil {
		return 0, false
	}
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64(), true
}

func resourceSpec(res acktypes.AWSResource) (interface{}, bool) {
	return fieldOf(res, "Spec", "spec")
}

// fieldOf returns the Spec or Status of a typed resource, or the matching
// map entry of an unstructured one.
func fieldOf(res acktypes.AWSResource, goName, jsonName string) (interface{}, bool) {
	if ackcompare.IsNil(res) || res.MetaObject() == nil {
		return nil, false
	}
	obj := res.RuntimeObject()
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u.Object[jsonName], true
	}
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}
	f := v.FieldByName(goName)
	if !f.IsValid() {
		return nil, false
	}
	return f.Interface(), true
}

var secretKeyReferenceType = reflect.TypeOf(ackv1alpha1.SecretKeyReference{})

// hasSecretReference reports whether v holds a set SecretKeyReference
// anywhere under it.
func hasSecretReference(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return false
		}
		return hasSecretReference(v.Elem())
	case reflect.Struct:
		if v.Type() == secretKeyReferenceType {
			return true
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() && hasSecretReference(v.Field(i)) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if hasSecretReference(v.Index(i)) {
				return true
			}
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			if hasSecretReference(v.MapIndex(k)) {
				return true
			}
		}
	}
	return false
}
