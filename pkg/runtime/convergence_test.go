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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8srtschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
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

	first := nextConvergenceRecord(convergenceRecord{}, "u1", "h1", calls)
	require.Equal(convergenceRecord{uid: "u1", deltaHash: "h1", apiCalls: calls, count: 1}, first)

	second := nextConvergenceRecord(first, "u1", "h1", calls)
	require.Equal(2, second.count)
	require.Equal(2, nextConvergenceRecord(second, "u1", "h1", calls).count, "capped")

	require.Equal(1, nextConvergenceRecord(second, "u1", "h2", calls).count, "new delta restarts")
	require.Equal(1, nextConvergenceRecord(second, "u1", "h1",
		[]string{"SQS.TagQueue"}).count, "new calls restart")

	skipping := second
	skipping.skippingSince = time.Unix(1, 0)
	require.True(nextConvergenceRecord(skipping, "u1", "h1", calls).skippingSince.IsZero(),
		"a counted Update starts skipping over")
}

func TestIsRequeueError(t *testing.T) {
	require := require.New(t)
	require.True(isRequeueError(requeue.NeededAfter(nil, time.Second)))
	require.True(isRequeueError(requeue.Needed(errors.New("x"))))
	require.True(isRequeueError(fmt.Errorf("wrapped: %w", requeue.NeededAfter(nil, time.Second))))
	require.False(isRequeueError(errors.New("x")))
	require.False(isRequeueError(nil))
}

// Reconciles of different CRs run concurrently and share one store.
func TestConvergenceStore_ConcurrentReconciles(t *testing.T) {
	var s convergenceStore
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := types.NamespacedName{Namespace: "default", Name: fmt.Sprintf("cr-%d", i)}
			for j := 0; j < 500; j++ {
				s.set(key, convergenceRecord{deltaHash: "h", count: j})
				rec, ok := s.get(key, "")
				assert.True(t, ok)
				assert.Equal(t, j, rec.count)
				s.delete(key)
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, s.records)
}

var convergenceTestKey = types.NamespacedName{Namespace: "default", Name: "mybook"}

// convergenceHarness drives updateResource across syncs of one CR.
type convergenceHarness struct {
	t          *testing.T
	gateOn     bool
	rr         *resourceReconciler
	ctx        context.Context
	now        time.Time // what the reconciler's clock returns
	uid        types.UID // the CR's UID
	delta      *ackcompare.Delta
	calls      []string // write calls the fake Update makes
	err        error    // what the fake Update returns
	synced     bool     // what IsSynced returns
	syncedCond bool     // the read sets ACK.ResourceSynced=False
	updates    int
	patchData  []string // every metadata+spec patch body
	sawNoRec   bool     // an Update ran without a recorder in its context
}

func newConvergenceHarness(t *testing.T, gateOn bool) *convergenceHarness {
	h := &convergenceHarness{
		t:      t,
		gateOn: gateOn,
		ctx:    context.TODO(),
		now:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		uid:    "uid-1",
		delta:  twoDiffDelta("false", "perQueue"),
		calls:  []string{"SQS.SetQueueAttributes"},
		synced: true,
	}
	h.start()
	return h
}

// start builds a new reconciler, as a controller (re)start does.
func (h *convergenceHarness) start() {
	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(k8srtschema.GroupVersionKind{
		Group: "bookstore.services.k8s.aws", Kind: "fakeBook",
	})
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})
	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("Delta", mock.Anything, mock.Anything).Return(
		func(a, b acktypes.AWSResource) *ackcompare.Delta { return h.delta },
	)
	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)

	r, kc, _ := reconcilerMocks(rmf)
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		data, err := args.Get(2).(client.Patch).Data(args.Get(1).(client.Object))
		require.NoError(h.t, err)
		h.patchData = append(h.patchData, string(data))
	}).Return(nil)

	rr := r.(*resourceReconciler)
	rr.cfg.FeatureGates[featuregate.ConvergenceDetection] = featuregate.Feature{Enabled: h.gateOn}
	rr.convergence.now = func() time.Time { return h.now }
	h.rr = rr
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

// newCR returns a fresh copy of the CR as the API server stores it.
func (h *convergenceHarness) newCR() *uResource {
	cr := newUResource(nil, map[string]interface{}{"a": "1"})
	cr.obj.SetNamespace(convergenceTestKey.Namespace)
	cr.obj.SetName(convergenceTestKey.Name)
	cr.obj.SetUID(h.uid)
	return cr
}

// sync runs one updateResource with fresh copies of the CR.
func (h *convergenceHarness) sync() (acktypes.AWSResource, error) {
	desired := h.newCR()
	latest := desired.DeepCopy()
	if h.syncedCond {
		msg := "cluster is UPDATING"
		ackcondition.SetSynced(latest, corev1.ConditionFalse, &msg, nil)
	}
	return h.rr.updateResource(h.ctx, h.rm(), desired, latest)
}

// syncs runs n syncs, each a resync period later than the last.
func (h *convergenceHarness) syncs(n int) {
	for i := 0; i < n; i++ {
		h.now = h.now.Add(10 * time.Hour)
		_, _ = h.sync()
	}
}

func (h *convergenceHarness) record() *convergenceRecord {
	if rec, ok := h.rr.convergence.get(convergenceTestKey, h.uid); ok {
		return &rec
	}
	return nil
}

func TestConvergence_SkipsAfterTwoCountedUpdates(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)

	for i := 1; i <= 2; i++ {
		_, err := h.sync()
		require.NoError(err)
		require.Equal(i, h.updates)
		require.Equal(i, h.record().count)
		require.Equal([]string{"SQS.SetQueueAttributes"}, h.record().apiCalls)
	}
	patches := len(h.patchData)
	for i := 0; i < 2; i++ {
		out, err := h.sync()
		require.NoError(err)
		require.Equal(2, h.updates, "skipped")
		require.NotNil(out)
	}
	require.Equal(patches, len(h.patchData), "a skip writes nothing to the CR")
}

// While the delta stays the same, ACK sends the Update once more 24 hours
// after it started skipping, then skips for another 24 hours.
func TestConvergence_RetriesAfter24Hours(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()

	start := h.now
	_, _ = h.sync()
	require.Equal(2, h.updates, "skipping starts")
	require.Equal(start, h.record().skippingSince)

	h.now = start.Add(24*time.Hour - time.Second)
	_, _ = h.sync()
	require.Equal(2, h.updates, "still skipping just before 24 hours")

	h.now = start.Add(24 * time.Hour)
	_, err := h.sync()
	require.NoError(err)
	require.Equal(3, h.updates, "sent once more after 24 hours")
	require.Equal(2, h.record().count)
	require.True(h.record().skippingSince.IsZero())

	restart := h.now.Add(time.Minute)
	h.now = restart
	_, _ = h.sync()
	require.Equal(3, h.updates, "skips again after the retry")

	h.now = restart.Add(24*time.Hour - time.Second)
	_, _ = h.sync()
	require.Equal(3, h.updates, "for another 24 hours")

	h.now = restart.Add(24 * time.Hour)
	_, _ = h.sync()
	require.Equal(4, h.updates, "then sends it once more")
}

// A retry that isn't counted, here a failed one, leaves the record alone, so
// ACK keeps sending the Update until one is counted, as it does today.
func TestConvergence_FailedRetryIsSentAgain(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.syncs(3)
	require.Equal(2, h.updates)

	h.now = h.now.Add(24 * time.Hour)
	h.err = errors.New("ThrottlingException")
	_, err := h.sync()
	require.Error(err)
	_, err = h.sync()
	require.Error(err)
	require.Equal(4, h.updates)

	h.err = nil
	h.syncs(2)
	require.Equal(5, h.updates, "the counted retry is followed by a skip")
}

// The record lives only in the controller's memory: after a restart ACK sends
// the Update up to 2 more times, then skips again.
func TestConvergence_RestartLosesRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.syncs(3)
	require.Equal(2, h.updates)

	h.start()
	require.Nil(h.record())
	h.syncs(3)
	require.Equal(4, h.updates)
}

// Nothing about the record reaches the CR: no annotation, no condition, and
// the same patches as with the gate off.
func TestConvergence_WritesNoAnnotationOrCondition(t *testing.T) {
	require := require.New(t)
	on := newConvergenceHarness(t, true)
	off := newConvergenceHarness(t, false)
	for i := 0; i < 4; i++ {
		out, err := on.sync()
		require.NoError(err)
		require.Empty(out.Conditions())
		require.Empty(out.MetaObject().GetAnnotations())
		_, err = off.sync()
		require.NoError(err)
	}
	require.Equal(2, on.updates)
	require.Equal(4, off.updates)
	require.Len(on.patchData, 2)
	require.Equal(off.patchData[:2], on.patchData)
	for _, d := range on.patchData {
		require.NotContains(d, "convergence")
	}
}

// Each skip writes one line at the default log level. It names the resource
// (the resource logger Reconcile builds carries kind, namespace and name) and
// the delta's field paths, with no values.
func TestConvergence_SkipLogLine(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	var lines []string
	sink := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{})
	h.ctx = context.WithValue(context.TODO(), ackrtlog.ContextKey, ackrtlog.NewResourceLogger(
		sink, h.newCR(),
		"kind", "fakeBook",
		"namespace", convergenceTestKey.Namespace,
		"name", convergenceTestKey.Name,
	))
	_, _ = h.sync()
	_, _ = h.sync()

	for i := 0; i < 2; i++ {
		lines = nil
		_, err := h.sync()
		require.NoError(err)
		require.Equal(2, h.updates)
		require.Len(lines, 1)
		line := lines[0]
		require.Contains(line, `"level"=0`)
		require.Contains(line, `"msg"="skipping update:`)
		require.Contains(line, `"kind"="fakeBook" "namespace"="default" "name"="mybook"`)
		require.Contains(line,
			`"fields"=["Spec.ContentBasedDeduplication" "Spec.FIFOThroughputLimit"]`)
		require.NotContains(line, "perQueue")
		require.NotContains(line, `"false"`)
	}
}

func TestConvergence_DeltaChangeSendsUpdateAgain(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.syncs(3)
	require.Equal(2, h.updates)

	h.delta = twoDiffDelta("true", "perQueue") // e.g. an outside change
	_, err := h.sync()
	require.NoError(err)
	require.Equal(3, h.updates)
	require.Equal(1, h.record().count)

	h.delta = twoDiffDelta("false", "perQueue") // AWS back to its own value
	h.syncs(3)
	require.Equal(5, h.updates, "two more Updates, then skip")
}

func TestConvergence_DifferentCallsNeverSkip(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	for i := 0; i < 4; i++ {
		h.calls = []string{fmt.Sprintf("EKS.Step%d", i%2)}
		_, err := h.sync()
		require.NoError(err)
	}
	require.Equal(4, h.updates)
	require.Equal(1, h.record().count)
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

func TestConvergence_UnhashableDeltaNeverSkips(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.delta = ackcompare.NewDelta()
	h.delta.Add("Spec.X", make(chan int), nil)
	h.syncs(4)
	require.Equal(4, h.updates)
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
	require.Empty(h.patchData)
}

// An Update that writes and then asks to be requeued (wafv2 WebACL before
// wafv2#78) is counted.
func TestConvergence_RequeueAfterWriteIsCounted(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.err = requeue.NeededAfter(nil, time.Second)
	for i := 1; i <= 2; i++ {
		_, err := h.sync()
		require.True(isRequeueError(err))
		require.Equal(i, h.record().count)
	}

	_, err := h.sync()
	require.NoError(err, "a skip returns no requeue, so the loop ends")
	require.Equal(2, h.updates)
}

func TestConvergence_TransitioningDoesNotSkip(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()

	h.synced = false
	_, err := h.sync()
	require.NoError(err)
	require.Equal(3, h.updates)
	require.True(h.record().skippingSince.IsZero(), "not skipping")

	h.synced = true
	_, err = h.sync()
	require.NoError(err)
	require.Equal(3, h.updates)
}

func TestConvergence_SyncedFalseConditionDoesNotSkip(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()

	h.syncedCond = true
	_, err := h.sync()
	require.NoError(err)
	require.Equal(3, h.updates)
}

func TestConvergence_EmptyDeltaRemovesRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()
	require.NotNil(h.record())

	h.delta = ackcompare.NewDelta()
	_, err := h.sync()
	require.NoError(err)
	require.Nil(h.record())
	require.Equal(2, h.updates)
}

func TestConvergence_GateOffBehavesAsToday(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, false)
	for i := 0; i < 4; i++ {
		_, err := h.sync()
		require.NoError(err)
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
	_, err := h.sync()
	require.NoError(err)
	require.Equal(5, h.updates, "the third outside change is corrected")
}

func TestConvergence_FailedUpdateOnNewDeltaDropsRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	_, _ = h.sync()
	_, _ = h.sync()
	require.Equal(2, h.record().count)

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
	require.Equal(1, h.record().count)

	h.calls = nil
	h.err = requeue.NeededAfter(nil, time.Second)
	_, _ = h.sync()
	require.Equal(1, h.record().count)

	h.calls, h.err = []string{"SQS.SetQueueAttributes"}, nil
	_, _ = h.sync()
	require.Equal(2, h.record().count)
}

// Reconcile drops the record once the CR is gone.
func TestConvergence_DeletedCRDropsRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.syncs(2)
	require.NotNil(h.record())

	reader := &ctrlrtclientmock.Reader{}
	reader.On("Get", mock.Anything, convergenceTestKey, mock.Anything).Return(
		apierrors.NewNotFound(k8srtschema.GroupResource{}, convergenceTestKey.Name),
	)
	h.rr.apiReader = reader
	_, err := h.rr.Reconcile(context.TODO(), ctrlrt.Request{NamespacedName: convergenceTestKey})
	require.NoError(err)
	require.Empty(h.rr.convergence.records)
}

// A CR re-created with the same name doesn't inherit the old CR's record.
func TestConvergence_RecreatedCRStartsWithoutRecord(t *testing.T) {
	require := require.New(t)
	h := newConvergenceHarness(t, true)
	h.syncs(3)
	require.Equal(2, h.updates)

	h.uid = "uid-2"
	require.Nil(h.record())
	_, err := h.sync()
	require.NoError(err)
	require.Equal(3, h.updates)
	require.Equal(1, h.record().count)
}
