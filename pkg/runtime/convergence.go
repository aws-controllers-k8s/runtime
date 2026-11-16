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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// convergenceSkipThreshold is how many counted Updates in a row, with the
// same delta and the same write calls, it takes before ACK skips the Update.
const convergenceSkipThreshold = 2

// convergenceRetryAfter is how long ACK skips an Update before it sends it
// once more while the delta stays the same.
const convergenceRetryAfter = 24 * time.Hour

// convergenceRecord describes the last counted Update of one CR. It holds no
// values from the spec or AWS.
type convergenceRecord struct {
	// uid is the CR's UID, so a CR re-created with the same name starts
	// without a record.
	uid types.UID
	// deltaHash is a SHA-256 of the delta at the last counted Update.
	deltaHash string
	// apiCalls are the AWS write calls that Update made successfully.
	apiCalls []string
	// count is how many counted Updates in a row matched both deltaHash and
	// apiCalls, capped at convergenceSkipThreshold.
	count int
	// skippingSince is when ACK started skipping this Update, or zero if it
	// hasn't skipped it since the last counted Update.
	skippingSince time.Time
}

// convergenceStore keeps the convergence record of each CR of one kind in the
// controller's memory, keyed by namespace and name. Records are never written
// to the CR, so a controller restart starts every CR without one. The lock
// guards the map across concurrent reconciles of different CRs.
// controller-runtime never reconciles one CR concurrently, so a get followed
// by a set for the same CR doesn't race. The zero value is ready to use.
type convergenceStore struct {
	mu      sync.Mutex
	records map[types.NamespacedName]convergenceRecord
	// now returns the current time; nil means time.Now. Tests replace it.
	now func() time.Time
}

// get returns the record of the CR at key, if it has one and it belongs to
// the CR with this UID.
func (s *convergenceStore) get(
	key types.NamespacedName,
	uid types.UID,
) (convergenceRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[key]
	if !ok || rec.uid != uid {
		return convergenceRecord{}, false
	}
	return rec, true
}

// set stores rec as the record of the CR at key.
func (s *convergenceStore) set(key types.NamespacedName, rec convergenceRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = map[types.NamespacedName]convergenceRecord{}
	}
	s.records[key] = rec
}

// delete removes the record of the CR at key, if any.
func (s *convergenceStore) delete(key types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
}

// clock returns the current time.
func (s *convergenceStore) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// convergenceKey returns the key of res's convergence record.
func convergenceKey(res acktypes.AWSResource) types.NamespacedName {
	meta := res.MetaObject()
	return types.NamespacedName{Namespace: meta.GetNamespace(), Name: meta.GetName()}
}

// deltaHash returns a SHA-256 of the delta's differences, independent of
// their order. It returns "" when a value can't be serialized; the caller then
// neither records nor skips.
func deltaHash(delta *ackcompare.Delta) string {
	if delta == nil || len(delta.Differences) == 0 {
		return ""
	}
	entries := make([]string, 0, len(delta.Differences))
	for _, d := range delta.Differences {
		b, err := json.Marshal(d)
		if err != nil {
			return ""
		}
		entries = append(entries, string(b))
	}
	sort.Strings(entries)
	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// deltaFieldPaths returns the field path of each difference in the delta,
// such as "Spec.Tags", without the values.
func deltaFieldPaths(delta *ackcompare.Delta) []string {
	paths := make([]string, 0, len(delta.Differences))
	for _, d := range delta.Differences {
		paths = append(paths, d.Path.String())
	}
	return paths
}

// nextConvergenceRecord returns the record after a counted Update: a
// successful Update, or one that asked to be requeued, that made write calls.
// prev is the zero record when the CR has none.
func nextConvergenceRecord(
	prev convergenceRecord,
	uid types.UID,
	hash string,
	calls []string,
) convergenceRecord {
	next := convergenceRecord{uid: uid, deltaHash: hash, apiCalls: calls, count: 1}
	if prev.deltaHash == hash && slices.Equal(prev.apiCalls, calls) {
		next.count = min(prev.count+1, convergenceSkipThreshold)
	}
	return next
}

// skipUpdate reports whether ACK skips the Update of the CR at key, whose
// delta hashes to hash and whose record is rec. ACK skips once the same
// Update has been counted convergenceSkipThreshold times in a row, unless the
// read says the resource is changing state. The first skip records when
// skipping started. Once convergenceRetryAfter has passed since then, ACK
// sends the Update again; a counted Update replaces the record, so skipping
// starts over.
func (r *resourceReconciler) skipUpdate(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	key types.NamespacedName,
	rec convergenceRecord,
	hash string,
	latest acktypes.AWSResource,
) bool {
	if rec.deltaHash != hash || rec.count < convergenceSkipThreshold {
		return false
	}
	if isTransitioning(ctx, rm, latest) {
		return false
	}
	now := r.convergence.clock()
	if rec.skippingSince.IsZero() {
		rec.skippingSince = now
		r.convergence.set(key, rec)
	}
	return now.Sub(rec.skippingSince) < convergenceRetryAfter
}

// isRequeueError reports whether err asks for the resource to be requeued.
func isRequeueError(err error) bool {
	var needed *requeue.RequeueNeeded
	var neededAfter *requeue.RequeueNeededAfter
	return errors.As(err, &needed) || errors.As(err, &neededAfter)
}

// isTransitioning reports whether the read says the resource is changing
// state, in which case ACK doesn't skip the Update.
func isTransitioning(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	latest acktypes.AWSResource,
) bool {
	if c := ackcondition.Synced(latest); c != nil && c.Status == corev1.ConditionFalse {
		return true
	}
	synced, err := rm.IsSynced(ctx, latest)
	return err != nil || !synced
}
