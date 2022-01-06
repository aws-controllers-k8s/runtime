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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
)

// AWSResourceDescriptor provides metadata that describes the Kubernetes
// metadata associated with an AWSResource, the Kubernetes runtime.Object
// prototype for that AWSResource, and the relationships between the
// AWSResource and other AWSResources
type AWSResourceDescriptor interface {
	// GroupKind returns a Kubernetes metav1.GroupKind struct that describes
	// the API Group and Kind of CRs described by the descriptor
	GroupKind() *metav1.GroupKind
	// EmptyRuntimeObject returns an empty object prototype that may be used in
	// apimachinery and k8s client operations
	EmptyRuntimeObject() rtclient.Object
	// ResourceFromRuntimeObject returns an AWSResource that has been
	// initialized with the supplied runtime.Object
	ResourceFromRuntimeObject(rtclient.Object) AWSResource
	// Delta returns an `ackcompare.Delta` object containing the difference between
	// one `AWSResource` and another.
	Delta(a, b AWSResource) *ackcompare.Delta
	// IsManaged returns true if the supplied AWSResource is under the
	// management of an ACK service controller. What this means in practice is
	// that the underlying custom resource (CR) in the AWSResource has had a
	// resource-specific finalizer associated with it.
	IsManaged(AWSResource) bool
	// MarkManaged places the supplied resource under the management of ACK.
	// What this typically means is that the resource manager will decorate the
	// underlying custom resource (CR) with a finalizer that indicates ACK is
	// managing the resource and the underlying CR may not be deleted until ACK
	// is finished cleaning up any backend AWS service resources associated
	// with the CR.
	MarkManaged(AWSResource)
	// MarkUnmanaged removes the supplied resource from management by ACK.
	// What this typically means is that the resource manager will remove a
	// finalizer underlying custom resource (CR) that indicates ACK is managing
	// the resource. This will allow the Kubernetes API server to delete the
	// underlying CR.
	MarkUnmanaged(AWSResource)
	// MarkAdopted places descriptors on the custom resource that indicate the
	// resource was not created from within ACK.
	MarkAdopted(AWSResource)
}
