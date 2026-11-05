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
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// A controller generated with `skip_repeated_updates: true` in its
// generator.yaml skips an Update that would repeat the last successful one.
//
// After every sync that ends with ACK.ResourceSynced and Ready both True, the
// reconciler keeps, in memory, a hash of the spec the delta was computed from
// (the resolved desired state, after the ignore-field-drift merge) and a hash
// of the spec AWS reported before any Update. On the next sync, a non-empty
// delta does not drive an Update when:
//
//   - the CR has such a record, so its last sync ended Synced and Ready,
//   - the spec the delta is computed from hashes the same,
//   - the spec AWS reports hashes the same, and
//   - the resource manager reports the resource synced from what AWS reports
//     now (rm.IsSynced reads the status fields the controller's synced
//     condition is generated from).
//
// Then the delta is the one the last sync acted on. That sync's Update, if it
// sent one, succeeded without changing anything the controller reads, so
// sending it again would change nothing either. This is the perpetual delta:
// a server default on an unset field, a value AWS rewrites, a list AWS
// reorders. Any change to the spec or to what AWS reports, and any sync that
// does not end Synced and Ready (a failed Update, a requeue), sends the
// Update as today.
//
// The record is not persisted, so it is empty after a controller restart or a
// leader change: the first sync of each CR then behaves as today.

// updateRecord holds the hashes from one sync.
type updateRecord struct {
	uid  types.UID
	spec uint64
	aws  uint64
}

// updateRecordStore is the reconciler's record, one per kind. pending holds
// the hashes of the sync in progress, committed those of the CR's last sync
// that ended Synced and Ready. Controller-runtime never reconciles the same
// object concurrently, so one pending entry per object is enough.
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

// begin stores the hashes of the sync in progress and returns the CR's
// committed record. ok is false when there is none, or it belongs to an
// earlier object with the same name. A nil store has no records.
func (s *updateRecordStore) begin(key types.NamespacedName, now updateRecord) (updateRecord, bool) {
	if s == nil {
		return updateRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[key] = now
	prev, ok := s.committed[key]
	if !ok || prev.uid != now.uid {
		return updateRecord{}, false
	}
	return prev, true
}

// finish ends a sync: it commits the pending hashes when the sync ended
// Synced and Ready, and otherwise drops the committed record so the next
// sync behaves as today.
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

// forget drops the CR's record. Called when the CR is deleted or gone.
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

// isRepeatedUpdate records this sync's hashes and reports whether an Update
// for `delta` would repeat the last successful one. desired is the resource
// the delta was computed from; latest is what AWS reports, before any Update.
func (r *resourceReconciler) isRepeatedUpdate(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) bool {
	key, uid, ok := resourceKey(desired)
	if !ok {
		return false
	}
	specHash, ok1 := hashSpec(desired)
	awsHash, ok2 := hashSpec(latest)
	if !ok1 || !ok2 {
		r.updates.forget(key)
		return false
	}
	prev, ok := r.updates.begin(key, updateRecord{uid: uid, spec: specHash, aws: awsHash})
	if !ok || prev.spec != specHash || prev.aws != awsHash {
		return false
	}
	synced, err := rm.IsSynced(ctx, latest)
	return err == nil && synced
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

// hashSpec returns a hash of the resource's spec. Only equality matters, so
// the spec itself is never stored. encoding/json is deterministic for structs
// and sorts map keys.
func hashSpec(res acktypes.AWSResource) (uint64, bool) {
	if ackcompare.IsNil(res) {
		return 0, false
	}
	var spec interface{}
	obj := res.RuntimeObject()
	if u, ok := obj.(*unstructured.Unstructured); ok {
		spec = u.Object["spec"]
	} else {
		v := reflect.ValueOf(obj)
		for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return 0, false
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return 0, false
		}
		f := v.FieldByName("Spec")
		if !f.IsValid() {
			return 0, false
		}
		spec = f.Interface()
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return 0, false
	}
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64(), true
}
