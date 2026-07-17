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
	// PopulateResourceFromAnnotation will set the Spec or Status field that user
	// provided from annotations
	PopulateResourceFromAnnotation(fields map[string]string) error
	// IdentifierFieldsFromARN parses the supplied ARN into the same
	// map[string]string of ReadOne identifier fields that
	// PopulateResourceFromAnnotation consumes (i.e. using the keys that
	// PopulateResourceFromAnnotation expects). It is used during tag-based
	// adoption to derive the resource's identifier from the ARN returned by the
	// Resource Groups Tagging API. It returns an error if this resource kind
	// cannot derive its identifier from an ARN.
	IdentifierFieldsFromARN(arn string) (map[string]string, error)
	// ResourceTypeFilter returns the Resource Groups Tagging API resource-type
	// filter for this kind (e.g. "ec2:vpc"), or an empty string if the kind does
	// not support tag-based adoption.
	ResourceTypeFilter() string
}
