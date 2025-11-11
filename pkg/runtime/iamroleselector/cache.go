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

package iamroleselector

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
)

// Cache wraps the informer for IAMRoleSelector resources
type Cache struct {
	sync.RWMutex
	namespaces *ackcache.NamespaceCache
	log        logr.Logger
	informer   cache.SharedIndexInformer
	selectors  map[string]*ackv1alpha1.IAMRoleSelector // name -> selector
}

// NewCache creates a new IAMRoleSelector cache
func NewCache(log logr.Logger) *Cache {
	return &Cache{
		log:        log.WithName("cache.iam-role-selector"),
		selectors:  make(map[string]*ackv1alpha1.IAMRoleSelector),
		namespaces: ackcache.NewNamespaceCache(log, nil, nil),
	}
}

// Run starts the cache and blocks until stopCh is closed
func (c *Cache) Run(client dynamic.Interface, namespaceClient kubernetes.Interface, stopCh <-chan struct{}) {
	c.log.V(1).Info("Starting IAMRoleSelector cache")

	// Create dynamic informer factory
	factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 0)

	gvr := schema.GroupVersionResource{
		Group:    "services.k8s.aws",
		Version:  "v1alpha1",
		Resource: "iamroleselectors",
	}

	c.informer = factory.ForResource(gvr).Informer()

	// Add event handlers that update our internal map
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.handleAdd(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			c.handleUpdate(oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			c.handleDelete(obj)
		},
	})

	factory.Start(stopCh)

	c.namespaces.Run(namespaceClient, stopCh)
}

func (c *Cache) handleAdd(obj interface{}) {
	u := obj.(*unstructured.Unstructured)
	selector := &ackv1alpha1.IAMRoleSelector{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, selector); err != nil {
		c.log.Error(err, "failed to convert object", "name", u.GetName())
		return
	}

	// Validate before storing
	if err := validateSelector(selector); err != nil {
		c.log.Error(err, "invalid IAMRoleSelector, not caching", "name", selector.Name)
		return
	}

	c.Lock()
	c.selectors[selector.Name] = selector
	c.Unlock()

	c.log.V(1).Info("cached IAMRoleSelector", "name", selector.Name)
}

func (c *Cache) handleUpdate(_, newObj interface{}) {
	u := newObj.(*unstructured.Unstructured)
	selector := &ackv1alpha1.IAMRoleSelector{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, selector); err != nil {
		c.log.Error(err, "failed to convert object", "name", u.GetName())
		return
	}

	// Validate before storing
	if err := validateSelector(selector); err != nil {
		c.log.Error(err, "invalid IAMRoleSelector, removing from cache", "name", selector.Name)
		// Remove from cache if it becomes invalid
		c.Lock()
		delete(c.selectors, selector.Name)
		c.Unlock()
		return
	}

	c.Lock()
	c.selectors[selector.Name] = selector
	c.Unlock()

	c.log.V(1).Info("updated IAMRoleSelector", "name", selector.Name)
}

func (c *Cache) handleDelete(obj interface{}) {
	u := obj.(*unstructured.Unstructured)
	name := u.GetName()

	c.Lock()
	delete(c.selectors, name)
	c.Unlock()

	c.log.V(1).Info("removed IAMRoleSelector from cache", "name", name)
}

// HasSynced returns true if the cache has synced
func (c *Cache) HasSynced() bool {
	if c.informer == nil {
		return false
	}
	return c.informer.HasSynced()
}

// GetMatchingSelectors returns the list of IAMRoleSelectors that match the given context
func (c *Cache) GetMatchingSelectors(
	namespace string,
	namespaceLabels map[string]string,
	gvk schema.GroupVersionKind,
) ([]*ackv1alpha1.IAMRoleSelector, error) {
	if c.informer == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	ctx := MatchContext{
		Namespace:       namespace,
		NamespaceLabels: namespaceLabels,
		GVK:             gvk,
	}

	c.RLock()
	defer c.RUnlock()

	var matches []*ackv1alpha1.IAMRoleSelector
	for _, selector := range c.selectors {
		if Matches(selector, ctx) {
			// Return a copy to avoid mutations
			matches = append(matches, selector.DeepCopy())
		}
	}

	return matches, nil
}

// GetSelector returns a specific selector by name (useful for testing/debugging)
func (c *Cache) GetSelector(name string) (*ackv1alpha1.IAMRoleSelector, bool) {
	c.RLock()
	defer c.RUnlock()

	selector, ok := c.selectors[name]
	if !ok {
		return nil, false
	}
	return selector.DeepCopy(), true
}

// ListSelectors returns all valid selectors in the cache
func (c *Cache) ListSelectors() []*ackv1alpha1.IAMRoleSelector {
	c.RLock()
	defer c.RUnlock()

	selectors := make([]*ackv1alpha1.IAMRoleSelector, 0, len(c.selectors))
	for _, selector := range c.selectors {
		selectors = append(selectors, selector.DeepCopy())
	}
	return selectors
}

// Matches returns a list of IAMRoleSelectors that match the given resource. This function
// should only be called after the cache has been started and synced.
func (c *Cache) Matches(ctx context.Context, resource runtime.Object) ([]*ackv1alpha1.IAMRoleSelector, error) {
	// Extract metadata from the resource
	metaObj, err := meta.Accessor(resource)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata from resource: %w", err)
	}

	namespaceName := metaObj.GetNamespace()
	namespaceLabels := c.namespaces.GetLabels(namespaceName)
	// Get GVK - should be set on ACK resources
	gvk := resource.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		// maybe panic?
		panic("GVK not set on resource")
	}

	// TODO: get namespace labels from a namespace lister/cache
	// For now, pass empty namespace labels
	return c.GetMatchingSelectors(namespaceName, namespaceLabels, gvk)
}
