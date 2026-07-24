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
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlrtconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	k8sscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

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

type fakeManager struct {
	// restCfg, when non-nil, is returned by GetConfig. It lets a test point
	// the manager (and therefore the discovery/RESTMapper clients built from
	// it) at a fake API server.
	restCfg *rest.Config
	// skipNameValidation, when true, makes GetControllerOptions disable
	// controller-runtime's process-global unique-name validation so a test can
	// bind reconcilers for the same GVK as other tests without collisions.
	skipNameValidation bool
}

func (m *fakeManager) GetLogger() logr.Logger {
	return logr.New(log.NullLogSink{})
}

func (m *fakeManager) GetControllerOptions() ctrlrtconfig.Controller {
	if m.skipNameValidation {
		return ctrlrtconfig.Controller{SkipNameValidation: ptr.To(true)}
	}
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
func (m *fakeManager) GetConfig() *rest.Config {
	if m.restCfg != nil {
		return m.restCfg
	}
	return &rest.Config{}
}
func (m *fakeManager) GetScheme() *runtime.Scheme                                     { return scheme }
func (m *fakeManager) GetClient() client.Client                                       { return nil }
func (m *fakeManager) GetFieldIndexer() client.FieldIndexer                           { return nil }
func (m *fakeManager) GetCache() cache.Cache                                          { return nil }
func (m *fakeManager) GetEventRecorderFor(name string) record.EventRecorder           { return nil }
func (m *fakeManager) GetEventRecorder(name string) events.EventRecorder              { return nil }
func (m *fakeManager) GetRESTMapper() meta.RESTMapper                                 { return nil }
func (m *fakeManager) GetAPIReader() client.Reader                                    { return nil }
func (m *fakeManager) GetWebhookServer() webhook.Server                               { return nil }
func (m *fakeManager) AddMetricsServerExtraHandler(string, http.Handler) error        { return nil }
func (m *fakeManager) GetConverterRegistry() conversion.Registry                      { return nil }

func TestServiceController(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		schema.GroupVersionKind{
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
	err := sc.BindControllerManager(mgr, cfg)
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
	err := sc.BindControllerManager(mgr, cfg)
	require.Nil(err)

	require.Equal(7*time.Second, sc.cfg.HTTPClientTimeout)
}

// newFieldExportInstalledAPIServer starts an httptest server that serves the
// minimal Kubernetes discovery documents needed for
// (*serviceController).GetFieldExportInstalled to report the FieldExport CRD as
// installed: the services.k8s.aws/v1alpha1 group must be discoverable and it
// must contain the "fieldexports" resource. It returns a *rest.Config pointed at
// the server so a fakeManager can hand it to the discovery/RESTMapper clients.
func newFieldExportInstalledAPIServer(t *testing.T) *rest.Config {
	t.Helper()

	gv := ackv1alpha1.GroupVersion // services.k8s.aws/v1alpha1

	writeJSON := func(w http.ResponseWriter, obj interface{}) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(obj); err != nil {
			t.Errorf("encoding discovery response: %v", err)
		}
	}

	mux := http.NewServeMux()
	// Legacy core discovery. Return an empty group list so the discovery client
	// treats the server as reachable with no core resources.
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, &metav1.APIVersions{Versions: []string{}})
	})
	// Aggregated /apis discovery listing the services.k8s.aws group. This backs
	// both discovery.ServerSupportsVersion and the dynamic RESTMapper's group
	// lookup.
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, &metav1.APIGroupList{
			Groups: []metav1.APIGroup{
				{
					Name: gv.Group,
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: gv.String(), Version: gv.Version},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{
						GroupVersion: gv.String(), Version: gv.Version,
					},
				},
			},
		})
	})
	// Per-group-version resource list containing the fieldexports resource, so
	// RESTMapper.KindFor(fieldexports) resolves instead of returning a
	// NoMatchError.
	mux.HandleFunc("/apis/"+gv.String(), func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, &metav1.APIResourceList{
			GroupVersion: gv.String(),
			APIResources: []metav1.APIResource{
				{
					Name:       "fieldexports",
					Namespaced: true,
					Kind:       "FieldExport",
					Group:      gv.Group,
					Version:    gv.Version,
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &rest.Config{Host: server.URL}
}

// TestBindControllerManager_FieldExportReconcilersRegistered is a regression
// test for the variable-shadowing bug fixed in
// aws-controllers-k8s/community#2964: a `exporterInstalled, err :=` inside the
// EnableFieldExportReconciler block shadowed the outer exporterInstalled, which
// therefore stayed false, so the per-resource field-export.<kind> reconcilers
// were never registered even when the FieldExport CRD was installed.
//
// The test points the manager at a fake API server that reports the FieldExport
// CRD as installed and asserts that binding registers a per-resource
// FieldExport reconciler. With the shadowing bug this slice stays empty.
func TestBindControllerManager_FieldExportReconcilersRegistered(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		schema.GroupVersionKind{
			Group:   "bookstore.services.k8s.aws",
			Version: "v1alpha1",
			Kind:    "fakeBook",
		},
	)
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	rmf := &mocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	zapOptions := ctrlrtzap.Options{Development: true, Level: zapcore.InfoLevel}
	sc := &serviceController{
		ServiceControllerMetadata: acktypes.ServiceControllerMetadata{
			ServiceAlias:    "bookstore-fe",
			ServiceAPIGroup: "bookstore.services.k8s.aws",
		},
		log: ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions)),
	}
	sc.WithResourceManagerFactories([]acktypes.AWSResourceManagerFactory{rmf})

	mgr := &fakeManager{
		restCfg:            newFieldExportInstalledAPIServer(t),
		skipNameValidation: true,
	}
	cfg := ackcfg.Config{
		EnableCARM:                  false,
		EnableFieldExportReconciler: true,
	}

	err := sc.BindControllerManager(mgr, cfg)
	require.Nil(err)

	// The CRD-level FieldExport reconciler is registered inside the
	// EnableFieldExportReconciler block regardless of the bug.
	require.NotNil(sc.fieldExportReconciler)
	// The per-resource field-export.<kind> reconcilers are what the shadowing
	// bug disabled. They must be registered, one per reconciled resource.
	require.NotEmpty(sc.resourceFieldExportReconcilers,
		"per-resource FieldExport reconcilers were not registered; "+
			"exporterInstalled likely shadowed in BindControllerManager")
	require.Len(sc.resourceFieldExportReconcilers, len(sc.reconcilers))
}

// TestBindControllerManager_FieldExportReconcilersNotRegisteredWhenCRDMissing
// verifies the complementary path: when the FieldExport CRD is not installed,
// neither the CRD-level nor the per-resource FieldExport reconcilers are
// registered even though EnableFieldExportReconciler is true. The default
// fakeManager returns an empty rest.Config, so discovery reports the CRD as
// absent.
func TestBindControllerManager_FieldExportReconcilersNotRegisteredWhenCRDMissing(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		schema.GroupVersionKind{
			Group:   "bookstore.services.k8s.aws",
			Version: "v1alpha1",
			Kind:    "fakeBook",
		},
	)
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})

	rmf := &mocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	zapOptions := ctrlrtzap.Options{Development: true, Level: zapcore.InfoLevel}
	sc := &serviceController{
		ServiceControllerMetadata: acktypes.ServiceControllerMetadata{
			ServiceAlias:    "bookstore-nofe",
			ServiceAPIGroup: "bookstore.services.k8s.aws",
		},
		log: ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions)),
	}
	sc.WithResourceManagerFactories([]acktypes.AWSResourceManagerFactory{rmf})

	mgr := &fakeManager{skipNameValidation: true}
	cfg := ackcfg.Config{
		EnableCARM:                  false,
		EnableFieldExportReconciler: true,
	}

	err := sc.BindControllerManager(mgr, cfg)
	require.Nil(err)

	require.Nil(sc.fieldExportReconciler)
	require.Empty(sc.resourceFieldExportReconcilers)
}
