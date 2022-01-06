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

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// AWSResource represents a custom resource object in the Kubernetes API that
// corresponds to a resource in an AWS service API.
type AWSResource interface {
	ConditionManager
	// Identifiers returns an AWSResourceIdentifiers object containing various
	// identifying information, including the AWS account ID that owns the
	// resource, the resource's AWS Resource Name (ARN)
	Identifiers() AWSResourceIdentifiers
	// IsBeingDeleted returns true if the Kubernetes resource has a non-zero
	// deletion timestamp
	IsBeingDeleted() bool
	// RuntimeObject returns the Kubernetes apimachinery/runtime representation
	// of the AWSResource
	RuntimeObject() rtclient.Object
	// MetaObject returns the Kubernetes apimachinery/apis/meta/v1.Object
	// representation of the AWSResource
	// TODO(vijtrip2) Consider removing MetaObject method since RuntimeObject
	// returns superset of MetaObject
	MetaObject() metav1.Object
	// SetObjectMeta sets the ObjectMeta field for the resource
	SetObjectMeta(meta metav1.ObjectMeta)
	// SetIdentifiers will set the the Spec or Status field that represents the
	// identifier for the resource.
	SetIdentifiers(*ackv1alpha1.AWSIdentifiers) error
	// SetStatus will set the Status field for the resource
	SetStatus(AWSResource)
	// DeepCopy will return a copy of the resource
	DeepCopy() AWSResource
}
