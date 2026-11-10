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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	corev1 "k8s.io/api/core/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// convergenceSkipThreshold is how many successful Updates in a row, with the
// same delta and the same write calls, it takes before ACK skips the Update.
const convergenceSkipThreshold = 2

var notConvergingMessage = "Update skipped: the same Update already " +
	"succeeded twice without changing anything. ACK sends it again when " +
	"the delta changes."

// convergenceRecord is stored in the AnnotationConvergence annotation.
type convergenceRecord struct {
	// DeltaHash is a SHA-256 of the delta at the last counted Update.
	DeltaHash string `json:"deltaHash"`
	// APICalls are the AWS write calls that Update made successfully.
	APICalls []string `json:"apiCalls"`
	// SuccessfulUpdates counts the successful Updates in a row that matched
	// both DeltaHash and APICalls, capped at convergenceSkipThreshold.
	SuccessfulUpdates int `json:"successfulUpdates"`
}

// deltaHash returns a SHA-256 of the delta's differences, independent of
// their order. It returns "" when a value can't be serialized; the caller then
// neither records nor skips.
func deltaHash(delta *ackcompare.Delta) string {
	if delta == nil || len(delta.Differences) == 0 {
		return ""
	}
	entries := make([]string, 0, len(delta.Differences))
	for _, d := range delta.Differences {
		b, err := json.Marshal(d)
		if err != nil {
			return ""
		}
		entries = append(entries, string(b))
	}
	sort.Strings(entries)
	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// nextConvergenceRecord returns the record after a successful Update that
// made write calls.
func nextConvergenceRecord(
	prev *convergenceRecord,
	hash string,
	calls []string,
) *convergenceRecord {
	next := &convergenceRecord{DeltaHash: hash, APICalls: calls, SuccessfulUpdates: 1}
	if prev != nil && prev.DeltaHash == hash && slices.Equal(prev.APICalls, calls) {
		next.SuccessfulUpdates = min(prev.SuccessfulUpdates+1, convergenceSkipThreshold)
	}
	return next
}

// shouldSkipUpdate reports whether the record says this delta already
// succeeded enough times without changing anything.
func shouldSkipUpdate(rec *convergenceRecord, hash string) bool {
	return rec != nil && hash != "" && rec.DeltaHash == hash &&
		rec.SuccessfulUpdates >= convergenceSkipThreshold
}

// isRequeueError reports whether err asks for the resource to be requeued.
func isRequeueError(err error) bool {
	var needed *requeue.RequeueNeeded
	var neededAfter *requeue.RequeueNeededAfter
	return errors.As(err, &needed) || errors.As(err, &neededAfter)
}

// convergenceRecordFrom returns the resource's record, or nil if it has none
// or the annotation can't be parsed.
func convergenceRecordFrom(res acktypes.AWSResource) *convergenceRecord {
	v := res.MetaObject().GetAnnotations()[ackv1alpha1.AnnotationConvergence]
	if v == "" {
		return nil
	}
	rec := &convergenceRecord{}
	if err := json.Unmarshal([]byte(v), rec); err != nil || rec.DeltaHash == "" {
		return nil
	}
	return rec
}

// setConvergenceRecord stores rec on res, or removes the annotation when rec
// is nil.
func setConvergenceRecord(res acktypes.AWSResource, rec *convergenceRecord) error {
	meta := res.MetaObject()
	anns := map[string]string{}
	for k, v := range meta.GetAnnotations() {
		anns[k] = v
	}
	if rec == nil {
		delete(anns, ackv1alpha1.AnnotationConvergence)
	} else {
		b, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		anns[ackv1alpha1.AnnotationConvergence] = string(b)
	}
	meta.SetAnnotations(anns)
	return nil
}

// isTransitioning reports whether the read says the resource is changing
// state, in which case ACK doesn't skip the Update.
func isTransitioning(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	latest acktypes.AWSResource,
) bool {
	if c := ackcondition.Synced(latest); c != nil && c.Status == corev1.ConditionFalse {
		return true
	}
	synced, err := rm.IsSynced(ctx, latest)
	return err != nil || !synced
}

// patchConvergenceRecord patches only the record annotation on the CR: the
// base is desired and the target a copy of desired with the record set, or
// removed when rec is nil.
func (r *resourceReconciler) patchConvergenceRecord(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	desired acktypes.AWSResource,
	rec *convergenceRecord,
) error {
	target := desired.DeepCopy()
	if err := setConvergenceRecord(target, rec); err != nil {
		return err
	}
	_, err := r.patchResourceMetadataAndSpec(ctx, rm, desired, target)
	return err
}
