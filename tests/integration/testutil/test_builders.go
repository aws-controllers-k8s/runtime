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

package testutil

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackruntime "github.com/aws-controllers-k8s/runtime/pkg/runtime"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// FakeResourceManagerFactory creates FakeResourceManager instances
type FakeResourceManagerFactory struct {
	descriptor       acktypes.AWSResourceDescriptor
	resourceManager  *FakeResourceManager
	isAdoptable      bool
	requeueOnSuccess int
}

func NewFakeResourceManagerFactory(rd acktypes.AWSResourceDescriptor) *FakeResourceManagerFactory {
	return &FakeResourceManagerFactory{
		descriptor:       rd,
		resourceManager:  NewFakeResourceManager(),
		isAdoptable:      true,
		requeueOnSuccess: 0,
	}
}

func (f *FakeResourceManagerFactory) ResourceDescriptor() acktypes.AWSResourceDescriptor {
	return f.descriptor
}

func (f *FakeResourceManagerFactory) ManagerFor(
	cfg ackcfg.Config,
	awsCfg aws.Config,
	log logr.Logger,
	metrics *ackmetrics.Metrics,
	reconciler acktypes.Reconciler,
	accountID ackv1alpha1.AWSAccountID,
	region ackv1alpha1.AWSRegion,
	roleARN ackv1alpha1.AWSResourceName,
) (acktypes.AWSResourceManager, error) {
	return f.resourceManager, nil
}

func (f *FakeResourceManagerFactory) IsAdoptable() bool {
	return f.isAdoptable
}

func (f *FakeResourceManagerFactory) RequeueOnSuccessSeconds() int {
	return f.requeueOnSuccess
}

// GetFakeResourceManager returns the underlying fake resource manager for test assertions
func (f *FakeResourceManagerFactory) GetFakeResourceManager() *FakeResourceManager {
	return f.resourceManager
}

// FakeResourceDescriptor implements AWSResourceDescriptor for testing
type FakeResourceDescriptor struct {
	gvk             schema.GroupVersionKind
	emptyRuntimeObj client.Object
	resourceFromObj func(client.Object) acktypes.AWSResource
	isManaged       func(acktypes.AWSResource) bool
	markManaged     func(acktypes.AWSResource)
	markUnmanaged   func(acktypes.AWSResource)
	markAdopted     func(acktypes.AWSResource)
	delta           func(acktypes.AWSResource, acktypes.AWSResource) *ackcompare.Delta
}

func NewFakeResourceDescriptor(gvk schema.GroupVersionKind, emptyObj client.Object) *FakeResourceDescriptor {
	return &FakeResourceDescriptor{
		gvk:             gvk,
		emptyRuntimeObj: emptyObj,
		// Default implementations
		isManaged: func(res acktypes.AWSResource) bool {
			finalizers := res.MetaObject().GetFinalizers()
			for _, f := range finalizers {
				if f == "finalizers.ack.services.k8s.aws/"+gvk.Kind {
					return true
				}
			}
			return false
		},
		markManaged: func(res acktypes.AWSResource) {
			finalizer := "finalizers.ack.services.k8s.aws/" + gvk.Kind
			finalizers := res.MetaObject().GetFinalizers()
			for _, f := range finalizers {
				if f == finalizer {
					return
				}
			}
			res.MetaObject().SetFinalizers(append(finalizers, finalizer))
		},
		markUnmanaged: func(res acktypes.AWSResource) {
			finalizer := "finalizers.ack.services.k8s.aws/" + gvk.Kind
			finalizers := res.MetaObject().GetFinalizers()
			newFinalizers := []string{}
			for _, f := range finalizers {
				if f != finalizer {
					newFinalizers = append(newFinalizers, f)
				}
			}
			res.MetaObject().SetFinalizers(newFinalizers)
		},
		markAdopted: func(res acktypes.AWSResource) {
			annotations := res.MetaObject().GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[ackv1alpha1.AnnotationAdopted] = "true"
			res.MetaObject().SetAnnotations(annotations)
		},
	}
}

func (f *FakeResourceDescriptor) GroupVersionKind() schema.GroupVersionKind {
	return f.gvk
}

func (f *FakeResourceDescriptor) EmptyRuntimeObject() client.Object {
	return f.emptyRuntimeObj
}

func (f *FakeResourceDescriptor) ResourceFromRuntimeObject(obj client.Object) acktypes.AWSResource {
	if f.resourceFromObj != nil {
		return f.resourceFromObj(obj)
	}
	// Fallback: assume obj implements AWSResource
	if res, ok := obj.(acktypes.AWSResource); ok {
		return res
	}
	return nil
}

func (f *FakeResourceDescriptor) IsManaged(res acktypes.AWSResource) bool {
	return f.isManaged(res)
}

func (f *FakeResourceDescriptor) MarkManaged(res acktypes.AWSResource) {
	f.markManaged(res)
}

func (f *FakeResourceDescriptor) MarkUnmanaged(res acktypes.AWSResource) {
	f.markUnmanaged(res)
}

func (f *FakeResourceDescriptor) MarkAdopted(res acktypes.AWSResource) {
	f.markAdopted(res)
}

func (f *FakeResourceDescriptor) Delta(a, b acktypes.AWSResource) *ackcompare.Delta {
	if f.delta != nil {
		return f.delta(a, b)
	}
	// Default: no differences
	return ackcompare.NewDelta()
}

// SetupServiceController creates a service controller and binds it to the manager.
// This follows the same pattern as real ACK controllers (e.g., s3-controller).
func SetupServiceController(
	mgr ctrlrt.Manager,
	rmf acktypes.AWSResourceManagerFactory,
	cfg ackcfg.Config,
) (acktypes.ServiceController, error) {
	sc := ackruntime.NewServiceController(
		"dynamodb", // service alias
		"dynamodb.services.k8s.aws",
		acktypes.VersionInfo{
			GitVersion: "v0.0.1-test",
			GitCommit:  "test",
			BuildDate:  "2024-01-01",
		},
	).WithLogger(
		ctrlrt.Log,
	).WithResourceManagerFactories(
		[]acktypes.AWSResourceManagerFactory{rmf},
	)

	if err := sc.BindControllerManager(mgr, cfg); err != nil {
		return nil, err
	}

	return sc, nil
}

// DefaultTestConfig returns a default configuration for tests
func DefaultTestConfig() ackcfg.Config {
	return ackcfg.Config{
		AccountID:      "123456789012",
		Region:         "us-west-2",
		DeletionPolicy: ackv1alpha1.DeletionPolicyDelete,
	}
}
