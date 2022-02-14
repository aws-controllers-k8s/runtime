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
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	informersv1 "k8s.io/client-go/informers/core/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	k8scache "k8s.io/client-go/tools/cache"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// namespaceInfo contains annotations ACK controllers care about
type namespaceInfo struct {
	// services.k8s.aws/default-region Annotation
	defaultRegion string
	// services.k8s.aws/owner-account-id Annotation
	ownerAccountID string
	// services.k8s.aws/endpoint-url Annotation
	endpointURL string
}

// getDefaultRegion returns the default region value
func (n *namespaceInfo) getDefaultRegion() string {
	if n == nil {
		return ""
	}
	return n.defaultRegion
}

// getOwnerAccountID returns the namespace owner Account ID
func (n *namespaceInfo) getOwnerAccountID() string {
	if n == nil {
		return ""
	}
	return n.ownerAccountID
}

// getEndpointURL returns the namespace Endpoint URL
func (n *namespaceInfo) getEndpointURL() string {
	if n == nil {
		return ""
	}
	return n.endpointURL
}

// NamespaceCache is responsible of keeping track of namespaces
// annotations, and caching those related to the ACK controller.
type NamespaceCache struct {
	sync.RWMutex
	log logr.Logger
	// namespaceInfos maps namespaces names to their known namespaceInfo
	namespaceInfos map[string]*namespaceInfo
}

// NewNamespaceCache instanciate a new NamespaceCache.
func NewNamespaceCache(log logr.Logger) *NamespaceCache {
	return &NamespaceCache{
		log:            log.WithName("cache.namespace"),
		namespaceInfos: make(map[string]*namespaceInfo),
	}
}

// isIgnoredNamespace returns true if an object is of type corev1.Namespace and
// its metadata name is the ACK system namespace, 'kube-system' or
// 'kube-public'
func isIgnoredNamespace(raw interface{}) bool {
	object, ok := raw.(*corev1.Namespace)
	return ok &&
		(object.ObjectMeta.Name == ackSystemNamespace ||
			object.ObjectMeta.Name == "kube-system" ||
			object.ObjectMeta.Name == "kube-public")
}

// Run instantiate a new shared informer for namespaces and runs it to begin processing items.
func (c *NamespaceCache) Run(clientSet kubernetes.Interface, stopCh <-chan struct{}) {
	informer := informersv1.NewNamespaceInformer(
		clientSet,
		informerResyncPeriod,
		k8scache.Indexers{},
	)
	informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if !isIgnoredNamespace(obj) {
				ns := obj.(*corev1.Namespace)
				c.setNamespaceInfoFromK8sObject(ns)
				c.log.V(1).Info("created namespace", "name", ns.ObjectMeta.Name)
			}
		},
		UpdateFunc: func(orig, desired interface{}) {
			if !isIgnoredNamespace(desired) {
				ns := desired.(*corev1.Namespace)
				c.setNamespaceInfoFromK8sObject(ns)
				c.log.V(1).Info("updated namespace", "name", ns.ObjectMeta.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if !isIgnoredNamespace(obj) {
				ns := obj.(*corev1.Namespace)
				c.deleteNamespaceInfo(ns.ObjectMeta.Name)
				c.log.V(1).Info("deleted namespace", "name", ns.ObjectMeta.Name)
			}
		},
	})
	go informer.Run(stopCh)
}

// GetDefaultRegion returns the default region if it it exists
func (c *NamespaceCache) GetDefaultRegion(namespace string) (string, bool) {
	info, ok := c.getNamespaceInfo(namespace)
	if ok {
		r := info.getDefaultRegion()
		return r, r != ""
	}
	return "", false
}

// GetOwnerAccountID returns the owner account ID if it exists
func (c *NamespaceCache) GetOwnerAccountID(namespace string) (string, bool) {
	info, ok := c.getNamespaceInfo(namespace)
	if ok {
		a := info.getOwnerAccountID()
		return a, a != ""
	}
	return "", false
}

// GetEndpointURL returns the endpoint URL if it exists
func (c *NamespaceCache) GetEndpointURL(namespace string) (string, bool) {
	info, ok := c.getNamespaceInfo(namespace)
	if ok {
		e := info.getEndpointURL()
		return e, e != ""
	}
	return "", false
}

// getNamespaceInfo reads a namespace cached annotations and
// return a given namespace default aws region, owner account id and endpoint url.
// This function is thread safe.
func (c *NamespaceCache) getNamespaceInfo(ns string) (*namespaceInfo, bool) {
	c.RLock()
	defer c.RUnlock()
	namespaceInfo, ok := c.namespaceInfos[ns]
	return namespaceInfo, ok
}

// setNamespaceInfoFromK8sObject takes a corev1.Namespace object and sets the
// namespace ACK related annotations in the cache map
func (c *NamespaceCache) setNamespaceInfoFromK8sObject(ns *corev1.Namespace) {
	nsa := ns.ObjectMeta.Annotations
	nsInfo := &namespaceInfo{}
	DefaultRegion, ok := nsa[ackv1alpha1.AnnotationDefaultRegion]
	if ok {
		nsInfo.defaultRegion = DefaultRegion
	}
	OwnerAccountID, ok := nsa[ackv1alpha1.AnnotationOwnerAccountID]
	if ok {
		nsInfo.ownerAccountID = OwnerAccountID
	}
	EndpointURL, ok := nsa[ackv1alpha1.AnnotationEndpointURL]
	if ok {
		nsInfo.endpointURL = EndpointURL
	}
	c.Lock()
	defer c.Unlock()
	c.namespaceInfos[ns.ObjectMeta.Name] = nsInfo
}

// deleteNamespace deletes an entry from cache map
func (c *NamespaceCache) deleteNamespaceInfo(ns string) {
	c.Lock()
	defer c.Unlock()
	delete(c.namespaceInfos, ns)
}
