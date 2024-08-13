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

package cache

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/jaypipes/envutil"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
)

const (
	// envVarACKSystemNamespace is the string key for the environment variable
	// storing the Kubernetes Namespace we use for ConfigMaps and other ACK
	// system configuration needs.
	envVarACKSystemNamespace = "ACK_SYSTEM_NAMESPACE"

	// envVarDeprecatedK8sNamespace is the string key for the old, deprecated
	// environment variable storing the Kubernetes Namespace we use for
	// ConfigMaps and other ACK system configuration needs.
	envVarDeprecatedK8sNamespace = "K8S_NAMESPACE"

	// defaultACKSystemNamespace is the namespace we look up the CARM account
	// map ConfigMap in if the environment variable ACK_SYSTEM_NAMESPACE is not
	// found.
	defaultACKSystemNamespace = "ack-system"

	// informerDefaultResyncPeriod is the period at which ShouldResync
	// is considered.
	// NOTE(jaypipes): setting this to zero means we are telling the client-go
	// caching system not to set up resyncs with an authoritative state source
	// (i.e. a Kubernetes API server) on a periodic basis.
	informerResyncPeriod = 0 * time.Second
)

// ackSystemNamespace is the namespace in which we look up ACK system
// configuration (ConfigMaps, etc)
var ackSystemNamespace string

func init() {
	ackSystemNamespace = envutil.WithDefault(
		envVarACKSystemNamespace, envutil.WithDefault(
			envVarDeprecatedK8sNamespace,
			defaultACKSystemNamespace,
		),
	)
}

// Config is used to configure the caches.
type Config struct {
	// WatchScope is a list of namespaces to watch for resources
	WatchScope []string
	// Ignored is a list of namespaces to ignore
	Ignored []string
}

// Caches is used to interact with the different caches
type Caches struct {
	// stopCh is a channel use to stop all the
	// owned caches
	stopCh chan struct{}

	// Accounts cache
	Accounts *CARMMap

	// CARMMaps v2 cache
	CARMMaps *CARMMap

	// Namespaces cache
	Namespaces *NamespaceCache
}

// New instantiate a new Caches object.
func New(log logr.Logger, config Config, features featuregate.FeatureGates) Caches {
	var carmMaps, accounts *CARMMap
	if features.IsEnabled(featuregate.CARMv2) {
		carmMaps = NewCARMMapCache(log)
	} else {
		accounts = NewCARMMapCache(log)
	}
	return Caches{
		Accounts:   accounts,
		CARMMaps:   carmMaps,
		Namespaces: NewNamespaceCache(log, config.WatchScope, config.Ignored),
	}
}

// Run runs all the owned caches
func (c Caches) Run(clientSet kubernetes.Interface) {
	stopCh := make(chan struct{})
	if c.Accounts != nil {
		c.Accounts.Run(ACKRoleAccountMap, clientSet, stopCh)
	}
	if c.CARMMaps != nil {
		c.CARMMaps.Run(ACKCARMMapV2, clientSet, stopCh)
	}
	if c.Namespaces != nil {
		c.Namespaces.Run(clientSet, stopCh)
	}
}

// WaitForCachesToSync waits for both of the namespace and configMap
// informers to sync - by checking their hasSynced functions.
func (c Caches) WaitForCachesToSync(ctx context.Context) bool {
	// if the cache is not initialized, sync status should be true
	namespaceSynced, accountSynced, carmSynced := true, true, true
	// otherwise check their hasSynced functions
	if c.Namespaces != nil {
		namespaceSynced = cache.WaitForCacheSync(ctx.Done(), c.Namespaces.hasSynced)
	}
	if c.Accounts != nil {
		accountSynced = cache.WaitForCacheSync(ctx.Done(), c.Accounts.hasSynced)
	}
	if c.CARMMaps != nil {
		carmSynced = cache.WaitForCacheSync(ctx.Done(), c.CARMMaps.hasSynced)
	}
	return namespaceSynced && accountSynced && carmSynced
}

// Stop closes the stop channel and cause all the SharedInformers
// by caches to stop running
func (c Caches) Stop() {
	close(c.stopCh)
}
