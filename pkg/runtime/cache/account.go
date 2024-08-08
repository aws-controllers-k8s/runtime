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
	// ErrKeyNotFound is an error that is returned when the account ID
	// is not found in the CARM configmap.
	ErrKeyNotFound = errors.New("key not found in CARM configmap")
	// ErrEmptyValue is an error that is returned when the role ARN is empty
	// in the CARM configmap.
	ErrEmptyValue = errors.New("role value is empty in CARM configmap")
)

const (
	// ACKRoleAccountMap is the name of the configmap map object storing
	// all the AWS Account IDs associated with their AWS Role ARNs.
	ACKRoleAccountMap = "ack-role-account-map"

	// ACKCARMMapV2 is the name of the v2 CARM map.
	// It stores the mapping for:
	// - Account ID to the AWS role ARNs.
	ACKCARMMapV2 = "ack-carm-map"
)

// CARMMap is responsible for caching the CARM configmap
// data. It is listening to all the events related to the CARM map and
// make the changes accordingly.
type CARMMap struct {
	sync.RWMutex
	log              logr.Logger
	data             map[string]string
	configMapCreated bool
	hasSynced        func() bool
}

// NewCARMMapCache instanciate a new CARMMap.
func NewCARMMapCache(log logr.Logger) *CARMMap {
	return &CARMMap{
		log:              log.WithName("cache.carm"),
		data:             make(map[string]string),
		configMapCreated: false,
	}
}

// resourceMatchCARMConfigMap verifies if a resource is
// the CARM configmap. It verifies the name, namespace and object type.
func resourceMatchCARMConfigMap(raw interface{}, name string) bool {
	object, ok := raw.(*corev1.ConfigMap)
	return ok && object.ObjectMeta.Name == name
}

// Run instantiate a new SharedInformer for ConfigMaps and runs it to begin processing items.
func (c *CARMMap) Run(name string, clientSet kubernetes.Interface, stopCh <-chan struct{}) {
	c.log.V(1).Info("Starting shared informer for accounts cache", "targetConfigMap", ACKRoleAccountMap)
	informer := informersv1.NewConfigMapInformer(
		clientSet,
		ackSystemNamespace,
		informerResyncPeriod,
		k8scache.Indexers{},
	)
	informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if resourceMatchCARMConfigMap(obj, name) {
				cm := obj.(*corev1.ConfigMap)
				object := cm.DeepCopy()
				// To avoid multiple mutex locks, we are updating the cache
				// and the configmap existence flag in the same function.
				configMapCreated := true
				c.updateData(configMapCreated, object.Data)
				c.log.V(1).Info("created account config map", "name", cm.ObjectMeta.Name)
			}
		},
		UpdateFunc: func(orig, desired interface{}) {
			if resourceMatchCARMConfigMap(desired, name) {
				cm := desired.(*corev1.ConfigMap)
				object := cm.DeepCopy()
				//TODO(a-hilaly): compare data checksum before updating the cache
				c.updateData(true, object.Data)
				c.log.V(1).Info("updated account config map", "name", cm.ObjectMeta.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if resourceMatchCARMConfigMap(obj, name) {
				cm := obj.(*corev1.ConfigMap)
				newMap := make(map[string]string)
				// To avoid multiple mutex locks, we are updating the cache
				// and the configmap existence flag in the same function.
				configMapCreated := false
				c.updateData(configMapCreated, newMap)
				c.log.V(1).Info("deleted account config map", "name", cm.ObjectMeta.Name)
			}
		},
	})
	go informer.Run(stopCh)
	c.hasSynced = informer.HasSynced
}

// GetValue queries the value
// from the cached CARM configmap. It will return an error if the
// configmap is not found, the key is not found or the value
// is empty.
//
// This function is thread safe.
func (c *CARMMap) GetValue(key string) (string, error) {
	c.RLock()
	defer c.RUnlock()

	if !c.configMapCreated {
		return "", ErrCARMConfigMapNotFound
	}
	roleARN, ok := c.data[key]
	if !ok {
		return "", ErrKeyNotFound
	}
	if roleARN == "" {
		return "", ErrEmptyValue
	}
	return roleARN, nil
}

// updateData updates the CARM map. This function is thread safe.
func (c *CARMMap) updateData(exist bool, data map[string]string) {
	c.Lock()
	defer c.Unlock()
	c.data = data
	c.configMapCreated = exist
}
