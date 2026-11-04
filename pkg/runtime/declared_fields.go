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
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// A controller generated with `unset_fields_unmanaged: true` in its
// generator.yaml treats a spec field the user never set as not managed by the
// resource: when AWS reports a value for it (typically a server-side default),
// that difference does not drive an Update. The setting reaches the runtime
// through the resource descriptor (AWSResourceDescriptorWithUnsetFields); it
// is chosen by the controller's maintainers, not by users.
//
// A nil field alone cannot tell "never set" from "removed by the user", so the
// controller keeps, in memory, which spec paths each CR set at its last
// successful sync (declaredFieldsStore). A difference whose desired side is
// unset is dropped only when its path is NOT in that record. A path that is in
// the record was set before, so the user removed it, and the difference is
// kept: removal behaves exactly as it does without the setting.
//
// The record is internal to the controller and is not persisted. When a CR
// has no entry, the controller can't tell what the user set before (after a
// controller restart or a leader change, or for a resource it did not create
// in this process), so that sync keeps today's behavior: no difference is
// dropped and the Update is sent. The entry is seeded from the current spec
// at that point, whatever the Update returns, so the next sync filters. The
// cost is one Update per such CR, on its first sync after a restart; a CR
// created by this process gets its entry from the create and pays nothing.

// declaredSpecPaths returns the dotted, JSON-style paths (relative to spec)
// of every field set on the resource's spec. It descends through structs only:
// list elements and map keys are not recorded, because the generated delta
// compares lists and maps as a whole. A typed resource is walked by
// reflection; an unstructured one by walking its spec map.
func declaredSpecPaths(res acktypes.AWSResource) []string {
	if ackcompare.IsNil(res) {
		return nil
	}
	obj := res.RuntimeObject()
	if u, ok := obj.(*unstructured.Unstructured); ok {
		spec, found, _ := unstructured.NestedMap(u.Object, "spec")
		if !found {
			return nil
		}
		var paths []string
		walkUnstructured(spec, "", &paths)
		sort.Strings(paths)
		return paths
	}
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	spec := v.FieldByName("Spec")
	if !spec.IsValid() {
		return nil
	}
	var paths []string
	walkTyped(spec, "", &paths)
	sort.Strings(paths)
	return paths
}

var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

// walkTyped appends the path of every set field reachable from v through
// structs. A field is set when it is a non-nil pointer, slice, map or
// interface, or a non-zero value of any other kind (the same rule as
// encoding/json's omitempty, except that an explicitly empty, non-nil list or
// map counts as set).
func walkTyped(v reflect.Value, prefix string, paths *[]string) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		// Skip unexported fields, except embedded structs, whose exported
		// fields encoding/json promotes into the parent.
		if sf.PkgPath != "" && !(sf.Anonymous && derefType(sf.Type).Kind() == reflect.Struct) {
			continue
		}
		name, inline := jsonFieldName(sf)
		if name == "-" {
			continue
		}
		fv := v.Field(i)
		if inline {
			walkTyped(fv, prefix, paths)
			continue
		}
		if !isSetValue(fv) {
			continue
		}
		p := name
		if prefix != "" {
			p = prefix + "." + name
		}
		*paths = append(*paths, p)
		if isLeafType(fv.Type()) {
			continue
		}
		walkTyped(fv, p, paths)
	}
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// jsonFieldName returns the JSON name of a struct field and whether the field
// is an embedded struct whose fields are inlined into the parent.
func jsonFieldName(sf reflect.StructField) (string, bool) {
	tag := sf.Tag.Get("json")
	name := strings.Split(tag, ",")[0]
	if sf.Anonymous && name == "" {
		return "", true
	}
	if name == "" {
		name = sf.Name
	}
	return name, false
}

// isSetValue reports whether a field value would be present in the object's
// JSON.
func isSetValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice:
		return !v.IsNil()
	default:
		return !v.IsZero()
	}
}

// isLeafType reports whether the walk should stop at a value of type t:
// lists, maps, scalars, and types with their own JSON encoding (for example
// metav1.Time) are leaves; only plain structs are descended into.
func isLeafType(t reflect.Type) bool {
	if t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType) {
		return true
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
		if t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType) {
			return true
		}
	}
	return t.Kind() != reflect.Struct
}

// walkUnstructured is the fallback for resources backed by unstructured
// objects. It cannot tell a struct from a map, so it descends into every map.
func walkUnstructured(m map[string]interface{}, prefix string, paths *[]string) {
	for k, v := range m {
		p := k
		if prefix != "" {
			p = prefix + "." + k
		}
		*paths = append(*paths, p)
		if sub, ok := v.(map[string]interface{}); ok {
			walkUnstructured(sub, p, paths)
		}
	}
}

// declaredKey returns the store key and UID of a resource.
func declaredKey(res acktypes.AWSResource) (types.NamespacedName, types.UID, bool) {
	if ackcompare.IsNil(res) || res.MetaObject() == nil {
		return types.NamespacedName{}, "", false
	}
	mo := res.MetaObject()
	return types.NamespacedName{Namespace: mo.GetNamespace(), Name: mo.GetName()}, mo.GetUID(), true
}

// declaredFieldsEntry returns the CR's entry. ok is false when the CR has
// none, or the entry belongs to an earlier object with the same name.
func (r *resourceReconciler) declaredFieldsEntry(res acktypes.AWSResource) (map[string]struct{}, bool) {
	key, uid, ok := declaredKey(res)
	if !ok {
		return nil, false
	}
	return r.declared.get(key, uid)
}

// seedDeclaredFields records the paths set on the CR right now. Called on a
// sync that found no entry, before its Update, so that the next sync filters
// even if this one fails.
func (r *resourceReconciler) seedDeclaredFields(res acktypes.AWSResource) {
	if key, uid, ok := declaredKey(res); ok {
		r.declared.set(key, uid, declaredSpecPaths(res))
	}
}

// isUnsetDeltaValue reports whether a value taken from a delta Difference is
// unset: nil, or a nil pointer, slice, map or interface. An explicitly empty,
// non-nil list or map is a value the user set.
func isUnsetDeltaValue(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice:
		return rv.IsNil()
	}
	return false
}

// filterUnsetUnmanagedDifferences removes, in place, every spec difference
// whose desired side (A) is unset and whose path the record does not contain.
// It returns the dropped paths for logging. Differences outside Spec, on tag
// fields, or on paths the user set before (a removal) are kept.
//
// unsetMeansEmpty lists the paths (Go field names, e.g. "Spec.Policy") whose
// differences are always kept.
func filterUnsetUnmanagedDifferences(
	delta *ackcompare.Delta,
	declared map[string]struct{},
	unsetMeansEmpty []string,
) []string {
	if delta == nil {
		return nil
	}
	var dropped []string
	kept := delta.Differences[:0]
	for _, diff := range delta.Differences {
		if diff != nil && isUnsetDeltaValue(diff.A) && diff.Path.Contains("Spec") &&
			!pathIsUnsetMeansEmpty(diff.Path, unsetMeansEmpty) && !pathInRecord(diff.Path, declared) {
			dropped = append(dropped, diff.Path.String())
			continue
		}
		kept = append(kept, diff)
	}
	delta.Differences = kept
	return dropped
}

func pathInRecord(p ackcompare.Path, declared map[string]struct{}) bool {
	for d := range declared {
		if p.EqualFold("Spec." + d) {
			return true
		}
	}
	return false
}

func pathIsUnsetMeansEmpty(p ackcompare.Path, unsetMeansEmpty []string) bool {
	for _, optedOut := range unsetMeansEmpty {
		// A difference at or under an opted-out field is kept.
		if p.ContainsFold(optedOut) {
			return true
		}
	}
	return false
}

// unsetFieldsUnmanaged reports whether the controller was generated with
// `unset_fields_unmanaged: true`.
func (r *resourceReconciler) unsetFieldsUnmanaged() bool {
	d, ok := r.rd.(acktypes.AWSResourceDescriptorWithUnsetFields)
	return ok && d.UnsetFieldsUnmanaged()
}

// unsetMeansEmptyPaths returns the opted-out paths from the descriptor.
func (r *resourceReconciler) unsetMeansEmptyPaths() []string {
	d, ok := r.rd.(acktypes.AWSResourceDescriptorWithUnsetFields)
	if !ok {
		return nil
	}
	return d.UnsetMeansEmptyPaths()
}

// nextDeclaredPaths returns the record to store after a successful sync: the
// paths set on `declared`, plus every previously recorded path that the user
// removed but AWS still reports in `observed` (the ReadOne result from before
// any Update). Keeping such a pending removal in the record keeps its
// difference visible on the next sync, so removal behaves exactly as it does
// without the gate until AWS no longer reports the value. Without this, a
// removal that the generated Update cannot apply would be tried once and then
// silently treated as unmanaged.
//
// previous is the CR's current entry, or nil when it has none.
func nextDeclaredPaths(declared, observed acktypes.AWSResource, previous map[string]struct{}) []string {
	now := declaredSpecPaths(declared)
	if previous == nil || ackcompare.IsNil(observed) {
		return now
	}
	set := map[string]struct{}{}
	for _, p := range now {
		set[p] = struct{}{}
	}
	for _, p := range declaredSpecPaths(observed) {
		if _, wasDeclared := previous[p]; wasDeclared {
			set[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// recordDeclaredFields updates the CR's entry after a successful Sync. It
// only touches controller memory.
func (r *resourceReconciler) recordDeclaredFields(declared, observed acktypes.AWSResource) {
	if !r.unsetFieldsUnmanaged() {
		return
	}
	key, uid, ok := declaredKey(declared)
	if !ok {
		return
	}
	previous, _ := r.declared.get(key, uid)
	r.declared.set(key, uid, nextDeclaredPaths(declared, observed, previous))
}

// forgetDeclaredFields drops the CR's entry when it is deleted.
func (r *resourceReconciler) forgetDeclaredFields(key types.NamespacedName) {
	r.declared.forget(key)
}
