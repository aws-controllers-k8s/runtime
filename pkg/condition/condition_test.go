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

package condition_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcond "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
)

func TestConditionGetters(t *testing.T) {
	assert := assert.New(t)

	conds := []*ackv1alpha1.Condition{}

	r := &ackmocks.AWSResource{}
	r.On("Conditions").Return(conds)

	got := ackcond.Synced(r)
	assert.Nil(got)

	got = ackcond.Terminal(r)
	assert.Nil(got)

	got = ackcond.ReferencesResolved(r)
	assert.Nil(got)

	conds = append(conds, &ackv1alpha1.Condition{
		Type:   ackv1alpha1.ConditionTypeResourceSynced,
		Status: corev1.ConditionFalse,
	})

	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return(conds)

	got = ackcond.Synced(r)
	assert.NotNil(got)

	got = ackcond.Terminal(r)
	assert.Nil(got)

	conds = append(conds, &ackv1alpha1.Condition{
		Type:   ackv1alpha1.ConditionTypeTerminal,
		Status: corev1.ConditionFalse,
	})

	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return(conds)

	got = ackcond.Synced(r)
	assert.NotNil(got)

	got = ackcond.Terminal(r)
	assert.NotNil(got)

	gotAll := ackcond.AllOfType(r, ackv1alpha1.ConditionTypeAdvisory)
	assert.Empty(gotAll)

	msg1 := "advice 1"
	conds = append(conds, &ackv1alpha1.Condition{
		Type:    ackv1alpha1.ConditionTypeAdvisory,
		Status:  corev1.ConditionTrue,
		Message: &msg1,
	})

	msg2 := "advice 2"
	conds = append(conds, &ackv1alpha1.Condition{
		Type:    ackv1alpha1.ConditionTypeAdvisory,
		Status:  corev1.ConditionTrue,
		Message: &msg2,
	})

	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return(conds)

	gotAll = ackcond.AllOfType(r, ackv1alpha1.ConditionTypeAdvisory)
	assert.NotEmpty(gotAll)
	assert.Equal(len(gotAll), 2)

	conds = append(conds, &ackv1alpha1.Condition{
		Type: ackv1alpha1.ConditionTypeReferencesResolved,
		Status: corev1.ConditionTrue,
	})
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return(conds)
	got = ackcond.ReferencesResolved(r)
	assert.NotNil(got)
}

func TestConditionSetters(t *testing.T) {
	r := &ackmocks.AWSResource{}
	r.On("Conditions").Return([]*ackv1alpha1.Condition{})

	// Ensure that if there is no synced condition, it gets added...
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			// We need to ignore timestamps for LastTransitionTime in our argument
			// assertions...
			return (subject[0].Type == ackv1alpha1.ConditionTypeResourceSynced &&
				subject[0].Status == corev1.ConditionTrue &&
				subject[0].Message == nil &&
				subject[0].Reason == nil)
		}),
	)

	ackcond.SetSynced(r, corev1.ConditionTrue, nil, nil)

	// Ensure that SetSynced doesn't overwrite any other conditions...
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return(
		[]*ackv1alpha1.Condition{
			&ackv1alpha1.Condition{
				Type:   ackv1alpha1.ConditionTypeTerminal,
				Status: corev1.ConditionTrue,
			},
		},
	)
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 2 {
				return false
			}
			return (subject[0].Type == ackv1alpha1.ConditionTypeTerminal &&
				subject[0].Status == corev1.ConditionTrue &&
				subject[1].Type == ackv1alpha1.ConditionTypeResourceSynced &&
				subject[1].Status == corev1.ConditionFalse)
		}),
	)

	ackcond.SetSynced(r, corev1.ConditionFalse, nil, nil)

	// Ensure that SetSynced overwrites an existing synced condition...
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return(
		[]*ackv1alpha1.Condition{
			&ackv1alpha1.Condition{
				Type:   ackv1alpha1.ConditionTypeResourceSynced,
				Status: corev1.ConditionFalse,
			},
		},
	)
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			return (subject[0].Type == ackv1alpha1.ConditionTypeResourceSynced &&
				subject[0].Status == corev1.ConditionTrue)
		}),
	)

	ackcond.SetSynced(r, corev1.ConditionTrue, nil, nil)

	msg1 := "message 1"
	reason1 := "reason 1"

	// Ensure that if there is no terminal condition, it gets added...
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return([]*ackv1alpha1.Condition{})
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			// We need to ignore timestamps for LastTransitionTime in our argument
			// assertions...
			return (subject[0].Type == ackv1alpha1.ConditionTypeTerminal &&
				subject[0].Status == corev1.ConditionTrue &&
				subject[0].Message == &msg1 &&
				subject[0].Reason == &reason1)
		}),
	)

	ackcond.SetTerminal(r, corev1.ConditionTrue, &msg1, &reason1)

	// ReferencesResolved condition
	// SetReferencesResolved
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return([]*ackv1alpha1.Condition{})
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			return (subject[0].Type == ackv1alpha1.ConditionTypeReferencesResolved &&
				subject[0].Status == corev1.ConditionTrue)
		}),
	)
	ackcond.SetReferencesResolved(r, corev1.ConditionTrue, nil, nil)

	//RemoveReferencesResolved
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return(
		[]*ackv1alpha1.Condition{
			&ackv1alpha1.Condition{
				Type:   ackv1alpha1.ConditionTypeResourceSynced,
				Status: corev1.ConditionTrue,
			},
			&ackv1alpha1.Condition{
				Type:   ackv1alpha1.ConditionTypeReferencesResolved,
				Status: corev1.ConditionTrue,
			},
		},
	)
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			return (subject[0].Type == ackv1alpha1.ConditionTypeResourceSynced &&
				subject[0].Status == corev1.ConditionTrue)
		}),
	)
	ackcond.RemoveReferencesResolved(r)

	//WithReferencesResolvedCondition
	// Without Error
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return([]*ackv1alpha1.Condition{})
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			return (subject[0].Type == ackv1alpha1.ConditionTypeReferencesResolved &&
				subject[0].Status == corev1.ConditionTrue)
		}),
	)
	ackcond.WithReferencesResolvedCondition(r, nil)
	// With Error
	errorMsg := "error message"
	err := errors.New(errorMsg)
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return([]*ackv1alpha1.Condition{})
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			return (subject[0].Type == ackv1alpha1.ConditionTypeReferencesResolved &&
				subject[0].Status == corev1.ConditionUnknown &&
				*subject[0].Message == errorMsg)
		}),
	)
	ackcond.WithReferencesResolvedCondition(r, err)
	// With Terminal Error
	terminalError := ackerr.ResourceReferenceTerminal
	r = &ackmocks.AWSResource{}
	r.On("Conditions").Return([]*ackv1alpha1.Condition{})
	r.On(
		"ReplaceConditions",
		mock.MatchedBy(func(subject []*ackv1alpha1.Condition) bool {
			if len(subject) != 1 {
				return false
			}
			return (subject[0].Type == ackv1alpha1.ConditionTypeReferencesResolved &&
				subject[0].Status == corev1.ConditionFalse &&
				*subject[0].Message == terminalError.Error())
		}),
	)
	ackcond.WithReferencesResolvedCondition(r, terminalError)
}
