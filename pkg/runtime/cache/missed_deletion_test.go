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

package cache_test

import (
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
)

// brokenWatch serves the WATCHes of resource from fake watchers, so that an
// object deleted from the tracker is not reported to the informer.
type brokenWatch struct {
	mu       sync.Mutex
	watchers []*watch.FakeWatcher
}

func newBrokenWatch(c *k8sfake.Clientset, resource string) *brokenWatch {
	b := &brokenWatch{}
	c.PrependWatchReactor(resource, func(k8stesting.Action) (bool, watch.Interface, error) {
		b.mu.Lock()
		defer b.mu.Unlock()
		w := watch.NewFake()
		b.watchers = append(b.watchers, w)
		return true, w, nil
	})
	return b
}

// expire ends the current WATCH with 410 Expired, as the API server does when
// the informer comes back after its resource version was compacted. The
// informer lists the objects again, and delivers the deletions it missed as
// DeletedFinalStateUnknown tombstones.
func (b *brokenWatch) expire(t *testing.T) {
	b.mu.Lock()
	n := len(b.watchers)
	b.watchers[n-1].Error(&metav1.Status{
		Status: metav1.StatusFailure, Code: 410, Reason: metav1.StatusReasonExpired,
		Message: "too old resource version",
	})
	b.mu.Unlock()
	require.Eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.watchers) > n
	}, 10*time.Second, 10*time.Millisecond, "the informer lists again and watches")
}

// A namespace deleted while the watch is broken reaches the handler as a
// tombstone. The cache removes it, instead of panicking on the type assertion
// and crashing the controller.
func TestNamespaceCache_MissedDeletion(t *testing.T) {
	ns := func(name string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{ackv1alpha1.AnnotationDefaultRegion: "eu-central-1"},
		}}
	}
	k8sClient := k8sfake.NewSimpleClientset(ns("team-a"), ns("team-b"))
	w := newBrokenWatch(k8sClient, "namespaces")

	namespaceCache := ackrtcache.NewNamespaceCache(logr.Discard(), nil, nil)
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	namespaceCache.Run(k8sClient, stopCh)
	require.Eventually(t, func() bool {
		_, ok := namespaceCache.GetDefaultRegion("team-b")
		return ok
	}, 5*time.Second, 10*time.Millisecond)

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	require.NoError(t, k8sClient.Tracker().Delete(gvr, "", "team-b"))
	w.expire(t)

	require.Eventually(t, func() bool {
		_, ok := namespaceCache.GetDefaultRegion("team-b")
		return !ok
	}, 5*time.Second, 10*time.Millisecond, "team-b is removed from the cache")
	_, ok := namespaceCache.GetDefaultRegion("team-a")
	require.True(t, ok)
}

// The CARM account map deleted while the watch is broken reaches the handler
// as a tombstone. The cache drops the map, instead of serving it until the
// controller restarts.
func TestCARMMap_MissedDeletion(t *testing.T) {
	k8sClient := k8sfake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ackrtcache.ACKRoleAccountMap, Namespace: "ack-system"},
		Data:       map[string]string{"222222222222": "arn:aws:iam::222222222222:role/team"},
	})
	w := newBrokenWatch(k8sClient, "configmaps")

	accounts := ackrtcache.NewCARMMapCache(logr.Discard())
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	accounts.Run(ackrtcache.ACKRoleAccountMap, k8sClient, stopCh)
	require.Eventually(t, func() bool {
		_, err := accounts.GetValue("222222222222")
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	require.NoError(t, k8sClient.Tracker().Delete(gvr, "ack-system", ackrtcache.ACKRoleAccountMap))
	w.expire(t)

	require.Eventually(t, func() bool {
		_, err := accounts.GetValue("222222222222")
		return err != nil
	}, 5*time.Second, 10*time.Millisecond, "the deleted account map is no longer served")
}
