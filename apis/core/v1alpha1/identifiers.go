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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AWSIdentifiers provide all unique ways to reference an AWS resource.
type AWSIdentifiers struct {
	// ARN is the AWS Resource Name for the resource. It is a globally
	// unique identifier.
	ARN *AWSResourceName `json:"arn,omitempty"`
	// NameOrId is a user-supplied string identifier for the resource. It may
	// or may not be globally unique, depending on the type of resource.
	NameOrID string `json:"nameOrID,omitempty"`
	// AdditionalKeys represents any additional arbitrary identifiers used when
	// describing the target resource.
	AdditionalKeys map[string]string `json:"additionalKeys,omitempty"`
}

// NamespacedTargetKubernetesResource provides all the values necessary to identify an ACK
// resource of a given type (within the same namespace).
type NamespacedTargetKubernetesResource struct {
	metav1.GroupKind `json:""`
	Name             *string `json:"name"`
}

// AdoptedResourceTarget provides the values necessary to create a
// Kubernetes resource and override any of its metadata values.
type AdoptedResourceTarget struct {
	metav1.GroupKind `json:""`
	Metadata         *PartialObjectMeta `json:"metadata,omitempty"`
}

// ResourceFieldSelector provides the values necessary to identify an individual
// path on an individual K8s resource.
type ResourceFieldSelector struct {
	Resource NamespacedTargetKubernetesResource `json:"resource"`
	Path     *string                            `json:"path"`
}

// FieldExportOutputSelector provides the values necessary to identify the
// output path for a field export.
type FieldExportOutputSelector struct {
	Name *string `json:"name"`
	// Namespace is marked as optional, so we cannot compose `NamespacedName`
	Namespace *string                `json:"namespace,omitempty"`
	Kind      *FieldExportOutputType `json:"kind"`
}

// AWSResourceReferenceWrapper provides a wrapper around *AWSResourceReference
// type to provide more user friendly syntax for references using 'from' field
// Ex:
// APIIDRef:
//   from:
//     name: my-api
type AWSResourceReferenceWrapper struct {
	From *AWSResourceReference `json:"from,omitempty"`
}

// AWSResourceReference provides all the values necessary to reference another
// k8s resource for finding the identifier(Id/ARN/Name)
type AWSResourceReference struct {
	Name *string `json:"name,omitempty"`
}
