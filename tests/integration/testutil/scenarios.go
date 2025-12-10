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
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws/awserr"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// CreateBehavior defines how Create operations behave
type CreateBehavior func(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error)

// ReadBehavior defines how ReadOne operations behave
type ReadBehavior func(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error)

// UpdateBehavior defines how Update operations behave
type UpdateBehavior func(ctx context.Context, desired, latest acktypes.AWSResource, delta *ackcompare.Delta) (acktypes.AWSResource, error)

// DeleteBehavior defines how Delete operations behave
type DeleteBehavior func(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error)

// Response represents an AWS operation response
type Response struct {
	Resource acktypes.AWSResource
	Error    error
}

// ====================
// Response Builders
// ====================

// Success returns a successful response with the given resource
func Success(res acktypes.AWSResource) Response {
	return Response{Resource: res, Error: nil}
}

// Err returns an error response
func Err(err error) Response {
	return Response{Resource: nil, Error: err}
}

// NotFound returns a NotFound error response
func NotFound() Response {
	return Response{Resource: nil, Error: ackerr.NotFound}
}

// Throttled returns a throttling error response
func Throttled() Response {
	return Response{
		Resource: nil,
		Error: awserr.NewRequestFailure(
			awserr.New("Throttling", "Rate exceeded", nil),
			429,
			"throttle-req-id",
		),
	}
}

// AccessDenied returns a 403 AccessDenied error
func AccessDenied() Response {
	return Response{
		Resource: nil,
		Error: awserr.NewRequestFailure(
			awserr.New("AccessDenied", "Access denied", nil),
			403,
			"access-denied-req-id",
		),
	}
}

// ValidationError returns a validation error
func ValidationError(message string) Response {
	return Response{
		Resource: nil,
		Error: awserr.NewRequestFailure(
			awserr.New("ValidationException", message, nil),
			400,
			"validation-req-id",
		),
	}
}

// ====================
// Behavior Combinators
// ====================

// Always returns the same response forever
func Always(resp Response) func() Response {
	return func() Response {
		return resp
	}
}

// Times returns a response n times, then switches to next behavior
func Times(resp Response, n int, next func() Response) func() Response {
	count := 0
	mu := sync.Mutex{}
	return func() Response {
		mu.Lock()
		defer mu.Unlock()
		if count < n {
			count++
			return resp
		}
		return next()
	}
}

// Sequence returns responses in order, panics when exhausted
func Sequence(responses ...Response) func() Response {
	index := 0
	mu := sync.Mutex{}
	return func() Response {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(responses) {
			panic(fmt.Sprintf("sequence exhausted after %d calls", index))
		}
		r := responses[index]
		index++
		return r
	}
}

// Cycle returns responses in order, loops back to start when exhausted
func Cycle(responses ...Response) func() Response {
	index := 0
	mu := sync.Mutex{}
	return func() Response {
		mu.Lock()
		defer mu.Unlock()
		r := responses[index]
		index = (index + 1) % len(responses)
		return r
	}
}

// ====================
// Scenario Builders
// ====================

// CreateScenario builds a CreateBehavior from a response generator
func CreateScenario(gen func() Response) CreateBehavior {
	return func(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error) {
		r := gen()
		return r.Resource, r.Error
	}
}

// ReadScenario builds a ReadBehavior from a response generator
func ReadScenario(gen func() Response) ReadBehavior {
	return func(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error) {
		r := gen()
		return r.Resource, r.Error
	}
}

// UpdateScenario builds an UpdateBehavior from a response generator
func UpdateScenario(gen func() Response) UpdateBehavior {
	return func(ctx context.Context, desired, latest acktypes.AWSResource, delta *ackcompare.Delta) (acktypes.AWSResource, error) {
		r := gen()
		return r.Resource, r.Error
	}
}

// DeleteScenario builds a DeleteBehavior from a response generator
func DeleteScenario(gen func() Response) DeleteBehavior {
	return func(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error) {
		r := gen()
		return r.Resource, r.Error
	}
}
