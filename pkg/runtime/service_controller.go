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
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	kubernetes "k8s.io/client-go/kubernetes"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// VersionInfo contains information about the version of the runtime and
// service controller in use
type VersionInfo struct {
	// GitCommit is the SHA1 commit for the service controller's code
	GitCommit string
	// GitVersion is the latest Git tag from the service controller's code
	GitVersion string
	// BuildDate is a timestamp of when the code was built
	BuildDate string
}

// serviceController wraps a number of `controller-runtime.Reconciler` that are
// related to a specific AWS service API.
type serviceController struct {
	metaLock sync.RWMutex
	// ServiceAlias is a string with the alias of the service API, e.g. "s3"
	ServiceAlias string
	// ServiceAPIGroup is a string with the full DNS-correct API group that
	// this service controller manages, e.g. "s3.services.k8s.aws"
	ServiceAPIGroup string
	// ServiceEndpointsID is a string with the service API's EndpointsID, e.g. "api.sagemaker"
	ServiceEndpointsID string
	// VersionInfo describes the service controller's built code
	VersionInfo VersionInfo
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

	restMapperClient, err := apiutil.NewDiscoveryRESTMapper(clusterConfig)
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
		c.rmFactories[rmf.ResourceDescriptor().GroupKind().String()] = rmf
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

	cache := ackrtcache.New(c.log)
	if cfg.WatchNamespace == "" {
		clusterConfig := mgr.GetConfig()
		clientSet, err := kubernetes.NewForConfig(clusterConfig)
		if err != nil {
			return err
		}
		cache.Run(clientSet)
	}

	adoptionInstalled, err := c.GetAdoptedResourceInstalled(mgr)
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

	exporterInstalled, err := c.GetFieldExportInstalled(mgr)
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

	for _, rmf := range c.rmFactories {
		rec := NewReconciler(c, rmf, c.log, cfg, c.metrics, cache)
		if err := rec.BindControllerManager(mgr); err != nil {
			return err
		}
		rd := rmf.ResourceDescriptor()
		feRec := NewFieldExportReconcilerForAWSResource(c, exporterLogger, cfg, c.metrics, cache, rd)
		if err := feRec.BindControllerManager(mgr); err != nil {
			return err
		}
		c.reconcilers = append(c.reconcilers, rec)
		c.resourceFieldExportReconcilers = append(c.resourceFieldExportReconcilers, feRec)
	}

	return nil
}

// NewServiceController returns a new serviceController instance
func NewServiceController(
	svcAlias string,
	svcAPIGroup string,
	svcEndpointsID string,
	versionInfo VersionInfo,
) acktypes.ServiceController {
	return &serviceController{
		ServiceAlias:       svcAlias,
		ServiceAPIGroup:    svcAPIGroup,
		ServiceEndpointsID: svcEndpointsID,
		VersionInfo:        versionInfo,
		metrics:            ackmetrics.NewMetrics(svcAlias),
	}
}
