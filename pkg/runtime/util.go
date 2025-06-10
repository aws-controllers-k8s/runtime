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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// TODO(jaypipes): Place this code somewhere separate
//     (michaelhtm)             ^ +1

// AdoptionPolicy stores adoptionPolicy values we expect users to
// provide in the resources `adoption-policy` annotation
//
// TODO(michaelhtm) Maybe we need a different place for this...
// next refactor maybe? 🤷‍♂️
type AdoptionPolicy string

const (
	// AdoptPolicy is ...
	AdoptionPolicy_Adopt AdoptionPolicy = "adopt"
	// AdoptPolicy is ...
	AdoptionPolicy_AdoptOrCreate AdoptionPolicy = "adopt-or-create"
)

// IsAdopted returns true if the supplied AWSResource was created with a
// non-nil ARN annotation, which indicates that the Kubernetes user who created
// the CR for the resource expects the ACK service controller to "adopt" a
// pre-existing resource and bring it under ACK management.
func IsAdopted(res acktypes.AWSResource) bool {
	mo := res.MetaObject()
	if mo == nil {
		// Should never happen... if it does, it's buggy code.
		panic("IsAdopted received resource with nil RuntimeObject")
	}
	for k, v := range mo.GetAnnotations() {
		if k == ackv1alpha1.AnnotationAdopted {
			return strings.ToLower(v) == "true"
		}
	}
	return false
}

// IsSynced returns true if the supplied AWSResource's CR and associated
// backend AWS service API resource are in sync.
func IsSynced(res acktypes.AWSResource) bool {
	for _, c := range res.Conditions() {
		if c.Type == ackv1alpha1.ConditionTypeResourceSynced {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// IsReadOnly returns true if the supplied AWSResource has an annotation
// indicating that it is in read-only mode.
func IsReadOnly(res acktypes.AWSResource) bool {
	mo := res.MetaObject()
	if mo == nil {
		// Should never happen... if it does, it's buggy code.
		panic("IsReadOnly received resource with nil RuntimeObject")
	}
	for k, v := range mo.GetAnnotations() {
		if k == ackv1alpha1.AnnotationReadOnly {
			return strings.ToLower(v) == "true"
		}
	}
	return false
}

// GetAdoptionPolicy returns the Adoption Policy of the resource
// defined by the user in annotation. Possible values are:
// adopt-only | adopt-or-create
// adopt-only keeps requing until the resource is found
// adopt-or-create creates the resource if does not exist
func GetAdoptionPolicy(res acktypes.AWSResource) (AdoptionPolicy, error) {
	mo := res.MetaObject()
	if mo == nil {
		panic("getAdoptionPolicy received resource with nil RuntimeObject")
	}
	policy, ok := mo.GetAnnotations()[ackv1alpha1.AnnotationAdoptionPolicy]
	if !ok {
		return "", nil
	}

	if policy != string(AdoptionPolicy_Adopt) && policy != string(AdoptionPolicy_AdoptOrCreate) {
		return "", fmt.Errorf("unrecognized adoption policy")
	}

	return AdoptionPolicy(policy), nil
}

// NeedAdoption returns true when the resource has
// adopt annotation but is not yet adopted
func NeedAdoption(res acktypes.AWSResource) bool {
	adoptionPolicy, _ := GetAdoptionPolicy(res)
	return adoptionPolicy != "" && !IsAdopted(res)
}

func ExtractAdoptionFields(res acktypes.AWSResource) (map[string]string, error) {
	fields := getAdoptionFields(res)

	extractedFields := &map[string]string{}
	err := json.Unmarshal([]byte(fields), extractedFields)
	if err != nil {
		return nil, err
	}

	return *extractedFields, nil
}

func getAdoptionFields(res acktypes.AWSResource) string {
	mo := res.MetaObject()
	if mo == nil {
		// Should never happen... if it does, it's buggy code.
		panic("ExtractRequiredFields received resource with nil RuntimeObject")
	}

	for k, v := range mo.GetAnnotations() {
		if k == ackv1alpha1.AnnotationAdoptionFields {
			return v
		}
	}
	return ""
}

// patchWithoutCancel performs a patch operation using context.WithoutCancel to prevent
// patch operations from being cancelled while preserving context values.
//
// NOTE(rushmash91): The 30s SIGTERM grace period acts as the effective timeout -
// no additional timeout needed to avoid interfering with normal Kubernetes client
// timeout/retry strategy.
func patchWithoutCancel(
	ctx context.Context,
	kc client.Client,
	obj client.Object,
	patch client.Patch,
) error {
	patchCtx := context.WithoutCancel(ctx)
	return kc.Patch(patchCtx, obj, patch)
}

// patchStatusWithoutCancel performs a status patch operation using context.WithoutCancel
// to prevent patch operations from being cancelled while preserving context values.
//
// NOTE(rushmash91): The 30s SIGTERM grace period acts as the effective timeout -
// no additional timeout needed to avoid interfering with normal Kubernetes client
// timeout/retry strategy.
func patchStatusWithoutCancel(
	ctx context.Context,
	kc client.Client,
	obj client.Object,
	patch client.Patch,
) error {
	patchCtx := context.WithoutCancel(ctx)
	return kc.Status().Patch(patchCtx, obj, patch)
}
