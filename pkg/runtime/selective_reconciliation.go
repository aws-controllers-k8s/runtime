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

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8srt "k8s.io/apimachinery/pkg/runtime"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// IgnoreFieldDriftPaths returns the list of dotted, JSON-style field paths that
// the supplied resource has marked as ignored via the
// services.k8s.aws/ignore-field-drift annotation. Returns an empty slice if the
// annotation is absent or empty.
func IgnoreFieldDriftPaths(res acktypes.AWSResource) []string {
	return splitAnnotationPaths(res, ackv1alpha1.AnnotationIgnoreFieldDrift)
}

// splitAnnotationPaths reads the named annotation from the resource and splits
// its comma-separated value into a slice of trimmed, non-empty paths.
func splitAnnotationPaths(res acktypes.AWSResource, annotation string) []string {
	mo := res.MetaObject()
	if mo == nil {
		return nil
	}
	raw, ok := mo.GetAnnotations()[annotation]
	if !ok {
		return nil
	}
	parts := strings.Split(raw, ",")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return paths
}

// HasSelectiveReconciliation returns true if the supplied resource carries any
// annotation that selectively scopes which fields the controller reconciles.
func HasSelectiveReconciliation(res acktypes.AWSResource) bool {
	return len(IgnoreFieldDriftPaths(res)) > 0
}

// applyIgnoredFields returns a deep copy of desired with the ignore-field-drift
// annotation applied by merging observed (latest) values in. The returned
// resource is used ONLY for delta computation and request building; the stored
// CR is never mutated.
//
// For each ignored path the whole field value from latest replaces the value in
// the desired copy (or is removed if absent from latest), so the field compares
// equal and never drives an Update.
//
// If the SelectiveReconciliation feature gate is disabled or the resource
// carries no ignore-field-drift annotation, desired is returned unchanged.
func applyIgnoredFields(
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
	featureGates featuregate.FeatureGates,
) (acktypes.AWSResource, error) {
	if !featureGates.IsEnabled(featuregate.SelectiveReconciliation) {
		return desired, nil
	}
	ignorePaths := IgnoreFieldDriftPaths(desired)
	if len(ignorePaths) == 0 {
		return desired, nil
	}

	out := desired.DeepCopy()
	dObj := out.RuntimeObject()
	d, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(dObj)
	if err != nil {
		return desired, err
	}
	l, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(latest.RuntimeObject())
	if err != nil {
		return desired, err
	}

	for _, p := range ignorePaths {
		parts := pathParts(p)
		if v, found, err := unstructured.NestedFieldCopy(l, parts...); err == nil && found {
			_ = unstructured.SetNestedField(d, v, parts...)
		} else {
			unstructured.RemoveNestedField(d, parts...)
		}
	}

	if err := k8srt.DefaultUnstructuredConverter.FromUnstructured(d, dObj); err != nil {
		return desired, err
	}
	return out, nil
}

// restoreIgnoredFields copies, for each ignored path, the value from the
// declared `desired` resource into `updated` in place, so the stored CR
// retains the user's declared value rather than the AWS-observed value that
// the anti-clobber merge placed into the update request. If the declared
// `desired` does not set a path, that path is removed from `updated` so the
// stored spec matches the declared (absent) state. No-op when the gate is
// disabled or no paths are supplied. Used ONLY on the update write-back path.
func restoreIgnoredFields(
	updated, desired acktypes.AWSResource,
	paths []string,
	featureGates featuregate.FeatureGates,
) error {
	if !featureGates.IsEnabled(featuregate.SelectiveReconciliation) {
		return nil
	}
	if len(paths) == 0 {
		return nil
	}

	uObj := updated.RuntimeObject()
	u, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(uObj)
	if err != nil {
		return err
	}
	d, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(desired.RuntimeObject())
	if err != nil {
		return err
	}

	for _, p := range paths {
		parts := pathParts(p)
		if v, found, err := unstructured.NestedFieldCopy(d, parts...); err == nil && found {
			_ = unstructured.SetNestedField(u, v, parts...)
		} else {
			unstructured.RemoveNestedField(u, parts...)
		}
	}

	return k8srt.DefaultUnstructuredConverter.FromUnstructured(u, uObj)
}

// rebaseIgnoredFieldsForPersist returns a deep copy of `latest` (the resource
// passed into LateInitialize, used as the late-init patch base) in which, for
// each ignored path that carries a late-initialized value the DECLARED CR left
// unset, the base value is replaced by the DECLARED `desired` value (or removed
// if `desired` leaves it unset). It mutates ONLY those paths; every other field
// of the returned base is identical to `latest`.
//
// This is used by lateInitializeResource to implement "late-init wins" for an
// ignore-field-drift field: when the user left the field unset and the AWS
// service defaulted it, late-init surfaces that value and it must be persisted
// to the stored CR exactly once. The normal MergeFrom(latest) base works for
// non-ignored fields, but for an ignored field the read path may have already
// populated `latest.Spec.X` from AWS, so a MergeFrom(latest) diff for the
// ignored path collapses to empty and the value never reaches the stored CR.
// Rebasing the path onto the declared (typically unset) value makes the
// MergeFrom diff carry the value, so the patch persists it once.
//
// A path is rebased ONLY when BOTH:
//
//   - the resource carries an ACK.LateInitialized condition — i.e. late-init is
//     actually configured for the resource and ran. This is the signal that the
//     value on the ignored field is a late-initialized (service-defaulted)
//     value rather than plain post-creation drift. It is precisely what
//     distinguishes "late-init wins" (persist, matrix Row 6) from drift
//     suppression on a non-late-init ignored field (do NOT persist, Row 5); AND
//   - the path is ABSENT (declared-unset) in the DECLARED `desired` spec.
//     Late-init only ever fills fields the user left unset, so persisting a
//     late-init value to the stored CR must fire ONLY for a declared-absent
//     path. For a path the user DID declare (set to any value, including an
//     edit), the normal retain path owns it: the stored CR keeps whatever the
//     user declared and is never force-rebased against the AWS-observed value.
//     The ACK.LateInitialized condition is a RESOURCE-level signal (set because
//     SOME field is late-init) and says nothing about a specific ignored path,
//     so this declared-absent check is what prevents clobbering a declared-set
//     ignored field (e.g. a user-edited spec.tags) on a late-init-configured
//     resource.
//
// No-op (returns latest unchanged) when the gate is disabled, no paths are
// supplied, or the resource carries no ACK.LateInitialized condition. Gated
// behind the ignore-field-drift annotation by its caller, so it never runs for
// non-annotated (e.g. testify-mock) resources.
func rebaseIgnoredFieldsForPersist(
	latest, desired, lateInited acktypes.AWSResource,
	paths []string,
	featureGates featuregate.FeatureGates,
) (acktypes.AWSResource, error) {
	if !featureGates.IsEnabled(featuregate.SelectiveReconciliation) {
		return latest, nil
	}
	if len(paths) == 0 {
		return latest, nil
	}
	// Only late-init-configured resources persist an ignored field's value.
	// Without a late-init condition the ignored value is plain drift (Row 5)
	// and must not be written back to the CR.
	if !hasLateInitializedCondition(lateInited) {
		return latest, nil
	}

	out := latest.DeepCopy()
	oObj := out.RuntimeObject()
	o, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(oObj)
	if err != nil {
		return latest, err
	}
	d, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(desired.RuntimeObject())
	if err != nil {
		return latest, err
	}

	for _, p := range paths {
		parts := pathParts(p)
		// Only rebase a path the user left UNSET in the declared spec. Late-init
		// only fills declared-absent fields, so a declared-PRESENT ignored path
		// (e.g. a user-edited spec.tags) is owned by the normal retain path and
		// must never be force-rebased against the AWS-observed value. Skipping it
		// here leaves the base equal to `latest` on that path, so the MergeFrom
		// patch diff for the path collapses and the user's declared value is
		// retained.
		if _, dFound, _ := unstructured.NestedFieldNoCopy(d, parts...); dFound {
			continue
		}
		// Declared-absent path that late-init filled: rebase the base onto the
		// declared (unset) state so the MergeFrom diff carries the late-init
		// value and it is persisted once. `desired` does not set the path, so
		// this removes it from the base.
		if v, found, err := unstructured.NestedFieldCopy(d, parts...); err == nil && found {
			_ = unstructured.SetNestedField(o, v, parts...)
		} else {
			unstructured.RemoveNestedField(o, parts...)
		}
	}

	if err := k8srt.DefaultUnstructuredConverter.FromUnstructured(o, oObj); err != nil {
		return latest, err
	}
	return out, nil
}

// hasLateInitializedCondition reports whether the resource carries an
// ACK.LateInitialized condition, which the generated LateInitialize sets only
// for resources that actually have late-init fields configured. It is used as
// the signal that an ignored field's value is a late-initialized
// (service-defaulted) value rather than plain drift.
func hasLateInitializedCondition(res acktypes.AWSResource) bool {
	cm, ok := res.(acktypes.ConditionManager)
	if !ok {
		return false
	}
	for _, c := range cm.Conditions() {
		if c != nil && c.Type == ackv1alpha1.ConditionTypeLateInitialized {
			return true
		}
	}
	return false
}

// pathParts splits a dotted JSON-style path (e.g. "spec.tags") into its
// component parts for use with the unstructured helpers.
func pathParts(p string) []string {
	return strings.Split(p, ".")
}

// FilterIgnoredDeltaDifferences removes, in place, any difference in the
// supplied delta that falls under a field path the resource has marked as
// ignored via the services.k8s.aws/ignore-field-drift annotation. This
// guarantees that drift on an ignored field never marks a resource out-of-sync,
// no matter which code path consumes the delta (update, requeue/synced
// decisions, etc.).
//
// It is a no-op (delta is left untouched) when the SelectiveReconciliation
// feature gate is disabled or when the resource carries no ignore-field-drift
// annotation, so resources that do not use the feature are unaffected.
//
// This is exported so it can be called from generated per-resource delta code,
// which has access to the resource but not to the controller's config object.
// Such callers pass the process-wide feature gates via
// featuregate.GetGlobalFeatureGates().
func FilterIgnoredDeltaDifferences(
	delta *ackcompare.Delta,
	res acktypes.AWSResource,
	fg featuregate.FeatureGates,
) {
	if delta == nil {
		return
	}
	if !fg.IsEnabled(featuregate.SelectiveReconciliation) {
		return
	}
	ignorePaths := IgnoreFieldDriftPaths(res)
	if len(ignorePaths) == 0 {
		return
	}

	// Convert the JSON-style annotation paths (e.g. "spec.tags") to the
	// capitalized Go-field form the generated Delta paths use (e.g.
	// "Spec.Tags"), matching the convention used elsewhere in this package.
	deltaPaths := make([]string, 0, len(ignorePaths))
	for _, p := range ignorePaths {
		deltaPaths = append(deltaPaths, toDeltaPath(p))
	}

	filtered := delta.Differences[:0]
	for _, diff := range delta.Differences {
		ignored := false
		for _, dp := range deltaPaths {
			// Path.Contains reports whether the difference's path lies at or
			// under the ignored path (exact, case-sensitive segment match).
			if diff.Path.Contains(dp) {
				ignored = true
				break
			}
		}
		if !ignored {
			filtered = append(filtered, diff)
		}
	}
	delta.Differences = filtered
}

// logIgnoredFieldDrift emits a single log line naming any ignore-field-drift
// field whose value on the AWS resource (latest) differs from the user's
// declared value (desired) -- i.e. drift the user has asked the controller to
// leave alone. v1 is LOG-ONLY: no condition is set and the ACK.ResourceSynced
// condition is intentionally left untouched, since the user opted out of
// managing these fields.
//
// Drift is detected by comparing each ignored path directly between desired and
// latest. It deliberately does NOT use r.rd.Delta: the generated delta runs
// FilterIgnoredDeltaDifferences, which strips exactly these differences, so a
// filtered delta would never surface the drift we want to log.
func (r *resourceReconciler) logIgnoredFieldDrift(
	ctx context.Context,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) {
	ignorePaths := IgnoreFieldDriftPaths(desired)
	if len(ignorePaths) == 0 {
		return
	}

	rlog := ackrtlog.FromContext(ctx)

	d, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(desired.RuntimeObject())
	if err != nil {
		return
	}
	l, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(latest.RuntimeObject())
	if err != nil {
		return
	}

	drifted := []string{}
	for _, p := range ignorePaths {
		parts := pathParts(p)
		dv, dFound, _ := unstructured.NestedFieldNoCopy(d, parts...)
		lv, lFound, _ := unstructured.NestedFieldNoCopy(l, parts...)
		// Drift = the observed (latest) value differs from the declared
		// (desired) value at this ignored path.
		if dFound != lFound || !equalitySemanticDeepEqual(dv, lv) {
			drifted = append(drifted, p)
		}
	}

	if len(drifted) == 0 {
		return
	}

	sort.Strings(drifted)
	rlog.Info(
		"selective reconciliation: skipping drifted ignore-field-drift fields",
		"skipped", true,
		"fields", drifted,
	)
}

// ignoredFieldNeedsPersist reports whether any ignore-field-drift field has a
// value being newly ADDED on `target` (the to-be-persisted object) that is
// ABSENT on `base` (the patch base) and therefore must be written through to
// the CR spec.
//
// This is used by patchResourceMetadataAndSpec to decide whether to fire a
// patch even when the generated (filtered) delta reports no difference. The
// generated delta strips differences on ignored fields, which is correct for
// the Update/IsSynced decisions but would otherwise cause a genuinely new value
// produced by late-initialization (which fills a previously-unset field) to
// never be persisted.
//
// It fires ONLY for a path that is ABSENT in `base` but PRESENT in `target` —
// i.e. a value being newly added, the late-init "fill an unset field" case. It
// deliberately does NOT fire when an existing value merely DIFFERS between base
// and target: that is the "user edited a declared ignored field" case (the path
// is present on both sides), which the normal retain path owns. Force-persisting
// there would clobber the user's edit back to the AWS-observed value. A
// late-init resource carries the RESOURCE-level ACK.LateInitialized condition
// regardless of which field is late-init, so this present-on-both exclusion is
// what protects a declared-set ignored field (e.g. an edited spec.tags) on a
// late-init-configured resource.
//
// It deliberately does NOT use r.rd.Delta (which runs
// FilterIgnoredDeltaDifferences) and instead compares each ignored path
// directly via the unstructured converter, mirroring logIgnoredFieldDrift /
// restoreIgnoredFields. It returns false (no-op) when the gate is disabled, the
// resource carries no ignore-field-drift annotation, or either object cannot be
// converted to unstructured — so resources that do not use the feature, and the
// testify-mock-backed reconciler tests, are unaffected.
func (r *resourceReconciler) ignoredFieldNeedsPersist(
	base acktypes.AWSResource,
	target acktypes.AWSResource,
) bool {
	if !r.cfg.FeatureGates.IsEnabled(featuregate.SelectiveReconciliation) {
		return false
	}
	ignorePaths := IgnoreFieldDriftPaths(target)
	if len(ignorePaths) == 0 {
		return false
	}

	b, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(base.RuntimeObject())
	if err != nil {
		return false
	}
	t, err := k8srt.DefaultUnstructuredConverter.ToUnstructured(target.RuntimeObject())
	if err != nil {
		return false
	}

	for _, p := range ignorePaths {
		parts := pathParts(p)
		_, bFound, _ := unstructured.NestedFieldNoCopy(b, parts...)
		_, tFound, _ := unstructured.NestedFieldNoCopy(t, parts...)
		// A new value to persist exists ONLY when the ignored field is ABSENT in
		// the patch base but PRESENT in the to-be-persisted object — i.e.
		// late-init filled a previously-unset field. A path present on both sides
		// (even with a differing value) is the user-edited-declared-field case,
		// owned by the normal retain path; force-persisting it here would clobber
		// the user's edit back to the AWS value, so we do NOT fire for it.
		if !bFound && tFound {
			return true
		}
	}
	return false
}

// equalitySemanticDeepEqual compares two unstructured field values for
// equality, treating two nil values as equal.
func equalitySemanticDeepEqual(a, b interface{}) bool {
	return apiequality.Semantic.DeepEqual(a, b)
}

// toDeltaPath converts a JSON-style annotation path (e.g. "spec.tags") into the
// capitalized form used by the generated Delta paths (e.g. "Spec.Tags"). It
// title-cases the first letter of each dotted segment, which is sufficient for
// the top-level field paths these annotations target.
func toDeltaPath(p string) string {
	parts := strings.Split(p, ".")
	for i, seg := range parts {
		if seg == "" {
			continue
		}
		parts[i] = strings.ToUpper(seg[:1]) + seg[1:]
	}
	return strings.Join(parts, ".")
}
