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
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlrt "sigs.k8s.io/controller-runtime"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
)

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

	// NewSession returns a new session object. By default the returned session
	// is created using pod IRSA environment variables. If assumeRoleARN is not
	// empty, NewSession will call STS::AssumeRole and use the returned
	// credentials to create the session.
	NewSession(
		ackv1alpha1.AWSRegion,
		*string,
		ackv1alpha1.AWSResourceName,
		schema.GroupVersionKind,
	) (*session.Session, error)
}
