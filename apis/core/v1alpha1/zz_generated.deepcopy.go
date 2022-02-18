//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSIdentifiers) DeepCopyInto(out *AWSIdentifiers) {
	*out = *in
	if in.ARN != nil {
		in, out := &in.ARN, &out.ARN
		*out = new(AWSResourceName)
		**out = **in
	}
	if in.AdditionalKeys != nil {
		in, out := &in.AdditionalKeys, &out.AdditionalKeys
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSIdentifiers.
func (in *AWSIdentifiers) DeepCopy() *AWSIdentifiers {
	if in == nil {
		return nil
	}
	out := new(AWSIdentifiers)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSResourceReference) DeepCopyInto(out *AWSResourceReference) {
	*out = *in
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSResourceReference.
func (in *AWSResourceReference) DeepCopy() *AWSResourceReference {
	if in == nil {
		return nil
	}
	out := new(AWSResourceReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSResourceReferenceWrapper) DeepCopyInto(out *AWSResourceReferenceWrapper) {
	*out = *in
	if in.From != nil {
		in, out := &in.From, &out.From
		*out = new(AWSResourceReference)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSResourceReferenceWrapper.
func (in *AWSResourceReferenceWrapper) DeepCopy() *AWSResourceReferenceWrapper {
	if in == nil {
		return nil
	}
	out := new(AWSResourceReferenceWrapper)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdoptedResource) DeepCopyInto(out *AdoptedResource) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdoptedResource.
func (in *AdoptedResource) DeepCopy() *AdoptedResource {
	if in == nil {
		return nil
	}
	out := new(AdoptedResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AdoptedResource) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdoptedResourceList) DeepCopyInto(out *AdoptedResourceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AdoptedResource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdoptedResourceList.
func (in *AdoptedResourceList) DeepCopy() *AdoptedResourceList {
	if in == nil {
		return nil
	}
	out := new(AdoptedResourceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AdoptedResourceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdoptedResourceSpec) DeepCopyInto(out *AdoptedResourceSpec) {
	*out = *in
	if in.Kubernetes != nil {
		in, out := &in.Kubernetes, &out.Kubernetes
		*out = new(AdoptedResourceTarget)
		(*in).DeepCopyInto(*out)
	}
	if in.AWS != nil {
		in, out := &in.AWS, &out.AWS
		*out = new(AWSIdentifiers)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdoptedResourceSpec.
func (in *AdoptedResourceSpec) DeepCopy() *AdoptedResourceSpec {
	if in == nil {
		return nil
	}
	out := new(AdoptedResourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdoptedResourceStatus) DeepCopyInto(out *AdoptedResourceStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]*Condition, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(Condition)
				(*in).DeepCopyInto(*out)
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdoptedResourceStatus.
func (in *AdoptedResourceStatus) DeepCopy() *AdoptedResourceStatus {
	if in == nil {
		return nil
	}
	out := new(AdoptedResourceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdoptedResourceTarget) DeepCopyInto(out *AdoptedResourceTarget) {
	*out = *in
	out.GroupKind = in.GroupKind
	if in.Metadata != nil {
		in, out := &in.Metadata, &out.Metadata
		*out = new(PartialObjectMeta)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdoptedResourceTarget.
func (in *AdoptedResourceTarget) DeepCopy() *AdoptedResourceTarget {
	if in == nil {
		return nil
	}
	out := new(AdoptedResourceTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Condition) DeepCopyInto(out *Condition) {
	*out = *in
	if in.LastTransitionTime != nil {
		in, out := &in.LastTransitionTime, &out.LastTransitionTime
		*out = (*in).DeepCopy()
	}
	if in.Reason != nil {
		in, out := &in.Reason, &out.Reason
		*out = new(string)
		**out = **in
	}
	if in.Message != nil {
		in, out := &in.Message, &out.Message
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Condition.
func (in *Condition) DeepCopy() *Condition {
	if in == nil {
		return nil
	}
	out := new(Condition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FieldExport) DeepCopyInto(out *FieldExport) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FieldExport.
func (in *FieldExport) DeepCopy() *FieldExport {
	if in == nil {
		return nil
	}
	out := new(FieldExport)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *FieldExport) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FieldExportList) DeepCopyInto(out *FieldExportList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]FieldExport, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FieldExportList.
func (in *FieldExportList) DeepCopy() *FieldExportList {
	if in == nil {
		return nil
	}
	out := new(FieldExportList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *FieldExportList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FieldExportOutputSelector) DeepCopyInto(out *FieldExportOutputSelector) {
	*out = *in
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
	if in.Namespace != nil {
		in, out := &in.Namespace, &out.Namespace
		*out = new(string)
		**out = **in
	}
	if in.Type != nil {
		in, out := &in.Type, &out.Type
		*out = new(FieldExportOutputType)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FieldExportOutputSelector.
func (in *FieldExportOutputSelector) DeepCopy() *FieldExportOutputSelector {
	if in == nil {
		return nil
	}
	out := new(FieldExportOutputSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FieldExportSpec) DeepCopyInto(out *FieldExportSpec) {
	*out = *in
	if in.From != nil {
		in, out := &in.From, &out.From
		*out = new(ResourceFieldSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.To != nil {
		in, out := &in.To, &out.To
		*out = new(FieldExportOutputSelector)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FieldExportSpec.
func (in *FieldExportSpec) DeepCopy() *FieldExportSpec {
	if in == nil {
		return nil
	}
	out := new(FieldExportSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FieldExportStatus) DeepCopyInto(out *FieldExportStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]*Condition, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(Condition)
				(*in).DeepCopyInto(*out)
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FieldExportStatus.
func (in *FieldExportStatus) DeepCopy() *FieldExportStatus {
	if in == nil {
		return nil
	}
	out := new(FieldExportStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespacedTargetKubernetesResource) DeepCopyInto(out *NamespacedTargetKubernetesResource) {
	*out = *in
	out.GroupKind = in.GroupKind
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespacedTargetKubernetesResource.
func (in *NamespacedTargetKubernetesResource) DeepCopy() *NamespacedTargetKubernetesResource {
	if in == nil {
		return nil
	}
	out := new(NamespacedTargetKubernetesResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PartialObjectMeta) DeepCopyInto(out *PartialObjectMeta) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.OwnerReferences != nil {
		in, out := &in.OwnerReferences, &out.OwnerReferences
		*out = make([]v1.OwnerReference, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PartialObjectMeta.
func (in *PartialObjectMeta) DeepCopy() *PartialObjectMeta {
	if in == nil {
		return nil
	}
	out := new(PartialObjectMeta)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceFieldSelector) DeepCopyInto(out *ResourceFieldSelector) {
	*out = *in
	in.Resource.DeepCopyInto(&out.Resource)
	if in.Path != nil {
		in, out := &in.Path, &out.Path
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceFieldSelector.
func (in *ResourceFieldSelector) DeepCopy() *ResourceFieldSelector {
	if in == nil {
		return nil
	}
	out := new(ResourceFieldSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceMetadata) DeepCopyInto(out *ResourceMetadata) {
	*out = *in
	if in.ARN != nil {
		in, out := &in.ARN, &out.ARN
		*out = new(AWSResourceName)
		**out = **in
	}
	if in.OwnerAccountID != nil {
		in, out := &in.OwnerAccountID, &out.OwnerAccountID
		*out = new(AWSAccountID)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceMetadata.
func (in *ResourceMetadata) DeepCopy() *ResourceMetadata {
	if in == nil {
		return nil
	}
	out := new(ResourceMetadata)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SecretKeyReference) DeepCopyInto(out *SecretKeyReference) {
	*out = *in
	out.SecretReference = in.SecretReference
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SecretKeyReference.
func (in *SecretKeyReference) DeepCopy() *SecretKeyReference {
	if in == nil {
		return nil
	}
	out := new(SecretKeyReference)
	in.DeepCopyInto(out)
	return out
}
