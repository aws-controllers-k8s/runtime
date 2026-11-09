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
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

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

func TestExtractAdoptionFields_EmptyFields(t *testing.T) {
	require := require.New(t)

	// No adoption-fields annotation at all
	res := &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{},
	})

	_, err := ExtractAdoptionFields(res)
	require.Error(err)
	require.True(strings.Contains(err.Error(), "services.k8s.aws/adoption-fields annotation is required"))
	require.True(strings.Contains(err.Error(), "adoption-policy is set to 'adopt'"))
}

func TestExtractAdoptionFields_InvalidJSON(t *testing.T) {
	require := require.New(t)

	res := &mocks.AWSResource{}
	res.On("MetaObject").Return(&metav1.ObjectMeta{
		Annotations: map[string]string{
			ackv1alpha1.AnnotationAdoptionFields: `not valid json`,
		},
	})

	_, err := ExtractAdoptionFields(res)
	require.Error(err)
	require.True(strings.Contains(err.Error(), "failed to parse services.k8s.aws/adoption-fields annotation as JSON"))
}

func TestWithShutdownGrace_CanceledOnceGracePassed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, shutdown := context.WithCancelCause(context.Background())
		ctx, cancel := withShutdownGrace(parent, 5*time.Second)
		defer cancel()

		// The grace period starts at the shutdown, not when the context is
		// created.
		time.Sleep(time.Minute)
		errShutdown := errors.New("shutting down")
		shutdown(errShutdown)
		synctest.Wait()
		require.NoError(t, ctx.Err(), "the context outlives its parent")

		time.Sleep(5*time.Second - time.Nanosecond)
		synctest.Wait()
		require.NoError(t, ctx.Err())

		time.Sleep(time.Nanosecond)
		synctest.Wait()
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		require.Equal(t, errShutdown, context.Cause(ctx))
	})
}

func TestWithShutdownGrace_KeepsValuesAndDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type key struct{}
		parent := context.WithValue(context.Background(), key{}, "value")
		parent, cancelParent := context.WithTimeout(parent, time.Minute)
		defer cancelParent()
		ctx, cancel := withShutdownGrace(parent, time.Hour)
		defer cancel()

		require.Equal(t, "value", ctx.Value(key{}))
		parentDeadline, _ := parent.Deadline()
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Equal(t, parentDeadline, deadline)

		// A reconciliation timeout is not a shutdown: it does not wait for
		// the grace period.
		time.Sleep(time.Minute)
		synctest.Wait()
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	})
}

func TestWithShutdownGrace_CancelFunc(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, shutdown := context.WithCancel(context.Background())
		defer shutdown()

		// Released before the shutdown.
		ctx, cancel := withShutdownGrace(parent, 5*time.Second)
		cancel()
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		require.NoError(t, parent.Err())

		// Released during the grace period, without waiting for it to pass.
		ctx, cancel = withShutdownGrace(parent, 5*time.Second)
		shutdown()
		synctest.Wait()
		require.NoError(t, ctx.Err())
		cancel()
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		require.Equal(t, context.Canceled, context.Cause(ctx))
	})
}
