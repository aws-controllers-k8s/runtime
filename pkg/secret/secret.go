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

package secret

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

// ResourceVersionLookup is a function that returns the resourceVersion of a
// Secret given a SecretKeyReference and the resource's namespace.
type ResourceVersionLookup func(context.Context, *ackv1alpha1.SecretKeyReference, string) (string, error)

// IndexKey returns the "namespace/name" key for a given SecretKeyReference,
// using the owning resource's namespace as fallback.
func IndexKey(ref *ackv1alpha1.SecretKeyReference, resourceNamespace string) string {
	if ref == nil {
		return ""
	}
	ns := ref.Namespace
	if ns == "" {
		ns = resourceNamespace
	}
	return ns + "/" + ref.Name
}

// SetResourceVersionsAnnotation looks up the current resourceVersion for each
// referenced Secret and stores them in the resource's annotation. This should
// be called in sdkFind after populating the resource object.
func SetResourceVersionsAnnotation(
	ctx context.Context,
	refs []*ackv1alpha1.SecretKeyReference,
	obj metav1.Object,
	lookupFn ResourceVersionLookup,
) {
	if len(refs) == 0 || obj == nil {
		return
	}

	versions := make(map[string]string, len(refs))
	resourceNS := obj.GetNamespace()
	for _, ref := range refs {
		if ref == nil || ref.Name == "" {
			continue
		}
		secretKey := IndexKey(ref, resourceNS)
		if _, ok := versions[secretKey]; ok {
			continue
		}
		version, err := lookupFn(ctx, ref, resourceNS)
		if err != nil || version == "" {
			continue
		}
		versions[secretKey] = version
	}

	if len(versions) == 0 {
		return
	}

	raw, err := json.Marshal(versions)
	if err != nil {
		return
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[ackv1alpha1.AnnotationSecretResourceVersions] = string(raw)
	obj.SetAnnotations(annotations)
}

// CopyAnnotation copies only the secret-resource-versions annotation from src
// to dst, preserving all other annotations on dst.
func CopyAnnotation(src, dst metav1.Object) {
	if src == nil || dst == nil {
		return
	}
	srcAnnotations := src.GetAnnotations()
	if srcAnnotations == nil {
		return
	}
	version, ok := srcAnnotations[ackv1alpha1.AnnotationSecretResourceVersions]
	if !ok {
		return
	}
	dstAnnotations := dst.GetAnnotations()
	if dstAnnotations == nil {
		dstAnnotations = make(map[string]string)
	}
	dstAnnotations[ackv1alpha1.AnnotationSecretResourceVersions] = version
	dst.SetAnnotations(dstAnnotations)
}

// ParseResourceVersions extracts the secret resource versions map from the
// annotation value.
func ParseResourceVersions(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}
	raw, ok := annotations[ackv1alpha1.AnnotationSecretResourceVersions]
	if !ok || raw == "" {
		return nil
	}
	var versions map[string]string
	if err := json.Unmarshal([]byte(raw), &versions); err != nil {
		return nil
	}
	return versions
}
