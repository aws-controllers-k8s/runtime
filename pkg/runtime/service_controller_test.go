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
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	k8sscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackrt "github.com/aws-controllers-k8s/runtime/pkg/runtime"

	mocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
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

func (b *fakeBook) GetObjectKind() schema.ObjectKind { return nil }
func (b *fakeBook) DeepCopy() *fakeBook              { return nil }
func (b *fakeBook) DeepCopyInto(*fakeBook)           {}
func (b *fakeBook) DeepCopyObject() runtime.Object   { return nil }

type fakeManager struct{}

func (m *fakeManager) Add(ctrlmanager.Runnable) error                                 { return nil }
func (m *fakeManager) Elected() <-chan struct{}                                       { return nil }
func (m *fakeManager) SetFields(interface{}) error                                    { return nil }
func (m *fakeManager) AddMetricsExtraHandler(path string, handler http.Handler) error { return nil }
func (m *fakeManager) AddHealthzCheck(name string, check healthz.Checker) error       { return nil }
func (m *fakeManager) AddReadyzCheck(name string, check healthz.Checker) error        { return nil }
func (m *fakeManager) Start(<-chan struct{}) error                                    { return nil }
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

	reg := ackrt.NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)

	vi := ackrt.VersionInfo{
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
