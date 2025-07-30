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

package types

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlrt "sigs.k8s.io/controller-runtime"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
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

type ServiceControllerMetadata struct {
	VersionInfo
	// ServiceAlias is a string with the alias of the service API, e.g. "s3"
	ServiceAlias string
	// ServiceAPIGroup is a string with the full DNS-correct API group that
	// this service controller manages, e.g. "s3.services.k8s.aws"
	ServiceAPIGroup string
}

// ServiceController wraps one or more reconcilers (for individual resources in
// an AWS API) with the upstream common controller-runtime machinery.
type ServiceController interface {
	// GetReconcilers returns a slice of types.AWSResourceReconcilers
	// associated with this service controller
	GetReconcilers() []AWSResourceReconciler
	// GetResourceManagerFactories returns the map of resource manager
	// factories, keyed by the GroupKind of the resource managed by the resource
	// manager produced by that factory
	GetResourceManagerFactories() map[string]AWSResourceManagerFactory

	// WithLogger sets up the service controller with the supplied logger
	WithLogger(logr.Logger) ServiceController
	// WithPrometheusRegistry registers all ACK service controller metrics with
	// the supplied prometheus Registry
	WithPrometheusRegistry(prometheus.Registerer) ServiceController
	// WithResourceManagerFactories sets the controller up to manage resources
	// with a set of supplied factories
	WithResourceManagerFactories(
		[]AWSResourceManagerFactory,
	) ServiceController

	// BindControllerManager takes a `controller-runtime.Manager`, creates all
	// the AWSResourceReconcilers needed for the service and binds all of the
	// reconcilers within the service controller with that manager
	BindControllerManager(
		ctrlrt.Manager,
		ackcfg.Config,
	) error

	// NewAWSConfig returns a new config object. By default the returned config
	// is created using pod IRSA environment variables. The BaseEndpoint is
	// configured if the provided endpointURL is not empty.
	NewAWSConfig(
		context.Context,
		ackv1alpha1.AWSRegion,
		*string,
		ackv1alpha1.AWSResourceName,
		schema.GroupVersionKind,
		string,
	) (aws.Config, string, error)

	// GetMetadata returns the metadata associated with the service controller.
	GetMetadata() ServiceControllerMetadata
}
