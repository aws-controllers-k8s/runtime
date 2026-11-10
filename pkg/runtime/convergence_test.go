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
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"

	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
)

func twoDiffDelta(a, b string) *ackcompare.Delta {
	d := ackcompare.NewDelta()
	d.Add("Spec.ContentBasedDeduplication", nil, aws.String(a))
	d.Add("Spec.FIFOThroughputLimit", nil, aws.String(b))
	return d
}

func TestDeltaHash(t *testing.T) {
	require := require.New(t)
	h := deltaHash(twoDiffDelta("false", "perQueue"))
	require.NotEmpty(h)
	require.Equal(h, deltaHash(twoDiffDelta("false", "perQueue")), "stable")

	reordered := ackcompare.NewDelta()
	reordered.Add("Spec.FIFOThroughputLimit", nil, aws.String("perQueue"))
	reordered.Add("Spec.ContentBasedDeduplication", nil, aws.String("false"))
	require.Equal(h, deltaHash(reordered), "independent of difference order")

	require.NotEqual(h, deltaHash(twoDiffDelta("true", "perQueue")), "changes with a value")
	require.Empty(deltaHash(ackcompare.NewDelta()))

	bad := ackcompare.NewDelta()
	bad.Add("Spec.X", make(chan int), nil)
	require.Empty(deltaHash(bad), "unserializable value gives no hash")
}

func TestNextConvergenceRecord(t *testing.T) {
	require := require.New(t)
	calls := []string{"SQS.SetQueueAttributes"}

	first := nextConvergenceRecord(nil, "h1", calls)
	require.Equal(1, first.SuccessfulUpdates)

	second := nextConvergenceRecord(first, "h1", calls)
	require.Equal(2, second.SuccessfulUpdates)
	require.Equal(2, nextConvergenceRecord(second, "h1", calls).SuccessfulUpdates, "capped")

	require.Equal(1, nextConvergenceRecord(second, "h2", calls).SuccessfulUpdates, "new delta restarts")
	require.Equal(1, nextConvergenceRecord(second, "h1",
		[]string{"SQS.TagQueue"}).SuccessfulUpdates, "new calls restart")
}

func TestShouldSkipUpdate(t *testing.T) {
	require := require.New(t)
	rec := &convergenceRecord{DeltaHash: "h1", SuccessfulUpdates: 2}
	require.True(shouldSkipUpdate(rec, "h1"))
	require.False(shouldSkipUpdate(rec, "h2"))
	require.False(shouldSkipUpdate(rec, ""))
	require.False(shouldSkipUpdate(nil, "h1"))
	require.False(shouldSkipUpdate(&convergenceRecord{DeltaHash: "h1", SuccessfulUpdates: 1}, "h1"))
}

func TestConvergenceRecordAnnotation(t *testing.T) {
	require := require.New(t)
	res := newUResource(map[string]string{"keep": "me"}, nil)
	require.Nil(convergenceRecordFrom(res))

	rec := &convergenceRecord{DeltaHash: "h1", APICalls: []string{"A.B"}, SuccessfulUpdates: 1}
	require.NoError(setConvergenceRecord(res, rec))
	require.Equal(rec, convergenceRecordFrom(res))
	require.Equal("me", res.MetaObject().GetAnnotations()["keep"])

	require.NoError(setConvergenceRecord(res, nil))
	require.Nil(convergenceRecordFrom(res))
	require.Equal("me", res.MetaObject().GetAnnotations()["keep"])

	bad := newUResource(map[string]string{ackv1alpha1.AnnotationConvergence: "{not json"}, nil)
	require.Nil(convergenceRecordFrom(bad), "malformed annotation counts as no record")
}

func TestIsRequeueError(t *testing.T) {
	require := require.New(t)
	require.True(isRequeueError(requeue.NeededAfter(nil, time.Second)))
	require.True(isRequeueError(requeue.Needed(errors.New("x"))))
	require.True(isRequeueError(fmt.Errorf("wrapped: %w", requeue.NeededAfter(nil, time.Second))))
	require.False(isRequeueError(errors.New("x")))
	require.False(isRequeueError(nil))
}

// convergenceHarness drives updateResource across syncs. The CR's annotations
// live in stored, to which every metadata patch applies its merge patch,
// standing in for the API server.
type convergenceHarness struct {
	t             *testing.T
	rr            *resourceReconciler
	stored        map[string]string
	delta         *ackcompare.Delta
	calls         []string // write calls the fake Update makes
	err           error    // what the fake Update returns
	synced        bool     // what IsSynced returns
	preCompare    func(a acktypes.AWSResource)
	updates       int
	patches       int
	recordPatches int      // patches that change the convergence annotation
	patchData     []string // every metadata+spec patch body
	sawNoRec      bool     // an Update ran without a recorder in its context
}

func newConvergenceHarness(t *testing.T, gateOn bool) *convergenceHarness {
	h := &convergenceHarness{
		t:      t,
		stored: map[string]string{},
		delta:  twoDiffDelta("false", "perQueue"),
		calls:  []string{"SQS.SetQueueAttributes"},
		synced: true,
	}

	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "bookstore.services.k8s.aws", Kind: "fakeBook",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})
	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("Delta", mock.Anything, mock.Anything).Return(
		func(a, b acktypes.AWSResource) *ackcompare.Delta {
			if h.preCompare != nil {
				h.preCompare(a)
			}
			return h.delta
		},
	)
	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	r, kc, _ := reconcilerMocks(rmf)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		h.patches++
		data, err := args.Get(2).(client.Patch).Data(args.Get(1).(client.Object))
		require.NoError(t, err)
		h.patchData = append(h.patchData, string(data))
		var p struct {
			Metadata struct {
				Annotations map[string]*string `json:"annotations"`
			} `json:"metadata"`
		}
		require.NoError(t, json.Unmarshal(data, &p))
		for k, v := range p.Metadata.Annotations {
			if k == ackv1alpha1.AnnotationConvergence {
				h.recordPatches++
			}
			if v == nil {
				delete(h.stored, k)
			} else {
				h.stored[k] = *v
			}
		}
	}).Return(nil)

	rr := r.(*resourceReconciler)
	rr.cfg.FeatureGates[featuregate.ConvergenceDetection] = featuregate.Feature{Enabled: gateOn}
	h.rr = rr
	return h
}

func (h *convergenceHarness) rm() *ackmocks.AWSResourceManager {
	rm := &ackmocks.AWSResourceManager{}
	// Generated ClearResolvedReferences returns a copy.
	rm.On("ClearResolvedReferences", mock.Anything).Return(
		func(r acktypes.AWSResource) acktypes.AWSResource { return r.DeepCopy() },
	)
	rm.On("FilterSystemTags", mock.Anything, mock.Anything).Return()
	rm.On("IsSynced", mock.Anything, mock.Anything).Return(
		func(context.Context, acktypes.AWSResource) (bool, error) { return h.synced, nil },
	)
	rm.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(
			ctx context.Context, desired, _ acktypes.AWSResource, _ *ackcompare.Delta,
		) (acktypes.AWSResource, error) {
			h.updates++
			rec := apiCallRecorderFrom(ctx)
			if rec == nil {
				h.sawNoRec = true
			} else {
				for _, c := range h.calls {
					rec.add(c)
				}
			}
			return desired, h.err
		},
	)
	return rm
}

// sync runs one updateResource against a CR carrying the stored annotations.
func (h *convergenceHarness) sync() (acktypes.AWSResource, error) {
	anns := map[string]string{}
	for k, v := range h.stored {
		anns[k] = v
	}
	desired := newUResource(anns, map[string]interface{}{"a": "1"})
	latest := desired.DeepCopy()
	return h.rr.updateResource(context.TODO(), h.rm(), desired, latest)
}

func (h *convergenceHarness) record() *convergenceRecord {
	return convergenceRecordFrom(newUResource(h.stored, nil))
}

func notConverging(res acktypes.AWSResource) bool {
	c := ackcondition.NotConverging(res)
	return c != nil && c.Status == corev1.ConditionTrue
}

func TestConvergence_SkipsAfterTwoSuccessfulUpdates(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)

	for i := 1; i <= 2; i++ {
		out, err := h.sync()
		require.NoError(err)
		require.False(notConverging(out))
		require.Equal(i, h.updates)
		require.Equal(i, h.record().SuccessfulUpdates)
		require.Equal([]string{"SQS.SetQueueAttributes"}, h.record().APICalls)
	}
	for i := 0; i < 2; i++ {
		out, err := h.sync()
		require.NoError(err)
		require.True(notConverging(out))
		require.Equal(2, h.updates, "skipped")
	}
}

func TestConvergence_DeltaChangeSendsUpdateAgain(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	for i := 0; i < 3; i++ {
		_, _ = h.sync()
	}
	require.Equal(2, h.updates)

	h.delta = twoDiffDelta("true", "perQueue") // e.g. an outside change
	out, err := h.sync()
	require.NoError(err)
	require.False(notConverging(out))
	require.Equal(3, h.updates)
	require.Equal(1, h.record().SuccessfulUpdates)

	h.delta = twoDiffDelta("false", "perQueue") // AWS back to its own value
	for i := 0; i < 3; i++ {
		_, _ = h.sync()
	}
	require.Equal(5, h.updates, "two more Updates, then skip")
}

func TestConvergence_DifferentCallsNeverSkip(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	for i := 0; i < 4; i++ {
		h.calls = []string{fmt.Sprintf("EKS.Step%d", i%2)}
		out, err := h.sync()
		require.NoError(err)
		require.False(notConverging(out))
	}
	require.Equal(4, h.updates)
	require.Equal(1, h.record().SuccessfulUpdates)
}

func TestConvergence_UpdateWithoutWriteCallsLeavesRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.calls = nil
	for i := 0; i < 3; i++ {
		_, err := h.sync()
		require.NoError(err)
	}
	require.Equal(3, h.updates)
	require.Nil(h.record())
}

func TestConvergence_FailedUpdateNotCounted(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.err = errors.New("ValidationException")
	for i := 0; i < 3; i++ {
		_, err := h.sync()
		require.Error(err)
	}
	require.Equal(3, h.updates)
	require.Nil(h.record())
	require.Equal(0, h.patches)
}

// An Update that writes and then asks to be requeued (wafv2 WebACL before
// wafv2#78) is counted through an annotation-only patch.
func TestConvergence_RequeueAfterWriteIsCounted(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.err = requeue.NeededAfter(nil, time.Second)
	for i := 1; i <= 2; i++ {
		_, err := h.sync()
		require.True(isRequeueError(err))
		require.Equal(i, h.record().SuccessfulUpdates)
	}
	require.Equal(2, h.patches, "one annotation patch per counted Update")

	out, err := h.sync()
	require.NoError(err, "a skip returns no requeue, so the loop ends")
	require.True(notConverging(out))
	require.Equal(2, h.updates)
}

func TestConvergence_TransitioningDoesNotSkip(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()

	h.synced = false
	before := h.patches
	out, err := h.sync()
	require.NoError(err)
	require.False(notConverging(out))
	require.Equal(3, h.updates)
	require.Equal(2, h.recordPatches)
	require.Equal(1, h.patches-before, "only the spec patch: an unchanged record isn't patched again")

	h.synced = true
	out, err = h.sync()
	require.NoError(err)
	require.True(notConverging(out))
	require.Equal(3, h.updates)
}

func TestConvergence_SyncedFalseConditionDoesNotSkip(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()

	anns := map[string]string{}
	for k, v := range h.stored {
		anns[k] = v
	}
	desired := newUResource(anns, map[string]interface{}{"a": "1"})
	latest := desired.DeepCopy()
	msg := "cluster is UPDATING"
	ackcondition.SetSynced(latest, corev1.ConditionFalse, &msg, nil)
	_, err := h.rr.updateResource(context.TODO(), h.rm(), desired, latest)
	require.NoError(err)
	require.Equal(3, h.updates)
}

func TestConvergence_EmptyDeltaRemovesRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.stored["keep"] = "me"
	_, _ = h.sync()
	_, _ = h.sync()
	require.NotNil(h.record())

	h.delta = ackcompare.NewDelta()
	out, err := h.sync()
	require.NoError(err)
	require.False(notConverging(out))
	require.Nil(h.record())
	require.Nil(convergenceRecordFrom(out))
	require.Equal("me", h.stored["keep"])
	require.Equal(2, h.updates)
}

func TestConvergence_GateOffBehavesAsToday(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, false)
	for i := 0; i < 4; i++ {
		out, err := h.sync()
		require.NoError(err)
		require.False(notConverging(out))
	}
	require.Equal(4, h.updates)
	require.Nil(h.record())
	require.True(h.sawNoRec, "no recorder in the Update's context")
}

// The record is dropped when the delta changes, even if that Update isn't
// counted. Otherwise a record left by an earlier change skips the same change
// the next time it happens: a Subnet whose perpetual delta makes no write call
// never replaces the record of a corrected outside change.
func TestConvergence_RecordDroppedWhenDeltaChanges(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	outside := twoDiffDelta("true", "perQueue")
	perpetual := twoDiffDelta("false", "perQueue")

	for i := 0; i < 2; i++ { // the outside change, corrected twice
		h.delta, h.calls = outside, []string{"EC2.ModifySubnetAttribute"}
		_, err := h.sync()
		require.NoError(err)
		h.delta, h.calls = perpetual, nil
		_, err = h.sync()
		require.NoError(err)
		require.Nil(h.record(), "a different delta drops the record")
	}

	h.delta, h.calls = outside, []string{"EC2.ModifySubnetAttribute"}
	out, err := h.sync()
	require.NoError(err)
	require.False(notConverging(out), "the third outside change is corrected")
	require.Equal(5, h.updates)
}

func TestConvergence_FailedUpdateOnNewDeltaDropsRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()
	require.Equal(2, h.record().SuccessfulUpdates)

	h.delta = twoDiffDelta("true", "perQueue")
	h.err = errors.New("ValidationException")
	_, err := h.sync()
	require.Error(err)
	require.Nil(h.record())
}

// An Update with no write call on the same delta, such as a guard that
// requeues while the resource is modifying, keeps the record.
func TestConvergence_SameDeltaWithoutCallsKeepsRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	require.Equal(1, h.record().SuccessfulUpdates)

	h.calls = nil
	h.err = requeue.NeededAfter(nil, time.Second)
	_, _ = h.sync()
	require.Equal(1, h.record().SuccessfulUpdates)

	h.calls, h.err = []string{"SQS.SetQueueAttributes"}, nil
	_, _ = h.sync()
	require.Equal(2, h.record().SuccessfulUpdates)
}

// Writing the record must not change what the spec patch writes. A compare
// hook that fills a default into its first argument in place (ec2 VPC
// customPreCompare) also fills the patch base, so the default isn't written
// to the spec. That holds only if the record isn't on the patch target: a
// metadata difference makes the patch skip the compare.
func TestConvergence_RecordWriteDoesNotChangeSpecPatch(t *testing.T) {
	for _, gateOn := range []bool{false, true} {
		h := newConvergenceHarness(t, gateOn)
		h.preCompare = func(a acktypes.AWSResource) {
			obj := a.RuntimeObject().(*unstructured.Unstructured).Object
			if _, found, _ := unstructured.NestedString(obj, "spec", "dflt"); !found {
				_ = unstructured.SetNestedField(obj, "aws-default", "spec", "dflt")
			}
		}
		for i := 0; i < 3; i++ {
			_, err := h.sync()
			require.NoError(t, err)
		}
		for _, d := range h.patchData {
			require.NotContains(t, d, "dflt", "gate=%v: patch %s", gateOn, d)
		}
	}
}
