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

package errors_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
)

// mockAWSError is a minimal smithy.APIError, as the AWS SDK surfaces service
// errors.
type mockAWSError struct {
	code string
}

func (e *mockAWSError) Error() string {
	return "api error " + e.code
}
func (e *mockAWSError) ErrorCode() string             { return e.code }
func (e *mockAWSError) ErrorMessage() string          { return e.Error() }
func (e *mockAWSError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

var _ smithy.APIError = &mockAWSError{}

func TestWrapPostCreateError_WrapsAWSError(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	awsErr := &mockAWSError{code: "UnauthorizedOperation"}

	wrapped := ackerr.WrapPostCreateError(awsErr)

	require.NotNil(wrapped)
	assert.True(ackerr.IsPostCreateError(wrapped),
		"an AWS API error must be marked as a post-create failure")

	// The original must stay reachable so terminal-code classification and HTTP
	// status introspection keep working through the wrapper.
	var postCreateErr *ackerr.PostCreateError
	require.True(errors.As(wrapped, &postCreateErr))
	assert.Equal(awsErr, postCreateErr.Unwrap())

	gotAWSErr, ok := ackerr.AWSError(wrapped)
	require.True(ok, "the wrapped AWS error must still be detected by AWSError")
	assert.Equal("UnauthorizedOperation", gotAWSErr.ErrorCode())

	assert.Contains(wrapped.Error(), "UnauthorizedOperation")
}

func TestWrapPostCreateError_Nil(t *testing.T) {
	assert.Nil(t, ackerr.WrapPostCreateError(nil))
	assert.False(t, ackerr.IsPostCreateError(nil))
}

// TestWrapPostCreateError_PassesThroughNonAWSErrors is the guarantee that lets
// the wrap be applied unconditionally to every error returned after a successful
// create: errors that would never have unmanaged a resource are returned
// untouched, so identity comparisons on them keep working.
func TestWrapPostCreateError_PassesThroughNonAWSErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ackerr.NotFound", ackerr.NotFound},
		{"ackerr.Terminal", ackerr.Terminal},
		{"ackerr.SecretNotFound", ackerr.SecretNotFound},
		{"ackerr.SecretTypeNotSupported", ackerr.SecretTypeNotSupported},
		{"requeue.Needed", ackrequeue.Needed(fmt.Errorf("resource created, requeuing"))},
		{"requeue.NeededAfter", ackrequeue.NeededAfter(
			fmt.Errorf("requeuing for post-create updates"), time.Second)},
		{"bare RequeueNeeded", &ackrequeue.RequeueNeeded{}},
		{"plain error", fmt.Errorf("something went wrong")},
		{"TerminalError", ackerr.NewTerminalError(fmt.Errorf("bad input"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ackerr.WrapPostCreateError(tt.err)
			assert.Same(t, tt.err, got,
				"a non-AWS error must be returned unchanged")
			assert.False(t, ackerr.IsPostCreateError(got))
		})
	}
}

// TestWrapPostCreateError_SentinelIdentityPreserved is the concrete reason the
// wrap is gated: the reconciler and generated controller code compare these
// sentinels by identity, not with errors.Is.
func TestWrapPostCreateError_SentinelIdentityPreserved(t *testing.T) {
	assert := assert.New(t)

	assert.True(ackerr.WrapPostCreateError(ackerr.NotFound) == ackerr.NotFound)
	assert.True(ackerr.WrapPostCreateError(ackerr.Terminal) == ackerr.Terminal)
}

// TestWrapPostCreateError_RequeueSurvivesWrappedAWSError covers a requeue that
// carries an AWS error. The wrap does fire here, so requeue detection has to
// keep working through it.
func TestWrapPostCreateError_RequeueSurvivesWrappedAWSError(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	awsErr := &mockAWSError{code: "ThrottlingException"}
	requeueErr := ackrequeue.NeededAfter(awsErr, 5*time.Second)

	wrapped := ackerr.WrapPostCreateError(requeueErr)

	require.True(ackerr.IsPostCreateError(wrapped))

	// This is how the reconciler detects requeues in HandleReconcileError.
	var requeueNeededAfter *ackrequeue.RequeueNeededAfter
	require.True(errors.As(wrapped, &requeueNeededAfter),
		"requeue detection must see through the post-create wrapper")
	assert.Equal(5*time.Second, requeueNeededAfter.Duration())
}

func TestWrapPostCreateError_DoubleWrapIsDetectedOnce(t *testing.T) {
	awsErr := &mockAWSError{code: "UnauthorizedOperation"}

	wrapped := ackerr.WrapPostCreateError(ackerr.WrapPostCreateError(awsErr))

	assert.True(t, ackerr.IsPostCreateError(wrapped))
	gotAWSErr, ok := ackerr.AWSError(wrapped)
	require.True(t, ok)
	assert.Equal(t, "UnauthorizedOperation", gotAWSErr.ErrorCode())
}
