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
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
	ackutil "github.com/aws-controllers-k8s/runtime/pkg/util"
)

const (
	// NamespaceKubeNodeLease is the name of the Kubernetes namespace that
	// contains the kube-node-lease resources (used for node hearthbeats)
	NamespaceKubeNodeLease = "kube-node-lease"
	// NamespacePublic is the name of the Kubernetes namespace that contains
	// the public info (ConfigMaps)
	NamespaceKubePublic = "kube-public"
	// NamespaceSystem is the name of the Kubernetes namespace where we place
	// system components.
	NamespaceKubeSystem = "kube-system"
)

// serviceController wraps a number of `controller-runtime.Reconciler` that are
// related to a specific AWS service API.
type serviceController struct {
	acktypes.ServiceControllerMetadata
	metaLock sync.RWMutex
	// rmFactories is a map of resource manager factories, keyed by the
	// GroupKind of the resource managed by the resource manager produced by
	// that factory
	rmFactories map[string]acktypes.AWSResourceManagerFactory
	// reconcilers is a map containing AWSResourceReconciler objects that are
	// bound to the `controller-runtime.Manager` in `BindControllerManager`
	reconcilers []acktypes.AWSResourceReconciler
	// adoptionReconciler contains a reconciler that for the adoption process
	// and is bound to the `controller-runtime.Manager` in
	// `BindControllerManager`
	adoptionReconciler acktypes.Reconciler
	// fieldExportReconciler contains a reconciler that for the field export
	// process and is bound to the `controller-runtime.Manager` in
	// `BindControllerManager`
	fieldExportReconciler acktypes.Reconciler
	// resourceFieldExportReconcilers is a slice containing
	// FieldExportReconciler objects that are bound to each resource
	resourceFieldExportReconcilers []acktypes.FieldExportReconciler
	// log refers to the logr.Logger object handling logging for the service
	// controller
	log logr.Logger
	// metrics contains a collection of Prometheus metric objects that the
	// service controller and its reconcilers track
	metrics *ackmetrics.Metrics
}

// GetReconcilers returns a slice of types.AWSResourceReconcilers associated
// with this service controller
func (c *serviceController) GetReconcilers() []acktypes.AWSResourceReconciler {
	c.metaLock.RLock()
	defer c.metaLock.RUnlock()
	return c.reconcilers
}

// GetResourceManagerFactories returns the map of resource manager
// factories, keyed by the GroupKind of the resource managed by the resource
// manager produced by that factory
func (c *serviceController) GetResourceManagerFactories() map[string]acktypes.AWSResourceManagerFactory {
	c.metaLock.RLock()
	defer c.metaLock.RUnlock()
	return c.rmFactories
}

// getResourceInstalled returns whether the given resource plural has been
// installed into the cluster, and is accessible by the service controller.
func (c *serviceController) getResourceInstalled(mgr ctrlrt.Manager, resourcePlural string) (bool, error) {
	clusterConfig := mgr.GetConfig()
	clientSet, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return false, err
	}

	gv := schema.GroupVersion{
		Group:   ackv1alpha1.GroupVersion.Group,
		Version: ackv1alpha1.GroupVersion.Version,
	}

	// Ensure GV is supported
	if err = discovery.ServerSupportsVersion(clientSet, gv); err != nil {
		return false, nil
	}

	httpClient, err := rest.HTTPClientFor(clusterConfig)
	if err != nil {
		return false, err
	}

	restMapperClient, err := apiutil.NewDynamicRESTMapper(clusterConfig, httpClient)
	if err != nil {
		return false, err
	}

	gvr := schema.GroupVersionResource{
		Group:    ackv1alpha1.GroupVersion.Group,
		Version:  ackv1alpha1.GroupVersion.Version,
		Resource: strings.ToLower(resourcePlural),
	}

	// Ensure individual kind is supported
	if _, err := restMapperClient.KindFor(gvr); meta.IsNoMatchError(err) {
		return false, nil
	}

	return true, nil
}

// GetAdoptedResourceInstalled returns whether the AdoptedResource CRD has been
// installed into the cluster, and is accessible by the service controller.
func (c *serviceController) GetAdoptedResourceInstalled(mgr ctrlrt.Manager) (bool, error) {
	return c.getResourceInstalled(mgr, "adoptedresources")
}

// GetFieldExportInstalled returns whether the FieldExport CRD has been
// installed into the cluster, and is accessible by the service controller.
func (c *serviceController) GetFieldExportInstalled(mgr ctrlrt.Manager) (bool, error) {
	return c.getResourceInstalled(mgr, "fieldexports")
}

// WithLogger sets up the service controller with the supplied logger
func (c *serviceController) WithLogger(log logr.Logger) acktypes.ServiceController {
	c.log = log
	return c
}

// WithPrometheusRegistry registers all ACK service controller metrics with the
// supplied prometheus Registry
func (c *serviceController) WithPrometheusRegistry(
	reg prometheus.Registerer,
) acktypes.ServiceController {
	if c.metrics == nil {
		return c
	}
	for _, collector := range c.metrics.Collectors() {
		reg.MustRegister(collector)
	}
	return c
}

// WithResourceManagerFactories sets the controller up to manage resources with
// a set of supplied factories
func (c *serviceController) WithResourceManagerFactories(
	rmfs []acktypes.AWSResourceManagerFactory,
) acktypes.ServiceController {
	c.metaLock.Lock()
	defer c.metaLock.Unlock()

	if c.rmFactories == nil {
		c.rmFactories = make(
			map[string]acktypes.AWSResourceManagerFactory,
			len(rmfs),
		)
	}

	for _, rmf := range rmfs {
		c.rmFactories[rmf.ResourceDescriptor().GroupVersionKind().GroupKind().String()] = rmf
	}
	return c
}

// BindControllerManager takes a `controller-runtime.Manager`, creates all the
// AWSResourceReconcilers needed for the service and binds all of the
// reconcilers within the service controller with that manager. The adoption
// reconciler will only be started if the types have been registered in the
// cluster.
func (c *serviceController) BindControllerManager(mgr ctrlrt.Manager, cfg ackcfg.Config) error {
	c.metaLock.Lock()
	defer c.metaLock.Unlock()

	namespaces, err := cfg.GetWatchNamespaces()
	if err != nil {
		return fmt.Errorf("unable to get watch namespaces: %v", err)
	}

	cache := ackrtcache.New(c.log, ackrtcache.Config{
		WatchScope: namespaces,
		// Default to ignoring the kube-system, kube-public, and
		// kube-node-lease namespaces.
		// NOTE: Maybe we should make this configurable? It's not clear that
		// we'd ever want to watch these namespaces.
		Ignored: []string{
			NamespaceKubeSystem,
			NamespaceKubePublic,
			NamespaceKubeNodeLease,
		}},
		cfg.FeatureGates,
	)
	// The caches are only used for cross account resource management. We
	// want to run them only when --enable-carm is set to true and
	// --watch-namespace is set to zero or more than one namespaces.
	if cfg.EnableCARM {
		if len(namespaces) == 1 {
			c.log.V(0).Info("--enable-carm is set to true but --watch-namespace is set to a single namespace. CARM will not be enabled.")
		} else {
			clusterConfig := mgr.GetConfig()
			clientSet, err := kubernetes.NewForConfig(clusterConfig)
			if err != nil {
				return err
			}
			// Run the caches. This will not block as the caches are run in
			// separate goroutines.
			cache.Run(clientSet)
			// Wait for the caches to sync
			ctx := context.TODO()
			synced := cache.WaitForCachesToSync(ctx)
			c.log.Info("Waited for the caches to sync", "synced", synced)
		}
	}

	// Setup adoption reconciler if enabled
	var adoptionInstalled bool
	if cfg.EnableAdoptedResourceReconciler {
		adoptionInstalled, err = c.GetAdoptedResourceInstalled(mgr)
		adoptionLogger := c.log.WithName("adoption")
		if err != nil {
			adoptionLogger.Error(err, "unable to determine if the AdoptedResource CRD is installed in the cluster")
		} else if !adoptionInstalled {
			adoptionLogger.Info("AdoptedResource CRD not installed. The adoption reconciler will not be started")
		} else {
			rec := NewAdoptionReconciler(c, adoptionLogger, cfg, c.metrics, cache)
			if err := rec.BindControllerManager(mgr); err != nil {
				return err
			}
			c.adoptionReconciler = rec
		}
	}

	// Setup field export reconciler if enabled
	var exporterInstalled bool
	if cfg.EnableFieldExportReconciler {
		exporterInstalled, err = c.GetFieldExportInstalled(mgr)
		exporterLogger := c.log.WithName("exporter")
		if err != nil {
			exporterLogger.Error(err, "unable to determine if the FieldExport CRD is installed in the cluster")
		} else if !exporterInstalled {
			exporterLogger.Info("FieldExport CRD not installed. The field export reconciler will not be started")
		} else {
			rec := NewFieldExportReconcilerForFieldExport(c, exporterLogger, cfg, c.metrics, cache)
			if err := rec.BindControllerManager(mgr); err != nil {
				return err
			}
			c.fieldExportReconciler = rec
		}
	}

	// Get the list of resources to reconcile from the config
	reconcileResources, err := cfg.GetReconcileResources()
	if err != nil {
		return fmt.Errorf("error parsing reconcile resources: %v", err)
	}

	if len(reconcileResources) == 0 {
		c.log.Info("No resources? Did they all go on vacation? Defaulting to reconciling all resources.")
	}
	// Filter the resource manager factories
	filteredRMFs := c.rmFactories
	if len(reconcileResources) > 0 {
		filteredRMFs = make(map[string]acktypes.AWSResourceManagerFactory)
		for key, rmf := range c.rmFactories {
			rd := rmf.ResourceDescriptor()
			resourceKind := rd.GroupVersionKind().Kind

			if ackutil.InStrings(resourceKind, reconcileResources) {
				filteredRMFs[key] = rmf
				c.log.Info("including reconciler for resource kind", "kind", resourceKind)
			} else {
				c.log.Info("excluding reconciler for resource kind", "kind", resourceKind, "reason", "not in reconcile-resources flag")
			}
		}
	}

	for _, rmf := range filteredRMFs {
		rec := NewReconciler(c, rmf, c.log, cfg, c.metrics, cache)
		if err := rec.BindControllerManager(mgr); err != nil {
			return err
		}
		c.reconcilers = append(c.reconcilers, rec)

		if cfg.EnableFieldExportReconciler && exporterInstalled {
			rd := rmf.ResourceDescriptor()
			exporterLogger := c.log.WithName("exporter")
			feRec := NewFieldExportReconcilerForAWSResource(c, exporterLogger, cfg, c.metrics, cache, rd)
			if err := feRec.BindControllerManager(mgr); err != nil {
				return err
			}
			c.resourceFieldExportReconcilers = append(c.resourceFieldExportReconcilers, feRec)
		}
	}

	return nil
}

// GetMetadata returns the metadata associated with the service controller.
func (c *serviceController) GetMetadata() acktypes.ServiceControllerMetadata {
	return c.ServiceControllerMetadata
}

// NewServiceController returns a new serviceController instance
func NewServiceController(
	svcAlias string,
	svcAPIGroup string,
	versionInfo acktypes.VersionInfo,
) acktypes.ServiceController {
	return &serviceController{
		ServiceControllerMetadata: acktypes.ServiceControllerMetadata{
			VersionInfo:     versionInfo,
			ServiceAlias:    svcAlias,
			ServiceAPIGroup: svcAPIGroup,
		},
		metrics: ackmetrics.NewMetrics(svcAlias),
	}
}
