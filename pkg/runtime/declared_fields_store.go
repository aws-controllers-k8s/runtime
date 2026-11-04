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
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// declaredFieldsStore is the controller's record of which spec paths each CR
// declared at its last successful sync. It is internal to the controller
// process: nothing is written to the CR or to the API server, so users never
// see it and keeping it up to date costs no API calls.
//
// There is one store per resourceReconciler, so per kind. Paths come from the
// kind's schema, so the store interns them once and each CR keeps only a
// bitset over the interned paths: an entry costs a few machine words no
// matter how large the field values are.
//
// Each entry also keeps a hash of the value AWS reported for every unset
// field on the previous sync, so a change to that value can be detected.
//
// The store is empty after a controller restart or a leader change. A CR with
// no entry keeps today's behavior for one sync, and its entry is seeded from
// its spec on that sync. See declared_fields.go.
type declaredFieldsStore struct {
	mu      sync.Mutex
	index   map[string]int
	paths   []string
	entries map[types.NamespacedName]declaredEntry
}

type declaredEntry struct {
	uid  types.UID
	bits []uint64
	// seen maps each unset field that AWS reported a value for on the
	// previous filtered sync (by interned delta path) to a hash of that
	// value. It is nil until the CR's first filtered sync.
	seen map[int]uint64
}

func newDeclaredFieldsStore() *declaredFieldsStore {
	return &declaredFieldsStore{
		index:   map[string]int{},
		entries: map[types.NamespacedName]declaredEntry{},
	}
}

// get returns the paths recorded for the CR. ok is false when there is no
// entry, or the entry belongs to an earlier object with the same name (a
// different UID), which happens when a CR is deleted and created again.
// A nil store has no entries.
func (s *declaredFieldsStore) get(key types.NamespacedName, uid types.UID) (map[string]struct{}, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || e.uid != uid {
		return nil, false
	}
	out := map[string]struct{}{}
	for w, word := range e.bits {
		for b := 0; b < 64; b++ {
			if word&(1<<uint(b)) != 0 {
				out[s.paths[w*64+b]] = struct{}{}
			}
		}
	}
	return out, true
}

// set records the paths for the CR, replacing any earlier entry. A nil store
// ignores the call.
func (s *declaredFieldsStore) set(key types.NamespacedName, uid types.UID, paths []string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := make([]int, 0, len(paths))
	for _, p := range paths {
		idx = append(idx, s.intern(p))
	}
	bits := make([]uint64, (len(s.paths)+63)/64)
	for _, i := range idx {
		bits[i/64] |= 1 << uint(i%64)
	}
	// Keep the values seen for the same object; a new object starts over.
	var seen map[int]uint64
	if e, ok := s.entries[key]; ok && e.uid == uid {
		seen = e.seen
	}
	s.entries[key] = declaredEntry{uid: uid, bits: bits, seen: seen}
}

// intern returns the index of p, adding it if needed. Callers hold s.mu.
func (s *declaredFieldsStore) intern(p string) int {
	i, ok := s.index[p]
	if !ok {
		i = len(s.paths)
		s.index[p] = i
		s.paths = append(s.paths, p)
	}
	return i
}

// seenValues returns the value hashes recorded for the CR's unset fields on
// its previous filtered sync, keyed by delta path. It returns nil when the CR
// has no entry or has not had a filtered sync yet.
func (s *declaredFieldsStore) seenValues(key types.NamespacedName, uid types.UID) map[string]uint64 {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || e.uid != uid || e.seen == nil {
		return nil
	}
	out := make(map[string]uint64, len(e.seen))
	for i, h := range e.seen {
		out[s.paths[i]] = h
	}
	return out
}

// setSeenValues replaces the value hashes for the CR's unset fields. It does
// nothing when the CR has no entry.
func (s *declaredFieldsStore) setSeenValues(key types.NamespacedName, uid types.UID, seen map[string]uint64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || e.uid != uid {
		return
	}
	e.seen = make(map[int]uint64, len(seen))
	for p, h := range seen {
		e.seen[s.intern(p)] = h
	}
	s.entries[key] = e
}

// forget drops the CR's entry. Called when the CR is being deleted or is gone.
func (s *declaredFieldsStore) forget(key types.NamespacedName) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

// recorded returns the sorted paths stored for the CR, for tests and logs.
func (s *declaredFieldsStore) recorded(key types.NamespacedName, uid types.UID) []string {
	set, ok := s.get(key, uid)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
