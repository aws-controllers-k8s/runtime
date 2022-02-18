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

// FieldExportSpec defines the desired state of the FieldExport.
type FieldExportSpec struct {
	From *ResourceFieldSelector     `json:"from"`
	To   *FieldExportOutputSelector `json:"to"`
}

// FieldExportStatus defines the observed status of the FieldExport.
type FieldExportStatus struct {
	// A collection of `ackv1alpha1.Condition` objects that describe the various
	// terminal states of the adopted resource CR and its target custom resource
	Conditions []*Condition `json:"conditions"`
}

// FieldExport is the schema for the FieldExport API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type FieldExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FieldExportSpec   `json:"spec,omitempty"`
	Status            FieldExportStatus `json:"status,omitempty"`
}

// FieldExportList defines a list of FieldExports.
// +kubebuilder:object:root=true
type FieldExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FieldExport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FieldExport{}, &FieldExportList{})
}
