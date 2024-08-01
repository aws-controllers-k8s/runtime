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
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
)

const (
	testNamespace1 = "production"
)

func TestNamespaceCache(t *testing.T) {
	// create a fake k8s client and fake watcher
	k8sClient := k8sfake.NewSimpleClientset()
	watcher := watch.NewFake()
	k8sClient.PrependWatchReactor("production", k8stesting.DefaultWatchReactor(watcher, nil))

	// New logger writing to specific buffer
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))

	// initlizing account cache
	namespaceCache := ackrtcache.NewNamespaceCache(fakeLogger, []string{}, []string{})
	stopCh := make(chan struct{})

	namespaceCache.Run(k8sClient, stopCh)

	// Test create events
	_, err := k8sClient.CoreV1().Namespaces().Create(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
				Annotations: map[string]string{
					ackv1alpha1.AnnotationDefaultRegion:  "us-west-2",
					ackv1alpha1.AnnotationOwnerAccountID: "012345678912",
					ackv1alpha1.AnnotationEndpointURL:    "https://amazon-service.region.amazonaws.com",
				},
			},
		},
		metav1.CreateOptions{},
	)
	require.Nil(t, err)

	time.Sleep(time.Second)

	defaultRegion, ok := namespaceCache.GetDefaultRegion("production")
	require.True(t, ok)
	require.Equal(t, "us-west-2", defaultRegion)

	ownerAccountID, ok := namespaceCache.GetOwnerAccountID("production")
	require.True(t, ok)
	require.Equal(t, "012345678912", ownerAccountID)

	endpointURL, ok := namespaceCache.GetEndpointURL("production")
	require.True(t, ok)
	require.Equal(t, "https://amazon-service.region.amazonaws.com", endpointURL)

	// Test update events
	_, err = k8sClient.CoreV1().Namespaces().Update(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
				Annotations: map[string]string{
					ackv1alpha1.AnnotationDefaultRegion:  "us-est-1",
					ackv1alpha1.AnnotationOwnerAccountID: "21987654321",
					ackv1alpha1.AnnotationEndpointURL:    "https://amazon-other-service.region.amazonaws.com",
				},
			},
		},
		metav1.UpdateOptions{},
	)
	require.Nil(t, err)

	time.Sleep(time.Second)

	defaultRegion, ok = namespaceCache.GetDefaultRegion("production")
	require.True(t, ok)
	require.Equal(t, "us-est-1", defaultRegion)

	ownerAccountID, ok = namespaceCache.GetOwnerAccountID("production")
	require.True(t, ok)
	require.Equal(t, "21987654321", ownerAccountID)

	endpointURL, ok = namespaceCache.GetEndpointURL("production")
	require.True(t, ok)
	require.Equal(t, "https://amazon-other-service.region.amazonaws.com", endpointURL)

	// Test delete events
	err = k8sClient.CoreV1().Namespaces().Delete(
		context.Background(),
		"production",
		metav1.DeleteOptions{},
	)
	require.Nil(t, err)

	time.Sleep(time.Second)

	_, ok = namespaceCache.GetDefaultRegion(testNamespace1)
	require.False(t, ok)
}

func TestNamespaceCacheWithRoleARN(t *testing.T) {
	// create a fake k8s client and fake watcher
	k8sClient := k8sfake.NewSimpleClientset()
	watcher := watch.NewFake()
	k8sClient.PrependWatchReactor("production", k8stesting.DefaultWatchReactor(watcher, nil))

	// New logger writing to specific buffer
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))

	// initlizing account cache
	namespaceCache := ackrtcache.NewNamespaceCache(fakeLogger, []string{}, []string{})
	stopCh := make(chan struct{})

	namespaceCache.Run(k8sClient, stopCh)

	// Test create events
	_, err := k8sClient.CoreV1().Namespaces().Create(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
				Annotations: map[string]string{
					ackv1alpha1.AnnotationDefaultRegion: "us-west-2",
					ackv1alpha1.AnnotationTeamID:        "team-a",
					ackv1alpha1.AnnotationEndpointURL:   "https://amazon-service.region.amazonaws.com",
				},
			},
		},
		metav1.CreateOptions{},
	)
	require.Nil(t, err)

	time.Sleep(time.Second)

	defaultRegion, ok := namespaceCache.GetDefaultRegion("production")
	require.True(t, ok)
	require.Equal(t, "us-west-2", defaultRegion)

	teamID, ok := namespaceCache.GetTeamID("production")
	require.True(t, ok)
	require.Equal(t, "team-a", teamID)

	endpointURL, ok := namespaceCache.GetEndpointURL("production")
	require.True(t, ok)
	require.Equal(t, "https://amazon-service.region.amazonaws.com", endpointURL)

	// Test update events
	_, err = k8sClient.CoreV1().Namespaces().Update(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
				Annotations: map[string]string{
					ackv1alpha1.AnnotationDefaultRegion: "us-est-1",
					ackv1alpha1.AnnotationTeamID:        "team-b",
					ackv1alpha1.AnnotationEndpointURL:   "https://amazon-other-service.region.amazonaws.com",
				},
			},
		},
		metav1.UpdateOptions{},
	)
	require.Nil(t, err)

	time.Sleep(time.Second)

	defaultRegion, ok = namespaceCache.GetDefaultRegion("production")
	require.True(t, ok)
	require.Equal(t, "us-est-1", defaultRegion)

	teamID, ok = namespaceCache.GetTeamID("production")
	require.True(t, ok)
	require.Equal(t, "team-b", teamID)

	endpointURL, ok = namespaceCache.GetEndpointURL("production")
	require.True(t, ok)
	require.Equal(t, "https://amazon-other-service.region.amazonaws.com", endpointURL)

	// Test delete events
	err = k8sClient.CoreV1().Namespaces().Delete(
		context.Background(),
		"production",
		metav1.DeleteOptions{},
	)
	require.Nil(t, err)

	time.Sleep(time.Second)

	_, ok = namespaceCache.GetDefaultRegion(testNamespace1)
	require.False(t, ok)
}

func TestScopedNamespaceCache(t *testing.T) {
	defaultConfig := ackrtcache.Config{
		WatchScope: []string{"watch-scope", "watch-scope-2"},
		Ignored:    []string{"ignored", "ignored-2"},
	}

	testCases := []struct {
		name            string
		createNamespace string
		expectCacheHit  bool
		cacheConfig     ackrtcache.Config
	}{
		{
			name:            "namespace in scope",
			createNamespace: "watch-scope",
			expectCacheHit:  true,
			cacheConfig:     defaultConfig,
		},
		{
			name:            "namespace not in scope",
			createNamespace: "watch-scope-3",
			expectCacheHit:  false,
			cacheConfig:     defaultConfig,
		},
		{
			name:            "namespace in ignored",
			createNamespace: "ignored",
			expectCacheHit:  false,
			cacheConfig:     defaultConfig,
		},
		{
			name:            "namespace is nor in scope or ignored",
			createNamespace: "random-penguin",
			expectCacheHit:  false,
			cacheConfig:     defaultConfig,
		},
		{
			name:            "namespace is in scope and ignored",
			createNamespace: "watch-scope-2",
			expectCacheHit:  true,
			cacheConfig:     defaultConfig,
		},
		{
			name:            "cache watching all namespaces - namespace in scope",
			cacheConfig:     ackrtcache.Config{},
			createNamespace: "watch-scope",
			expectCacheHit:  true,
		},
		{
			name:            "cache watching all namespaces - namespace is ignored",
			cacheConfig:     ackrtcache.Config{Ignored: []string{"kube-system"}},
			createNamespace: "kube-system",
			expectCacheHit:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a fake k8s client and fake watcher
			k8sClient := k8sfake.NewSimpleClientset()
			watcher := watch.NewFake()
			k8sClient.PrependWatchReactor("random-penguin", k8stesting.DefaultWatchReactor(watcher, nil))

			// New logger writing to specific buffer
			zapOptions := ctrlrtzap.Options{
				Development: true,
				Level:       zapcore.InfoLevel,
				DestWriter:  io.Discard,
			}
			fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))

			// initlizing account cache
			namespaceCache := ackrtcache.NewNamespaceCache(fakeLogger, tc.cacheConfig.WatchScope, tc.cacheConfig.Ignored)
			stopCh := make(chan struct{})

			namespaceCache.Run(k8sClient, stopCh)

			// Create namespace with name testNamespace1
			_, err := k8sClient.CoreV1().Namespaces().Create(
				context.Background(),
				newNamespace(tc.createNamespace),
				metav1.CreateOptions{},
			)
			require.Nil(t, err)

			// Need a better way to wait for the cache to be updated
			// Thinking informer.WaitForCacheSync() ~ but it's not exported
			time.Sleep(time.Millisecond * 50)

			_, found := namespaceCache.GetDefaultRegion(tc.createNamespace)
			require.Equal(t, tc.expectCacheHit, found)
		})
	}
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				ackv1alpha1.AnnotationDefaultRegion: "us-west-2",
			},
		},
	}
}
