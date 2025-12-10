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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws-controllers-k8s/runtime/tests/integration/testutil"
)

var _ = Describe("Table Reconciliation", func() {
	var (
		namespace string
	)

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-%s", rand.String(5))

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Reset the fake resource manager for each test
		fakeRM.Reset()
	})

	Describe("CREATE", func() {
		It("calls Create on resource manager when resource doesn't exist", func() {
			table := &testutil.Table{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-table",
					Namespace: namespace,
				},
				Spec: testutil.TableSpec{TableName: "users"},
			}
			Expect(k8sClient.Create(ctx, table)).To(Succeed())

			// Wait for the controller to finish reconciling
			// After Create, the controller updates status which triggers another reconcile
			Eventually(func() int {
				return len(fakeRM.Operations)
			}, timeout, interval).Should(Equal(3))

			// Verify exact operation sequence:
			// 1st reconcile: ReadOne (NotFound) -> Create
			// 2nd reconcile (after status update): ReadOne (resource exists)
			Expect(fakeRM.Operations).To(HaveLen(3))
			Expect(fakeRM.Operations[0].Type).To(Equal("ReadOne"))
			Expect(fakeRM.Operations[1].Type).To(Equal("Create"))
			Expect(fakeRM.Operations[2].Type).To(Equal("ReadOne"))

			// Verify call counts
			Expect(fakeRM.ReadCalls).To(HaveLen(2))
			Expect(fakeRM.CreateCalls).To(HaveLen(1))
			Expect(fakeRM.UpdateCalls).To(HaveLen(0))
			Expect(fakeRM.DeleteCalls).To(HaveLen(0))

			// Fetch the latest version to verify status and metadata
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(table), table)).To(Succeed())

			// Verify finalizer is set
			Expect(table.Finalizers).To(ContainElement(testutil.FinalizerString))

			// Verify ACKResourceMetadata is populated
			Expect(table.Status.ACKResourceMetadata).NotTo(BeNil())
			Expect(table.Status.ACKResourceMetadata.ARN).NotTo(BeNil())
			Expect(string(*table.Status.ACKResourceMetadata.ARN)).To(ContainSubstring("arn:aws:test:"))

			// Verify conditions are set
			Expect(table.Status.Conditions).NotTo(BeEmpty())
		})
	})

	Describe("UPDATE", func() {
		It("calls Update when provisioned throughput changes", func() {
			table := &testutil.Table{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-table",
					Namespace: namespace,
				},
				Spec: testutil.TableSpec{
					TableName:   "my-table",
					BillingMode: ptr("PROVISIONED"),
					ProvisionedThroughput: &testutil.ProvisionedThroughput{
						ReadCapacityUnits:  ptr(int64(5)),
						WriteCapacityUnits: ptr(int64(5)),
					},
				},
			}
			Expect(k8sClient.Create(ctx, table)).To(Succeed())

			// Wait for the controller to create the resource
			Eventually(func() int {
				return len(fakeRM.CreateCalls)
			}, timeout, interval).Should(Equal(1))

			// Wait for operations to stabilize after create
			Eventually(func() int {
				return len(fakeRM.Operations)
			}, timeout, interval).Should(Equal(3))

			// Fetch the latest version of the resource
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(table), table)).To(Succeed())

			// Update provisioned throughput (scale up)
			table.Spec.ProvisionedThroughput.ReadCapacityUnits = ptr(int64(10))
			table.Spec.ProvisionedThroughput.WriteCapacityUnits = ptr(int64(10))
			Expect(k8sClient.Update(ctx, table)).To(Succeed())

			// Wait for the controller to process the update
			Eventually(func() int {
				return len(fakeRM.UpdateCalls)
			}, timeout, interval).Should(Equal(1))

			// Verify exact operation sequence:
			// Create phase (3 ops): ReadOne (NotFound) -> Create -> ReadOne
			// Update phase (2 ops): ReadOne -> Update
			Expect(fakeRM.Operations).To(HaveLen(5))
			Expect(fakeRM.Operations[0].Type).To(Equal("ReadOne"))
			Expect(fakeRM.Operations[1].Type).To(Equal("Create"))
			Expect(fakeRM.Operations[2].Type).To(Equal("ReadOne"))
			Expect(fakeRM.Operations[3].Type).To(Equal("ReadOne"))
			Expect(fakeRM.Operations[4].Type).To(Equal("Update"))

			// Verify call counts
			Expect(fakeRM.ReadCalls).To(HaveLen(3))
			Expect(fakeRM.CreateCalls).To(HaveLen(1))
			Expect(fakeRM.UpdateCalls).To(HaveLen(1))
			Expect(fakeRM.DeleteCalls).To(HaveLen(0))

			// Verify the update was called with the correct delta
			Expect(fakeRM.UpdateCalls[0].Delta.DifferentAt("Spec.ProvisionedThroughput.ReadCapacityUnits")).To(BeTrue())
			Expect(fakeRM.UpdateCalls[0].Delta.DifferentAt("Spec.ProvisionedThroughput.WriteCapacityUnits")).To(BeTrue())
		})
	})

	Describe("DELETE", func() {
		It("calls Delete when resource is being deleted", func() {
			table := &testutil.Table{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-table",
					Namespace: namespace,
				},
				Spec: testutil.TableSpec{TableName: "to-delete"},
			}
			Expect(k8sClient.Create(ctx, table)).To(Succeed())

			// Wait for the controller to create the resource
			Eventually(func() int {
				return len(fakeRM.CreateCalls)
			}, timeout, interval).Should(Equal(1))

			// Now delete the resource
			Expect(k8sClient.Delete(ctx, table)).To(Succeed())

			// Wait for the controller to delete the resource
			Eventually(func() int {
				return len(fakeRM.DeleteCalls)
			}, timeout, interval).Should(Equal(1))

			// Verify exact operation sequence:
			// Create phase (3 ops): ReadOne (NotFound) -> Create -> ReadOne (status update reconcile)
			// Delete phase (2 ops): ReadOne -> Delete
			Expect(fakeRM.Operations).To(HaveLen(5))
			Expect(fakeRM.Operations[0].Type).To(Equal("ReadOne"))
			Expect(fakeRM.Operations[1].Type).To(Equal("Create"))
			Expect(fakeRM.Operations[2].Type).To(Equal("ReadOne"))
			Expect(fakeRM.Operations[3].Type).To(Equal("ReadOne"))
			Expect(fakeRM.Operations[4].Type).To(Equal("Delete"))

			// Verify call counts
			Expect(fakeRM.ReadCalls).To(HaveLen(3))
			Expect(fakeRM.CreateCalls).To(HaveLen(1))
			Expect(fakeRM.UpdateCalls).To(HaveLen(0))
			Expect(fakeRM.DeleteCalls).To(HaveLen(1))
		})
	})
})

func ptr[T any](s T) *T {
	return &s
}
