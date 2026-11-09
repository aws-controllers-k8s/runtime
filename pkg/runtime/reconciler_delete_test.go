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

// deletingResources returns the CR of a resource being deleted (current) and
// the resource ReadOne returns for it (observed), which shares the CR's
// metadata, as a generated ReadOne does. The state in AWS differs from the
// spec: the delta between current and observed has a spec difference.
func deletingResources(rd *ackmocks.AWSResourceDescriptor) (*ackmocks.AWSResource, *ackmocks.AWSResource) {
	current, metaObj := newMockRes()
	current.On("IsBeingDeleted").Return(true)
	observed := resourceSharingMetadata(metaObj)
	drift := ackcompare.NewDelta()
	drift.Add("Spec.Tags", "set by the user", "found in AWS")
	rd.On("Delta", current, observed).Return(drift)
	rd.On("Delta", observed, observed).Return(ackcompare.NewDelta())
	return current, observed
}

// resourceSharingMetadata returns a resource mock with the supplied metadata.
func resourceSharingMetadata(metaObj *k8sobj.Unstructured) *ackmocks.AWSResource {
	rtObj := &ctrlrtclientmock.Object{}
	rtObj.On("DeepCopyObject").Return(rtObj)
	res := &ackmocks.AWSResource{}
	res.On("MetaObject").Return(metaObj)
	res.On("RuntimeObject").Return(rtObj)
	res.On("DeepCopy").Return(res)
	res.On("SetStatus", mock.Anything).Return()
	return res
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
	current, observed := deletingResources(rd)

	rm := deleteResourceMocks(current)
	rm.On("ReadOne", mock.Anything, current).Return(observed, nil)
	deleteErr := errors.New("DeleteConflict: Cannot delete a policy attached to entities.")
	rm.On("Delete", mock.Anything, observed).Return(observed, deleteErr)

	rec, kc := newPreDeleteReconciler(rd)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.reconcile(context.Background(), rm, current)
	require.ErrorIs(t, err, deleteErr)
	kc.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything)
	rd.AssertNotCalled(t, "Delta", current, observed)
}

// An asynchronous Delete may set Spec fields or metadata on the resource it
// returns. Those changes are still saved, as a patch computed against the
// resource rm.Delete received.
func TestDeleteResource_SavesWhatDeleteChanged(t *testing.T) {
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{Group: "bookstore.services.k8s.aws", Kind: "Book"})
	rd.On("IsManaged", mock.Anything).Return(true)
	current, observed := deletingResources(rd)
	deleting := resourceSharingMetadata(current.MetaObject().(*k8sobj.Unstructured))
	changed := ackcompare.NewDelta()
	changed.Add("Spec.FinalSnapshotIdentifier", nil, "set by Delete")
	rd.On("Delta", observed, deleting).Return(changed)
	rd.On("Delta", current, deleting).Return(changed)

	rm := deleteResourceMocks(current)
	rm.On("ReadOne", mock.Anything, current).Return(observed, nil)
	rm.On("Delete", mock.Anything, observed).Return(deleting, requeue.NeededAfter(nil, time.Minute))

	rec, kc := newPreDeleteReconciler(rd)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := rec.reconcile(context.Background(), rm, current)
	var requeueNeededAfter *requeue.RequeueNeededAfter
	require.ErrorAs(t, err, &requeueNeededAfter)
	kc.AssertNumberOfCalls(t, "Patch", 1)
	rd.AssertCalled(t, "Delta", observed, deleting)
	rd.AssertNotCalled(t, "Delta", current, deleting)
}
