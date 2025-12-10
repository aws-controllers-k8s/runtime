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

package core_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	"github.com/aws-controllers-k8s/runtime/tests/integration/testutil"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	fakeRM    *testutil.FakeResourceManager
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

func TestCore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Core Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDs: []*apiextensionsv1.CustomResourceDefinition{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tables.dynamodb.services.k8s.aws",
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "dynamodb.services.k8s.aws",
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Kind:     "Table",
						ListKind: "TableList",
						Plural:   "tables",
						Singular: "table",
					},
					Scope: apiextensionsv1.NamespaceScoped,
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Served:  true,
							Storage: true,
							Schema: &apiextensionsv1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"spec": {
											Type: "object",
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"tableName":          {Type: "string"},
												"billingMode":        {Type: "string"},
												"deletionProtection": {Type: "boolean"},
												"provisionedThroughput": {
													Type: "object",
													Properties: map[string]apiextensionsv1.JSONSchemaProps{
														"readCapacityUnits":  {Type: "integer"},
														"writeCapacityUnits": {Type: "integer"},
													},
												},
												"sseSpecification": {
													Type: "object",
													Properties: map[string]apiextensionsv1.JSONSchemaProps{
														"enabled": {Type: "boolean"},
														"sseType": {Type: "string"},
													},
												},
												"tags": {
													Type: "array",
													Items: &apiextensionsv1.JSONSchemaPropsOrArray{
														Schema: &apiextensionsv1.JSONSchemaProps{
															Type: "object",
															Properties: map[string]apiextensionsv1.JSONSchemaProps{
																"key":   {Type: "string"},
																"value": {Type: "string"},
															},
														},
													},
												},
											},
										},
										"status": {
											Type:                   "object",
											XPreserveUnknownFields: boolPtr(true),
										},
									},
								},
							},
							Subresources: &apiextensionsv1.CustomResourceSubresources{
								Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
							},
						},
					},
				},
			},
		},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = ackv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = testutil.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create a controller-runtime manager
	mgr, err := ctrlrt.NewManager(cfg, ctrlrt.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server for tests
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Create the fake resource manager factory
	rd := testutil.NewTableDescriptor()
	rmf := testutil.NewFakeResourceManagerFactory(rd)
	fakeRM = rmf.GetFakeResourceManager()

	// Set up the service controller using the proper ACK pattern
	_, err = testutil.SetupServiceController(mgr, rmf, testutil.DefaultTestConfig())
	Expect(err).NotTo(HaveOccurred())

	// Start the manager in a goroutine
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// Get the k8s client from the manager (has caching)
	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())
})

func boolPtr(b bool) *bool {
	return &b
}

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
