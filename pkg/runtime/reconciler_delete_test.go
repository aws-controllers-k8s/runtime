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
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	k8sobj "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"

	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// deletingResources returns the CR of a resource being deleted (current), the
// resource ReadOne returns for it (observed), which shares the CR's metadata,
// as a generated ReadOne does, and the copy of observed that deleteResource
// takes before calling Delete (beforeDelete). The state in AWS differs from the
// spec: the delta between current and observed has a spec difference.
func deletingResources(rd *ackmocks.AWSResourceDescriptor) (current, observed, beforeDelete *ackmocks.AWSResource) {
	current, metaObj := newMockRes()
	current.On("IsBeingDeleted").Return(true)
	beforeDelete = resourceSharingMetadata(metaObj, nil)
	observed = resourceSharingMetadata(metaObj, beforeDelete)
	drift := ackcompare.NewDelta()
	drift.Add("Spec.Tags", "set by the user", "found in AWS")
	rd.On("Delta", same(current), same(observed)).Return(drift)
	rd.On("Delta", same(observed), same(observed)).Return(ackcompare.NewDelta())
	return current, observed, beforeDelete
}

// resourceSharingMetadata returns a resource mock with the supplied metadata.
// Its DeepCopy returns copied, or the mock itself when copied is nil.
func resourceSharingMetadata(metaObj *k8sobj.Unstructured, copied *ackmocks.AWSResource) *ackmocks.AWSResource {
	rtObj := &ctrlrtclientmock.Object{}
	rtObj.On("DeepCopyObject").Return(rtObj)
	res := &ackmocks.AWSResource{}
	if copied == nil {
		copied = res
	}
	res.On("MetaObject").Return(metaObj)
	res.On("RuntimeObject").Return(rtObj)
	res.On("DeepCopy").Return(copied)
	res.On("SetStatus", mock.Anything).Return()
	return res
}

// same matches the supplied resource itself, not a resource equal to it: the
// mocks above differ only by identity.
func same(res acktypes.AWSResource) interface{} {
	return mock.MatchedBy(func(r acktypes.AWSResource) bool { return r == res })
}

func deleteResourceMocks(current acktypes.AWSResource) *ackmocks.AWSResourceManager {
	rm := &ackmocks.AWSResourceManager{}
	rm.On("ResolveReferences", mock.Anything, mock.Anything, current).Return(current, false, nil)
	rm.On("ClearResolvedReferences", mock.Anything).Return(
		func(r acktypes.AWSResource) acktypes.AWSResource { return r },
	)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything)
	return rm
}

// A Delete that fails returns the resource it was given: the state read from
// AWS. Saving the Spec and metadata changes of rm.Delete must not write that
// state over the CR's spec.
func TestDeleteResource_FailedDeleteKeepsSpec(t *testing.T) {
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{Group: "bookstore.services.k8s.aws", Kind: "Book"})
	rd.On("IsManaged", mock.Anything).Return(true)
	current, observed, beforeDelete := deletingResources(rd)
	rd.On("Delta", same(beforeDelete), same(observed)).Return(ackcompare.NewDelta())

	rm := deleteResourceMocks(current)
	rm.On("ReadOne", mock.Anything, current).Return(observed, nil)
	deleteErr := errors.New("DeleteConflict: Cannot delete a policy attached to entities.")
	rm.On("Delete", mock.Anything, same(observed)).Return(observed, deleteErr)

	rec, kc := newPreDeleteReconciler(rd)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.reconcile(context.Background(), rm, current)
	require.ErrorIs(t, err, deleteErr)
	kc.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything)
	rd.AssertNotCalled(t, "Delta", same(current), same(observed))
}

// A generated Delete does not copy the resource it receives, and delete hooks
// change it in place: CloudFront's Distribution sets Enabled to false on it,
// disables the distribution and returns the same resource with a requeue. That
// change is saved, as a patch computed against the copy taken before Delete.
func TestDeleteResource_SavesWhatDeleteChangedInPlace(t *testing.T) {
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{Group: "bookstore.services.k8s.aws", Kind: "Book"})
	rd.On("IsManaged", mock.Anything).Return(true)
	current, observed, beforeDelete := deletingResources(rd)
	changed := ackcompare.NewDelta()
	changed.Add("Spec.DistributionConfig.Enabled", true, false)
	rd.On("Delta", same(beforeDelete), same(observed)).Return(changed)

	rm := deleteResourceMocks(current)
	rm.On("ReadOne", mock.Anything, current).Return(observed, nil)
	rm.On("Delete", mock.Anything, same(observed)).Return(observed, requeue.NeededAfter(nil, time.Minute))

	rec, kc := newPreDeleteReconciler(rd)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.reconcile(context.Background(), rm, current)
	var requeueNeededAfter *requeue.RequeueNeededAfter
	require.ErrorAs(t, err, &requeueNeededAfter)
	kc.AssertNumberOfCalls(t, "Patch", 1)
	rd.AssertCalled(t, "Delta", same(beforeDelete), same(observed))
	rd.AssertNotCalled(t, "Delta", same(current), same(observed))
}
