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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// LabelSelector is a label query over a set of resources.
type LabelSelector struct {
	MatchLabels map[string]string `json:"matchLabels"`
}

// IAMRoleSelectorSpec defines the desired state of IAMRoleSelector
type NamespaceSelector struct {
	Names         []string      `json:"names"`
	LabelSelector LabelSelector `json:"labelSelector,omitempty"`
}

type IAMRoleSelectorSpec struct {
	ARN                  string                    `json:"arn"`
	NamespaceSelector    NamespaceSelector         `json:"namespaceSelector,omitempty"`
	ResourceTypeSelector []schema.GroupVersionKind `json:"resourceTypeSelector,omitempty"`
}

type IAMRoleSelectorStatus struct{}

// IAMRoleSelector is the schema for the IAMRoleSelector API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
type IAMRoleSelector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IAMRoleSelectorSpec   `json:"spec,omitempty"`
	Status            IAMRoleSelectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type IAMRoleSelectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMRoleSelector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMRoleSelector{}, &IAMRoleSelectorList{})
}
