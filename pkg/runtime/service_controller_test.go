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
	"net/http"
	"sync/atomic"
	"testing"
	"time"

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
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlrtconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	k8sscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	mocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	groupVersion := schema.GroupVersion{Group: "bookstore.services.k8s.aws", Version: "v1alpha1"}
	schemeBuilder := &k8sscheme.Builder{GroupVersion: groupVersion}
	schemeBuilder.Register(&fakeBook{}, &fakeLazyBook{}, &fakeCancelBook{})

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

type fakeLazyBook struct{ fakeBook }

func (b *fakeLazyBook) DeepCopyObject() runtime.Object { return nil }

type fakeCancelBook struct{ fakeBook }

func (b *fakeCancelBook) DeepCopyObject() runtime.Object { return nil }

type fakeRESTMapper struct {
	meta.RESTMapper
	kindForFunc func(resource schema.GroupVersionResource) (schema.GroupVersionKind, error)
}

func (m *fakeRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	if m.kindForFunc != nil {
		return m.kindForFunc(resource)
	}
	return schema.GroupVersionKind{}, &meta.NoResourceMatchError{PartialResource: resource}
}

type fakeManager struct {
	restMapper meta.RESTMapper
}

func (m *fakeManager) GetLogger() logr.Logger {
	return logr.New(log.NullLogSink{})
}

func (m *fakeManager) GetControllerOptions() ctrlrtconfig.Controller {
	return ctrlrtconfig.Controller{}
}

func (m *fakeManager) GetHTTPClient() *http.Client {
	return &http.Client{}
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
func (m *fakeManager) GetEventRecorder(name string) events.EventRecorder              { return nil }
func (m *fakeManager) GetRESTMapper() meta.RESTMapper {
	if m.restMapper != nil {
		return m.restMapper
	}
	return &fakeRESTMapper{}
}
func (m *fakeManager) GetAPIReader() client.Reader                              { return nil }
func (m *fakeManager) GetWebhookServer() webhook.Server                         { return nil }
func (m *fakeManager) AddMetricsServerExtraHandler(string, http.Handler) error  { return nil }
func (m *fakeManager) GetConverterRegistry() conversion.Registry                { return nil }

func TestServiceController(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		schema.GroupVersionKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "fakeBook",
		},
	)
	rd.On("GroupVersionResource").Return(
		schema.GroupVersionResource{
			Group:    "bookstore.services.k8s.aws",
			Version:  "v1alpha1",
			Resource: "fakebooks",
		},
	)
	rd.On("EmptyRuntimeObject").Return(
		&fakeBook{},
	)

	rmf := &mocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	reg := NewRegistry()
	reg.RegisterResourceManagerFactory(rmf)

	vi := acktypes.VersionInfo{
		GitCommit:  "test-commit",
		GitVersion: "test-version",
		BuildDate:  "now",
	}

	sc := NewServiceController("bookstore", "bookstore.services.k8s.aws", vi)
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
	cfg := ackcfg.Config{
		// Disable caches, by setting EnableCARM to false
		EnableCARM: false,
	}
	err := sc.BindControllerManager(context.Background(), mgr, cfg)
	require.Nil(err)

	recons = sc.GetReconcilers()
	require.NotEmpty(recons)

	foundfakeBookRecon := false
	for _, recon := range recons {
		if recon.GroupVersionKind().GroupKind().String() == "fakeBook.bookstore.services.k8s.aws" {
			foundfakeBookRecon = true
		}
	}
	require.True(foundfakeBookRecon)
	rd.AssertCalled(t, "EmptyRuntimeObject")
}

func TestBindControllerManagerStashesCfg(t *testing.T) {
	require := require.New(t)

	// Construct a serviceController directly (no rmFactories) to avoid
	// controller name collisions with TestServiceController.
	sc := &serviceController{
		ServiceControllerMetadata: acktypes.ServiceControllerMetadata{
			ServiceAlias:    "cfgtest",
			ServiceAPIGroup: "cfgtest.services.k8s.aws",
		},
	}
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	sc.log = ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))

	mgr := &fakeManager{}
	cfg := ackcfg.Config{
		EnableCARM:        false,
		HTTPClientTimeout: 7 * time.Second,
	}
	err := sc.BindControllerManager(context.Background(), mgr, cfg)
	require.Nil(err)

	require.Equal(7*time.Second, sc.cfg.HTTPClientTimeout)
}

func TestBindControllerManager_LazyBind_CRDNotInstalled(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		schema.GroupVersionKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "lazyFakeBook",
		},
	)
	rd.On("GroupVersionResource").Return(
		schema.GroupVersionResource{
			Group:    "bookstore.services.k8s.aws",
			Version:  "v1alpha1",
			Resource: "lazyfakebooks",
		},
	)
	rd.On("EmptyRuntimeObject").Return(&fakeLazyBook{})

	rmf := &mocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	var installCallCount int32
	sc := &serviceController{
		ServiceControllerMetadata: acktypes.ServiceControllerMetadata{
			ServiceAlias:    "lazytest",
			ServiceAPIGroup: "bookstore.services.k8s.aws",
		},
		rmFactories: map[string]acktypes.AWSResourceManagerFactory{
			"lazyFakeBook.bookstore.services.k8s.aws": rmf,
		},
	}
	sc.log = ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}))

	mapper := &fakeRESTMapper{
		kindForFunc: func(_ schema.GroupVersionResource) (schema.GroupVersionKind, error) {
			atomic.AddInt32(&installCallCount, 1)
			if atomic.LoadInt32(&installCallCount) >= 3 {
				return schema.GroupVersionKind{Group: "bookstore.services.k8s.aws", Version: "v1alpha1", Kind: "lazyFakeBook"}, nil
			}
			return schema.GroupVersionKind{}, &meta.NoResourceMatchError{}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr := &fakeManager{restMapper: mapper}
	cfg := ackcfg.Config{
		EnableCARM:            false,
		LazyBindReconcilers:   true,
		LazyBindRetryInterval: 100 * time.Millisecond,
	}

	err := sc.BindControllerManager(ctx, mgr, cfg)
	require.Nil(err)

	// Reconciler should not be bound yet (it's in the background)
	require.Empty(sc.GetReconcilers())

	// Wait for the background goroutine to poll and detect the CRD
	require.Eventually(func() bool {
		return atomic.LoadInt32(&installCallCount) >= 3
	}, 5*time.Second, 50*time.Millisecond)
}

func TestBindControllerManager_LazyBind_ContextCancelled(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		schema.GroupVersionKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "cancelFakeBook",
		},
	)
	rd.On("GroupVersionResource").Return(
		schema.GroupVersionResource{
			Group:    "bookstore.services.k8s.aws",
			Version:  "v1alpha1",
			Resource: "cancelfakebooks",
		},
	)
	rd.On("EmptyRuntimeObject").Return(&fakeCancelBook{})

	rmf := &mocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	sc := &serviceController{
		ServiceControllerMetadata: acktypes.ServiceControllerMetadata{
			ServiceAlias:    "canceltest",
			ServiceAPIGroup: "bookstore.services.k8s.aws",
		},
		rmFactories: map[string]acktypes.AWSResourceManagerFactory{
			"cancelFakeBook.bookstore.services.k8s.aws": rmf,
		},
	}
	sc.log = ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}))

	mapper := &fakeRESTMapper{
		kindForFunc: func(_ schema.GroupVersionResource) (schema.GroupVersionKind, error) {
			return schema.GroupVersionKind{}, &meta.NoResourceMatchError{}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	mgr := &fakeManager{restMapper: mapper}
	cfg := ackcfg.Config{
		EnableCARM:            false,
		LazyBindReconcilers:   true,
		LazyBindRetryInterval: 100 * time.Millisecond,
	}

	err := sc.BindControllerManager(ctx, mgr, cfg)
	require.Nil(err)
	require.Empty(sc.GetReconcilers())

	// Cancel context — goroutine should exit gracefully
	cancel()

	// Give the goroutine time to exit, then confirm no reconciler was bound
	time.Sleep(100 * time.Millisecond)
	require.Empty(sc.GetReconcilers())
}
