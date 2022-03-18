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

package condition

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

var (
	NotManagedMessage = "Resource already exists"
	NotManagedReason  = "This resource already exists but is not managed by ACK. " +
		"To bring the resource under ACK management, you should explicitly adopt " +
		"the resource by creating a services.k8s.aws/AdoptedResource"
	NotSyncedMessage = "Resource not synced"
	SyncedMessage    = "Resource synced successfully"
)

// Synced returns the Condition in the resource's Conditions collection that is
// of type ConditionTypeResourceSynced. If no such condition is found, returns
// nil.
func Synced(subject acktypes.ConditionManager) *ackv1alpha1.Condition {
	return FirstOfType(subject, ackv1alpha1.ConditionTypeResourceSynced)
}

// Terminal returns the Condition in the resource's Conditions collection that
// is of type ConditionTypeTerminal. If no such condition is found, returns
// nil.
func Terminal(subject acktypes.ConditionManager) *ackv1alpha1.Condition {
	return FirstOfType(subject, ackv1alpha1.ConditionTypeTerminal)
}

// Recoverable returns the Condition in the resource's Conditions collection
// that is of type ConditionTypeRecoverable. If no such condition is found,
// returns nil.
func Recoverable(subject acktypes.ConditionManager) *ackv1alpha1.Condition {
	return FirstOfType(subject, ackv1alpha1.ConditionTypeRecoverable)
}

// LateInitialized returns the Condition in the resource's Conditions collection that
// is of type ConditionTypeLateInitialized. If no such condition is found, returns
// nil.
func LateInitialized(subject acktypes.ConditionManager) *ackv1alpha1.Condition {
	return FirstOfType(subject, ackv1alpha1.ConditionTypeLateInitialized)
}

// ReferencesResolved returns the Condition in the resource's Conditions collection
// that is of type ConditionTypeReferencesResolved. If no such condition is found,
// returns nil.
func ReferencesResolved(subject acktypes.ConditionManager) *ackv1alpha1.Condition {
	return FirstOfType(subject, ackv1alpha1.ConditionTypeReferencesResolved)
}

// FirstOfType returns the first Condition in the resource's Conditions
// collection of the supplied type. If no such condition is found, returns nil.
func FirstOfType(
	subject acktypes.ConditionManager,
	condType ackv1alpha1.ConditionType,
) *ackv1alpha1.Condition {
	for _, condition := range subject.Conditions() {
		if condition.Type == condType {
			return condition
		}
	}
	return nil
}

// AllOfType returns a slice of Conditions in the resource's Conditions
// collection of the supplied type.
func AllOfType(
	subject acktypes.ConditionManager,
	condType ackv1alpha1.ConditionType,
) []*ackv1alpha1.Condition {
	res := []*ackv1alpha1.Condition{}
	for _, condition := range subject.Conditions() {
		if condition.Type == condType {
			res = append(res, condition)
		}
	}
	return res
}

// SetSynced sets the resource's Condition of type ConditionTypeResourceSynced
// to the supplied status, optional message and reason.
func SetSynced(
	subject acktypes.ConditionManager,
	status corev1.ConditionStatus,
	message *string,
	reason *string,
) {
	allConds := subject.Conditions()
	var c *ackv1alpha1.Condition
	if c = Synced(subject); c == nil {
		c = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeResourceSynced,
		}
		allConds = append(allConds, c)
	}
	now := metav1.Now()
	c.LastTransitionTime = &now
	c.Status = status
	c.Message = message
	c.Reason = reason
	subject.ReplaceConditions(allConds)
}

// SetTerminal sets the resource's Condition of type ConditionTypeTerminal to
// the supplied status, optional message and reason.
func SetTerminal(
	subject acktypes.ConditionManager,
	status corev1.ConditionStatus,
	message *string,
	reason *string,
) {
	allConds := subject.Conditions()
	var c *ackv1alpha1.Condition
	if c = Terminal(subject); c == nil {
		c = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeTerminal,
		}
		allConds = append(allConds, c)
	}
	now := metav1.Now()
	c.LastTransitionTime = &now
	c.Status = status
	c.Message = message
	c.Reason = reason
	subject.ReplaceConditions(allConds)
}

// SetRecoverable sets the resource's Condition of type ConditionTypeRecoverable
// to the supplied status, optional message and reason.
func SetRecoverable(
	subject acktypes.ConditionManager,
	status corev1.ConditionStatus,
	message *string,
	reason *string,
) {
	allConds := subject.Conditions()
	var c *ackv1alpha1.Condition
	if c = Recoverable(subject); c == nil {
		c = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeRecoverable,
		}
		allConds = append(allConds, c)
	}
	now := metav1.Now()
	c.LastTransitionTime = &now
	c.Status = status
	c.Message = message
	c.Reason = reason
	subject.ReplaceConditions(allConds)
}

// SetLateInitialized sets the resource's Condition of type ConditionTypeLateInitialized to
// the supplied status, optional message and reason.
func SetLateInitialized(
	subject acktypes.ConditionManager,
	status corev1.ConditionStatus,
	message *string,
	reason *string,
) {
	allConds := subject.Conditions()
	var c *ackv1alpha1.Condition
	if c = LateInitialized(subject); c == nil {
		c = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeLateInitialized,
		}
		allConds = append(allConds, c)
	}
	now := metav1.Now()
	c.LastTransitionTime = &now
	c.Status = status
	c.Message = message
	c.Reason = reason
	subject.ReplaceConditions(allConds)
}

// SetReferencesResolved sets the resource's Condition of type ConditionTypeReferencesResolved
// to the supplied status, optional message and reason.
func SetReferencesResolved(
	subject acktypes.ConditionManager,
	status corev1.ConditionStatus,
	message *string,
	reason *string,
) {
	allConds := subject.Conditions()
	var c *ackv1alpha1.Condition
	if c = ReferencesResolved(subject); c == nil {
		c = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeReferencesResolved,
		}
		allConds = append(allConds, c)
	}
	now := metav1.Now()
	c.LastTransitionTime = &now
	c.Status = status
	c.Message = message
	c.Reason = reason
	subject.ReplaceConditions(allConds)
}

// RemoveReferencesResolved removes the condition of type ConditionTypeReferencesResolved
// from the resource's conditions
func RemoveReferencesResolved(
	subject acktypes.ConditionManager,
) {
	allConds := subject.Conditions()
	var newConds []*ackv1alpha1.Condition
	if c := ReferencesResolved(subject); c != nil {
		for _, cond := range allConds {
			if cond.Type != ackv1alpha1.ConditionTypeReferencesResolved {
				newConds = append(newConds, cond)
			}
		}
		subject.ReplaceConditions(newConds)
	}
}

// WithReferencesResolvedCondition sets the ConditionTypeReferencesResolved in
// AWSResource based on the err parameter and returns (AWSResource,error)
func WithReferencesResolvedCondition(
	resource acktypes.AWSResource,
	err error,
) (acktypes.AWSResource, error) {
	if err != nil {
		errString := err.Error()
		conditionStatus := corev1.ConditionUnknown
		if strings.Contains(errString, ackerr.ResourceReferenceTerminal.Error()) {
			conditionStatus = corev1.ConditionFalse
		}
		SetReferencesResolved(resource, conditionStatus, &errString, nil)
	} else {
		SetReferencesResolved(resource, corev1.ConditionTrue, nil, nil)
	}
	return resource, err
}

// LateInitializationInProgress return true if ConditionTypeLateInitialized has "False" status
// False status means that resource has LateInitializationConfig but has not been completely
// late initialized yet.
func LateInitializationInProgress(subject acktypes.ConditionManager) bool {
	c := LateInitialized(subject)
	return c != nil && c.Status == corev1.ConditionFalse
}

// Clear resets the resource's collection of Conditions to an empty list.
func Clear(
	subject acktypes.ConditionManager,
) {
	subject.ReplaceConditions([]*ackv1alpha1.Condition{})
}
