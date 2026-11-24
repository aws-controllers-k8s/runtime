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
	"sort"
	"strings"
	"sync"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/smithy-go/middleware"
)

// apiCallRecorderMiddlewareID names the SDK middleware that records write calls.
const apiCallRecorderMiddlewareID = "ACKAPICallRecorder"

// readOperationPrefixes are the operation name prefixes treated as reads.
var readOperationPrefixes = []string{"Describe", "Get", "List", "Head", "BatchGet"}

// apiCallRecorder collects the AWS write calls that succeed during one Update.
type apiCallRecorder struct {
	mu    sync.Mutex
	calls map[string]struct{}
}

type apiCallRecorderKey struct{}

// withAPICallRecorder returns a context carrying a new, empty recorder.
func withAPICallRecorder(ctx context.Context) (context.Context, *apiCallRecorder) {
	rec := &apiCallRecorder{calls: map[string]struct{}{}}
	return context.WithValue(ctx, apiCallRecorderKey{}, rec), rec
}

func apiCallRecorderFrom(ctx context.Context) *apiCallRecorder {
	rec, _ := ctx.Value(apiCallRecorderKey{}).(*apiCallRecorder)
	return rec
}

func (r *apiCallRecorder) add(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[call] = struct{}{}
}

// Calls returns the recorded calls, sorted and de-duplicated.
func (r *apiCallRecorder) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.calls))
	for c := range r.calls {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func isReadOperation(op string) bool {
	for _, p := range readOperationPrefixes {
		if strings.HasPrefix(op, p) {
			return true
		}
	}
	return false
}

// addAPICallRecorder is an aws.Config APIOptions entry. It records
// "ServiceID.Operation" in the context's recorder when a write call returns
// without error. It sits at the end of the Initialize step, so it sees the
// final result after retries, and does nothing when the context has no
// recorder.
func addAPICallRecorder(stack *middleware.Stack) error {
	return stack.Initialize.Add(middleware.InitializeMiddlewareFunc(
		apiCallRecorderMiddlewareID,
		func(
			ctx context.Context,
			in middleware.InitializeInput,
			next middleware.InitializeHandler,
		) (middleware.InitializeOutput, middleware.Metadata, error) {
			out, md, err := next.HandleInitialize(ctx, in)
			if err != nil {
				return out, md, err
			}
			rec := apiCallRecorderFrom(ctx)
			if rec == nil {
				return out, md, err
			}
			op := awsmiddleware.GetOperationName(ctx)
			if op == "" || isReadOperation(op) {
				return out, md, err
			}
			rec.add(awsmiddleware.GetServiceID(ctx) + "." + op)
			return out, md, err
		},
	), middleware.After)
}
