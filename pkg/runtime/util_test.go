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

package runtime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"

	mocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
)

func TestIsAdopted(t *testing.T) {
	require := require.New(t)

	res := &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdopted: "true",
		},
	})
	require.True(IsAdopted(res))

	res = &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{})
	require.False(IsAdopted(res))
}

func TestIsSynced(t *testing.T) {
	require := require.New(t)

	res := &mocks.AWSResource{}
	res.On("Conditions").Return([]*ackv1alpha1.Condition{
		&ackv1alpha1.Condition{
			Type:   ackv1alpha1.ConditionTypeResourceSynced,
			Status: corev1.ConditionTrue,
		},
	})
	require.True(IsSynced(res))

	res = &mocks.AWSResource{}
	res.On("Conditions").Return([]*ackv1alpha1.Condition{
		&ackv1alpha1.Condition{
			Type:   ackv1alpha1.ConditionTypeResourceSynced,
			Status: corev1.ConditionUnknown,
		},
		&ackv1alpha1.Condition{
			Type:   ackv1alpha1.ConditionTypeResourceSynced,
			Status: corev1.ConditionFalse,
		},
	})
	require.False(IsSynced(res))
}

func TestIsForcedAdoption(t *testing.T) {
	require := require.New(t)

	res := &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdoptionPolicy: "adopt",
			ackv1alpha1.AnnotationAdopted:        "false",
		},
	})
	require.True(NeedAdoption(res))

	res = &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdoptionPolicy: "adopt",
			ackv1alpha1.AnnotationAdopted:        "true",
		},
	})
	require.False(NeedAdoption(res))

	res = &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdopted: "true",
		},
	})
	require.False(NeedAdoption(res))
}

func TestExtractAdoptionFields(t *testing.T) {
	require := require.New(t)

	res := &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdoptionFields: `{
				"clusterName": "my-cluster",
				"name": "ng-1234"
			}`,
		},
	})

	expected := map[string]string{
		"clusterName": "my-cluster",
		"name":        "ng-1234",
	}
	actual, err := ExtractAdoptionFields(res)
	require.NoError(err)
	require.Equal(expected, actual)
}

func TestExtractAdoptionFields_NoAdoptionFields(t *testing.T) {
	require := require.New(t)

	res := &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{},
	})

	_, err := ExtractAdoptionFields(res)
	require.Error(err)
	require.Equal(err, fmt.Errorf("%s annotation is not defined. Cannot extract resource identifiers", ackv1alpha1.AnnotationAdoptionFields))
}

func TestExtractAdoptionFields_InvalidAdoptionFields(t *testing.T) {
	require := require.New(t)

	res := &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdoptionFields: "misconfigured",
		},
	})

	_, err := ExtractAdoptionFields(res)
	require.Error(err)
	expectedErr := fmt.Sprintf("error parsing content of %s annotation", ackv1alpha1.AnnotationAdoptionFields)
	require.True(strings.Contains(err.Error(), expectedErr))
}
