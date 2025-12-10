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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

var (
	// SchemeGroupVersion is the GroupVersion for Table
	SchemeGroupVersion = schema.GroupVersion{Group: "dynamodb.services.k8s.aws", Version: "v1alpha1"}

	// SchemeBuilder is used to add Table to the scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds Table types to the scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Table{},
		&TableList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// TableList is a list of Table resources
type TableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Table `json:"items"`
}

func (t *TableList) DeepCopyObject() runtime.Object {
	return &TableList{
		TypeMeta: t.TypeMeta,
		ListMeta: *t.ListMeta.DeepCopy(),
		Items:    append([]Table{}, t.Items...),
	}
}

// Table is a test resource modeled after DynamoDB Table
type Table struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TableSpec   `json:"spec,omitempty"`
	Status            TableStatus `json:"status,omitempty"`
}

type TableSpec struct {
	TableName             string                 `json:"tableName"`
	BillingMode           *string                `json:"billingMode,omitempty"`
	DeletionProtection    *bool                  `json:"deletionProtection,omitempty"`
	ProvisionedThroughput *ProvisionedThroughput `json:"provisionedThroughput,omitempty"`
	SSESpecification      *SSESpecification      `json:"sseSpecification,omitempty"`
	Tags                  []*Tag                 `json:"tags,omitempty"`
}

type ProvisionedThroughput struct {
	ReadCapacityUnits  *int64 `json:"readCapacityUnits,omitempty"`
	WriteCapacityUnits *int64 `json:"writeCapacityUnits,omitempty"`
}

type SSESpecification struct {
	Enabled *bool   `json:"enabled,omitempty"`
	SSEType *string `json:"sseType,omitempty"`
}

type Tag struct {
	Key   *string `json:"key,omitempty"`
	Value *string `json:"value,omitempty"`
}

type TableStatus struct {
	ACKResourceMetadata *ackv1alpha1.ResourceMetadata `json:"ackResourceMetadata,omitempty"`
	CreationDateTime    *metav1.Time                  `json:"creationDateTime,omitempty"`
	LastUpdateDateTime  *metav1.Time                  `json:"lastUpdateDateTime,omitempty"`
	TableID             *string                       `json:"tableID,omitempty"`
	TableStatus         *string                       `json:"tableStatus,omitempty"`
	Conditions          []*ackv1alpha1.Condition      `json:"conditions,omitempty"`
}

// tableIdentifiers implements AWSResourceIdentifiers
type tableIdentifiers struct {
	arn            *ackv1alpha1.AWSResourceName
	ownerAccountID *ackv1alpha1.AWSAccountID
	region         *ackv1alpha1.AWSRegion
}

func (ids *tableIdentifiers) ARN() *ackv1alpha1.AWSResourceName {
	return ids.arn
}

func (ids *tableIdentifiers) OwnerAccountID() *ackv1alpha1.AWSAccountID {
	return ids.ownerAccountID
}

func (ids *tableIdentifiers) Region() *ackv1alpha1.AWSRegion {
	return ids.region
}

func (t *Table) Identifiers() acktypes.AWSResourceIdentifiers {
	if t.Status.ACKResourceMetadata == nil {
		return &tableIdentifiers{}
	}
	return &tableIdentifiers{
		arn:            t.Status.ACKResourceMetadata.ARN,
		ownerAccountID: t.Status.ACKResourceMetadata.OwnerAccountID,
		region:         t.Status.ACKResourceMetadata.Region,
	}
}

func (t *Table) IsBeingDeleted() bool {
	return !t.DeletionTimestamp.IsZero()
}

func (t *Table) RuntimeObject() rtclient.Object {
	return t
}

func (t *Table) MetaObject() metav1.Object {
	return t
}

func (t *Table) Conditions() []*ackv1alpha1.Condition {
	return t.Status.Conditions
}

func (t *Table) ReplaceConditions(conditions []*ackv1alpha1.Condition) {
	t.Status.Conditions = conditions
}

func (t *Table) DeepCopy() acktypes.AWSResource {
	return t.DeepCopyTable()
}

func (t *Table) DeepCopyTable() *Table {
	return &Table{
		TypeMeta:   t.TypeMeta,
		ObjectMeta: *t.ObjectMeta.DeepCopy(),
		Spec:       t.Spec,
		Status:     t.Status,
	}
}

func (t *Table) DeepCopyObject() runtime.Object {
	return t.DeepCopyTable()
}

func (t *Table) SetIdentifiers(ids *ackv1alpha1.AWSIdentifiers) error {
	if t.Status.ACKResourceMetadata == nil {
		t.Status.ACKResourceMetadata = &ackv1alpha1.ResourceMetadata{}
	}
	t.Status.ACKResourceMetadata.ARN = ids.ARN
	return nil
}

func (t *Table) SetObjectMeta(meta metav1.ObjectMeta) {
	t.ObjectMeta = meta
}

func (t *Table) SetStatus(res acktypes.AWSResource) {
	if tr, ok := res.(*Table); ok {
		t.Status = tr.Status
	}
}

func (t *Table) PopulateResourceFromAnnotation(fields map[string]string) error {
	return nil
}

// TableFromRuntimeObject converts a client.Object to a Table AWSResource
func TableFromRuntimeObject(obj rtclient.Object) acktypes.AWSResource {
	if table, ok := obj.(*Table); ok {
		return table
	}
	return nil
}

// NewTableDescriptor creates a FakeResourceDescriptor configured for Table
func NewTableDescriptor() *FakeResourceDescriptor {
	gvk := TableGVK()
	return &FakeResourceDescriptor{
		gvk:             gvk,
		emptyRuntimeObj: &Table{},
		resourceFromObj: TableFromRuntimeObject,
		isManaged: func(res acktypes.AWSResource) bool {
			finalizers := res.MetaObject().GetFinalizers()
			for _, f := range finalizers {
				if f == FinalizerString {
					return true
				}
			}
			return false
		},
		markManaged: func(res acktypes.AWSResource) {
			finalizers := res.MetaObject().GetFinalizers()
			for _, f := range finalizers {
				if f == FinalizerString {
					return
				}
			}
			res.MetaObject().SetFinalizers(append(finalizers, FinalizerString))
		},
		markUnmanaged: func(res acktypes.AWSResource) {
			finalizers := res.MetaObject().GetFinalizers()
			newFinalizers := []string{}
			for _, f := range finalizers {
				if f != FinalizerString {
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
		delta: TableDelta,
	}
}

// TableGVK returns the GroupVersionKind for Table
func TableGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "dynamodb.services.k8s.aws",
		Version: "v1alpha1",
		Kind:    "Table",
	}
}

const FinalizerString = "finalizers.dynamodb.services.k8s.aws/Table"
