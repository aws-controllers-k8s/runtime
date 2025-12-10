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
	"reflect"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// TableDelta returns a Delta object containing the difference between
// one AWSResource and another. This matches the signature expected by
// FakeResourceDescriptor.delta
func TableDelta(a, b acktypes.AWSResource) *ackcompare.Delta {
	return newTableDelta(a.(*Table), b.(*Table))
}

// newTableDelta returns a Delta object containing the difference between
// one Table resource and another.
func newTableDelta(
	a *Table,
	b *Table,
) *ackcompare.Delta {
	delta := ackcompare.NewDelta()
	if (a == nil && b != nil) ||
		(a != nil && b == nil) {
		delta.Add("", a, b)
		return delta
	}

	// Spec.TableName (required string field)
	if a.Spec.TableName != b.Spec.TableName {
		delta.Add("Spec.TableName", a.Spec.TableName, b.Spec.TableName)
	}

	// Spec.BillingMode (nullable string pointer)
	if ackcompare.HasNilDifference(a.Spec.BillingMode, b.Spec.BillingMode) {
		delta.Add("Spec.BillingMode", a.Spec.BillingMode, b.Spec.BillingMode)
	} else if a.Spec.BillingMode != nil && b.Spec.BillingMode != nil {
		if *a.Spec.BillingMode != *b.Spec.BillingMode {
			delta.Add("Spec.BillingMode", a.Spec.BillingMode, b.Spec.BillingMode)
		}
	}

	// Spec.DeletionProtection (nullable bool pointer)
	if ackcompare.HasNilDifference(a.Spec.DeletionProtection, b.Spec.DeletionProtection) {
		delta.Add("Spec.DeletionProtection", a.Spec.DeletionProtection, b.Spec.DeletionProtection)
	} else if a.Spec.DeletionProtection != nil && b.Spec.DeletionProtection != nil {
		if *a.Spec.DeletionProtection != *b.Spec.DeletionProtection {
			delta.Add("Spec.DeletionProtection", a.Spec.DeletionProtection, b.Spec.DeletionProtection)
		}
	}

	// Spec.ProvisionedThroughput (nested object)
	if ackcompare.HasNilDifference(a.Spec.ProvisionedThroughput, b.Spec.ProvisionedThroughput) {
		delta.Add("Spec.ProvisionedThroughput", a.Spec.ProvisionedThroughput, b.Spec.ProvisionedThroughput)
	} else if a.Spec.ProvisionedThroughput != nil && b.Spec.ProvisionedThroughput != nil {
		if ackcompare.HasNilDifference(a.Spec.ProvisionedThroughput.ReadCapacityUnits, b.Spec.ProvisionedThroughput.ReadCapacityUnits) {
			delta.Add("Spec.ProvisionedThroughput.ReadCapacityUnits", a.Spec.ProvisionedThroughput.ReadCapacityUnits, b.Spec.ProvisionedThroughput.ReadCapacityUnits)
		} else if a.Spec.ProvisionedThroughput.ReadCapacityUnits != nil && b.Spec.ProvisionedThroughput.ReadCapacityUnits != nil {
			if *a.Spec.ProvisionedThroughput.ReadCapacityUnits != *b.Spec.ProvisionedThroughput.ReadCapacityUnits {
				delta.Add("Spec.ProvisionedThroughput.ReadCapacityUnits", a.Spec.ProvisionedThroughput.ReadCapacityUnits, b.Spec.ProvisionedThroughput.ReadCapacityUnits)
			}
		}
		if ackcompare.HasNilDifference(a.Spec.ProvisionedThroughput.WriteCapacityUnits, b.Spec.ProvisionedThroughput.WriteCapacityUnits) {
			delta.Add("Spec.ProvisionedThroughput.WriteCapacityUnits", a.Spec.ProvisionedThroughput.WriteCapacityUnits, b.Spec.ProvisionedThroughput.WriteCapacityUnits)
		} else if a.Spec.ProvisionedThroughput.WriteCapacityUnits != nil && b.Spec.ProvisionedThroughput.WriteCapacityUnits != nil {
			if *a.Spec.ProvisionedThroughput.WriteCapacityUnits != *b.Spec.ProvisionedThroughput.WriteCapacityUnits {
				delta.Add("Spec.ProvisionedThroughput.WriteCapacityUnits", a.Spec.ProvisionedThroughput.WriteCapacityUnits, b.Spec.ProvisionedThroughput.WriteCapacityUnits)
			}
		}
	}

	// Spec.SSESpecification (nested object)
	if ackcompare.HasNilDifference(a.Spec.SSESpecification, b.Spec.SSESpecification) {
		delta.Add("Spec.SSESpecification", a.Spec.SSESpecification, b.Spec.SSESpecification)
	} else if a.Spec.SSESpecification != nil && b.Spec.SSESpecification != nil {
		if ackcompare.HasNilDifference(a.Spec.SSESpecification.Enabled, b.Spec.SSESpecification.Enabled) {
			delta.Add("Spec.SSESpecification.Enabled", a.Spec.SSESpecification.Enabled, b.Spec.SSESpecification.Enabled)
		} else if a.Spec.SSESpecification.Enabled != nil && b.Spec.SSESpecification.Enabled != nil {
			if *a.Spec.SSESpecification.Enabled != *b.Spec.SSESpecification.Enabled {
				delta.Add("Spec.SSESpecification.Enabled", a.Spec.SSESpecification.Enabled, b.Spec.SSESpecification.Enabled)
			}
		}
		if ackcompare.HasNilDifference(a.Spec.SSESpecification.SSEType, b.Spec.SSESpecification.SSEType) {
			delta.Add("Spec.SSESpecification.SSEType", a.Spec.SSESpecification.SSEType, b.Spec.SSESpecification.SSEType)
		} else if a.Spec.SSESpecification.SSEType != nil && b.Spec.SSESpecification.SSEType != nil {
			if *a.Spec.SSESpecification.SSEType != *b.Spec.SSESpecification.SSEType {
				delta.Add("Spec.SSESpecification.SSEType", a.Spec.SSESpecification.SSEType, b.Spec.SSESpecification.SSEType)
			}
		}
	}

	// Spec.Tags (slice of objects)
	if len(a.Spec.Tags) != len(b.Spec.Tags) {
		delta.Add("Spec.Tags", a.Spec.Tags, b.Spec.Tags)
	} else if len(a.Spec.Tags) > 0 {
		if !reflect.DeepEqual(a.Spec.Tags, b.Spec.Tags) {
			delta.Add("Spec.Tags", a.Spec.Tags, b.Spec.Tags)
		}
	}

	return delta
}
