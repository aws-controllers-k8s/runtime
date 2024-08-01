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
	"errors"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	informersv1 "k8s.io/client-go/informers/core/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	k8scache "k8s.io/client-go/tools/cache"
)

var (
	// ErrCARMConfigMapNotFound is an error that is returned when the CARM
	// configmap is not found.
	ErrCARMConfigMapNotFound = errors.New("CARM configmap not found")
	// ErrAccountIDNotFound is an error that is returned when the account ID
	// is not found in the CARM configmap.
	ErrAccountIDNotFound = errors.New("account ID not found in CARM configmap")
	// ErrEmptyRoleARN is an error that is returned when the role ARN is empty
	// in the CARM configmap.
	ErrEmptyRoleARN = errors.New("role ARN is empty in CARM configmap")
)

const (
	// ACKRoleAccountMap is the name of the configmap map object storing
	// all the AWS Account IDs associated with their AWS Role ARNs.
	ACKRoleAccountMap = "ack-role-account-map"
)

// AccountCache is responsible for caching the CARM configmap
// data. It is listening to all the events related to the CARM map and
// make the changes accordingly.
type AccountCache struct {
	sync.RWMutex
	log              logr.Logger
	roleARNs         map[string]string
	configMapCreated bool
	hasSynced        func() bool
}

// NewAccountCache instanciate a new AccountCache.
func NewAccountCache(log logr.Logger) *AccountCache {
	return &AccountCache{
		log:              log.WithName("cache.account"),
		roleARNs:         make(map[string]string),
		configMapCreated: false,
	}
}

// resourceMatchACKRoleAccountConfigMap verifies if a resource is
// the CARM configmap. It verifies the name, namespace and object type.
func resourceMatchACKRoleAccountsConfigMap(raw interface{}) bool {
	object, ok := raw.(*corev1.ConfigMap)
	return ok && object.ObjectMeta.Name == ACKRoleAccountMap
}

// Run instantiate a new SharedInformer for ConfigMaps and runs it to begin processing items.
func (c *AccountCache) Run(clientSet kubernetes.Interface, stopCh <-chan struct{}) {
	c.log.V(1).Info("Starting shared informer for accounts cache", "targetConfigMap", ACKRoleAccountMap)
	informer := informersv1.NewConfigMapInformer(
		clientSet,
		ackSystemNamespace,
		informerResyncPeriod,
		k8scache.Indexers{},
	)
	informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if resourceMatchACKRoleAccountsConfigMap(obj) {
				cm := obj.(*corev1.ConfigMap)
				object := cm.DeepCopy()
				// To avoid multiple mutex locks, we are updating the cache
				// and the configmap existence flag in the same function.
				configMapCreated := true
				c.updateAccountRoleData(configMapCreated, object.Data)
				c.log.V(1).Info("created account config map", "name", cm.ObjectMeta.Name)
			}
		},
		UpdateFunc: func(orig, desired interface{}) {
			if resourceMatchACKRoleAccountsConfigMap(desired) {
				cm := desired.(*corev1.ConfigMap)
				object := cm.DeepCopy()
				//TODO(a-hilaly): compare data checksum before updating the cache
				c.updateAccountRoleData(true, object.Data)
				c.log.V(1).Info("updated account config map", "name", cm.ObjectMeta.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if resourceMatchACKRoleAccountsConfigMap(obj) {
				cm := obj.(*corev1.ConfigMap)
				newMap := make(map[string]string)
				// To avoid multiple mutex locks, we are updating the cache
				// and the configmap existence flag in the same function.
				configMapCreated := false
				c.updateAccountRoleData(configMapCreated, newMap)
				c.log.V(1).Info("deleted account config map", "name", cm.ObjectMeta.Name)
			}
		},
	})
	go informer.Run(stopCh)
	c.hasSynced = informer.HasSynced
}

// GetAccountRoleARN queries the AWS accountID associated Role ARN
// from the cached CARM configmap. It will return an error if the
// configmap is not found, the accountID is not found or the role ARN
// is empty.
//
// This function is thread safe.
func (c *AccountCache) GetAccountRoleARN(accountID string) (string, error) {
	c.RLock()
	defer c.RUnlock()

	if !c.configMapCreated {
		return "", ErrCARMConfigMapNotFound
	}
	roleARN, ok := c.roleARNs[accountID]
	if !ok {
		return "", ErrAccountIDNotFound
	}
	if roleARN == "" {
		return "", ErrEmptyRoleARN
	}
	return roleARN, nil
}

// updateAccountRoleData updates the CARM map. This function is thread safe.
func (c *AccountCache) updateAccountRoleData(exist bool, data map[string]string) {
	c.Lock()
	defer c.Unlock()
	c.roleARNs = data
	c.configMapCreated = exist
}
