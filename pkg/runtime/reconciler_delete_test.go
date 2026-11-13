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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// These tests drive deleteResource with `uResource`, whose DeepCopy returns a
// real copy, so a resource changed in place and a copy of it are told apart as
// they are in a generated controller. The state in AWS differs from the CR's
// spec in `tags`, and the merge patch bodies sent to the API server are
// checked: the state read from AWS must never be written over the user's spec.

// deletingSpecs returns the spec of the CR (current) and the spec ReadOne
// reads from AWS (observed).
func deletingSpecs() (current, observed map[string]interface{}) {
	current = map[string]interface{}{"enabled": true, "tags": "set by the user"}
	observed = map[string]interface{}{"enabled": true, "tags": "found in AWS"}
	return current, observed
}

// capturePatchBodies records the merge patch body of every metadata and spec
// patch sent through kc.
func capturePatchBodies(kc *ctrlrtclientmock.Client) *[]map[string]interface{} {
	var bodies []map[string]interface{}
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		obj, _ := args.Get(1).(client.Object)
		patch, _ := args.Get(2).(client.Patch)
		if obj == nil || patch == nil {
			return
		}
		data, err := patch.Data(obj)
		if err != nil {
			return
		}
		var body map[string]interface{}
		if json.Unmarshal(data, &body) == nil {
			bodies = append(bodies, body)
		}
	}).Return(nil)
	return &bodies
}

// A Delete that fails returns the resource it was given: the state read from
// AWS. Saving the Spec and metadata changes of rm.Delete must not write that
// state over the CR's spec.
func TestDeleteResource_FailedDeleteKeepsSpec(t *testing.T) {
	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, false)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)
	currentSpec, observedSpec := deletingSpecs()
	current := newDriftRes(false, currentSpec)
	observed := newDriftRes(false, observedSpec)

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(observed, nil)
	deleteErr := errors.New("DeleteConflict: Cannot delete a policy attached to entities.")
	rm.On("Delete", mock.Anything, mock.Anything).Return(
		func(_ context.Context, r acktypes.AWSResource) acktypes.AWSResource { return r },
		deleteErr,
	)
	bodies := capturePatchBodies(kc)

	_, err := rec.deleteResource(context.TODO(), rm, current)
	require.ErrorIs(t, err, deleteErr)
	assert.Empty(t, *bodies, "a failed Delete must not patch the CR's metadata or spec")
}

// A generated Delete does not copy the resource it receives, and delete hooks
// change it in place: CloudFront's Distribution sets Enabled to false on it,
// disables the distribution and returns the same resource with a requeue. That
// change is written to the CR, and only that change.
func TestDeleteResource_SavesWhatDeleteChangedInPlace(t *testing.T) {
	rec, rm, rd, kc, scmd := ignoreDriftReconcilerMocks(t, false)
	wireDescriptorCommon(rd)
	wireManagerCommon(rm, scmd)
	currentSpec, observedSpec := deletingSpecs()
	current := newDriftRes(false, currentSpec)
	observed := newDriftRes(false, observedSpec)

	rm.On("ReadOne", mock.Anything, mock.Anything).Return(observed, nil)
	rm.On("Delete", mock.Anything, mock.Anything).Return(
		func(_ context.Context, r acktypes.AWSResource) acktypes.AWSResource {
			_ = unstructured.SetNestedField(r.(*uResource).obj.Object, false, "spec", "enabled")
			return r
		},
		requeue.NeededAfter(nil, time.Minute),
	)
	bodies := capturePatchBodies(kc)

	_, err := rec.deleteResource(context.TODO(), rm, current)
	var requeueNeededAfter *requeue.RequeueNeededAfter
	require.ErrorAs(t, err, &requeueNeededAfter)
	require.Len(t, *bodies, 1)
	assert.Equal(t,
		map[string]interface{}{"spec": map[string]interface{}{"enabled": false}},
		(*bodies)[0],
		"the patch must carry what Delete changed and nothing read from AWS",
	)
}
