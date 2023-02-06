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

package runtime_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	k8sscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	mocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackrt "github.com/aws-controllers-k8s/runtime/pkg/runtime"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	groupVersion := schema.GroupVersion{Group: "bookstore.services.k8s.aws", Version: "v1alpha1"}
	schemeBuilder := &k8sscheme.Builder{GroupVersion: groupVersion}
	schemeBuilder.Register(&fakeBook{})

	_ = schemeBuilder.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = ackv1alpha1.AddToScheme(scheme)
}

type fakeBook struct{}

func (b *fakeBook) GetNamespace() string                                       { return "" }
func (b *fakeBook) SetNamespace(namespace string)                              {}
func (b *fakeBook) GetName() string                                            { return "" }
func (b *fakeBook) SetName(name string)                                        {}
func (b *fakeBook) GetGenerateName() string                                    { return "" }
func (b *fakeBook) SetGenerateName(name string)                                {}
func (b *fakeBook) GetUID() types.UID                                          { return "" }
func (b *fakeBook) SetUID(uid types.UID)                                       {}
func (b *fakeBook) GetResourceVersion() string                                 { return "" }
func (b *fakeBook) SetResourceVersion(version string)                          {}
func (b *fakeBook) GetGeneration() int64                                       { return 0 }
func (b *fakeBook) SetGeneration(generation int64)                             {}
func (b *fakeBook) GetSelfLink() string                                        { return "" }
func (b *fakeBook) SetSelfLink(selfLink string)                                {}
func (b *fakeBook) GetCreationTimestamp() metav1.Time                          { return metav1.Time{} }
func (b *fakeBook) SetCreationTimestamp(timestamp metav1.Time)                 {}
func (b *fakeBook) GetDeletionTimestamp() *metav1.Time                         { return nil }
func (b *fakeBook) SetDeletionTimestamp(timestamp *metav1.Time)                {}
func (b *fakeBook) GetDeletionGracePeriodSeconds() *int64                      { return nil }
func (b *fakeBook) SetDeletionGracePeriodSeconds(i *int64)                     {}
func (b *fakeBook) GetLabels() map[string]string                               { return nil }
func (b *fakeBook) SetLabels(labels map[string]string)                         {}
func (b *fakeBook) GetAnnotations() map[string]string                          { return nil }
func (b *fakeBook) SetAnnotations(annotations map[string]string)               {}
func (b *fakeBook) GetFinalizers() []string                                    { return nil }
func (b *fakeBook) SetFinalizers(finalizers []string)                          {}
func (b *fakeBook) GetOwnerReferences() []metav1.OwnerReference                { return nil }
func (b *fakeBook) SetOwnerReferences(references []metav1.OwnerReference)      {}
func (b *fakeBook) GetClusterName() string                                     { return "" }
func (b *fakeBook) SetClusterName(clusterName string)                          {}
func (b *fakeBook) GetManagedFields() []metav1.ManagedFieldsEntry              { return nil }
func (b *fakeBook) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {}

func (b *fakeBook) GetObjectKind() schema.ObjectKind { return nil }
func (b *fakeBook) DeepCopy() *fakeBook              { return nil }
func (b *fakeBook) DeepCopyInto(*fakeBook)           {}
func (b *fakeBook) DeepCopyObject() runtime.Object   { return nil }

type fakeManager struct{}

func (m *fakeManager) GetLogger() logr.Logger {
	return logr.New(log.NullLogSink{})
}

func (m *fakeManager) GetControllerOptions() v1alpha1.ControllerConfigurationSpec {
	return v1alpha1.ControllerConfigurationSpec{}
}

func (m *fakeManager) Add(ctrlmanager.Runnable) error                                 { return nil }
func (m *fakeManager) Elected() <-chan struct{}                                       { return nil }
func (m *fakeManager) SetFields(interface{}) error                                    { return nil }
func (m *fakeManager) AddMetricsExtraHandler(path string, handler http.Handler) error { return nil }
func (m *fakeManager) AddHealthzCheck(name string, check healthz.Checker) error       { return nil }
func (m *fakeManager) AddReadyzCheck(name string, check healthz.Checker) error        { return nil }
func (m *fakeManager) Start(ctx context.Context) error                                { return nil }
func (m *fakeManager) GetConfig() *rest.Config                                        { return &rest.Config{} }
func (m *fakeManager) GetScheme() *runtime.Scheme                                     { return scheme }
func (m *fakeManager) GetClient() client.Client                                       { return nil }
func (m *fakeManager) GetFieldIndexer() client.FieldIndexer                           { return nil }
func (m *fakeManager) GetCache() cache.Cache                                          { return nil }
func (m *fakeManager) GetEventRecorderFor(name string) record.EventRecorder           { return nil }
func (m *fakeManager) GetRESTMapper() meta.RESTMapper                                 { return nil }
func (m *fakeManager) GetAPIReader() client.Reader                                    { return nil }
func (m *fakeManager) GetWebhookServer() *webhook.Server                              { return nil }

func TestServiceController(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupKind").Return(
		&metav1.GroupKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "fakeBook",
		},
	)
	rd.On("EmptyRuntimeObject").Return(
		&fakeBook{},
	)

	rmf := &mocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	reg := ackrt.NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)

	vi := acktypes.VersionInfo{
		GitCommit:  "test-commit",
		GitVersion: "test-version",
		BuildDate:  "now",
	}

	sc := ackrt.NewServiceController("bookstore", "bookstore.services.k8s.aws", "bookstore", vi)
	require.NotNil(sc)
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	sc.WithLogger(fakeLogger)
	sc.WithResourceManagerFactories(reg.GetResourceManagerFactories())

	recons := sc.GetReconcilers()

	// Before we bind to a controller manager, there are no reconcilers in the
	// service controller
	require.Empty(recons)

	mgr := &fakeManager{}
	cfg := ackcfg.Config{}
	err := sc.BindControllerManager(mgr, cfg)
	require.Nil(err)

	recons = sc.GetReconcilers()
	require.NotEmpty(recons)

	foundfakeBookRecon := false
	for _, recon := range recons {
		if recon.GroupKind().String() == "fakeBook.bookstore.services.k8s.aws" {
			foundfakeBookRecon = true
		}
	}
	require.True(foundfakeBookRecon)
	rd.AssertCalled(t, "EmptyRuntimeObject")
}
