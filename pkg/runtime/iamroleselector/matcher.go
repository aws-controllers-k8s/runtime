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

package iamroleselector

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

// MatchContext contains the attributes to match against an IAMRoleSelector
type MatchContext struct {
	Namespace       string
	NamespaceLabels map[string]string
	GVK             schema.GroupVersionKind
	ResourceLabels  map[string]string
}

// Matches checks if a selector matches the given context
// Rules: AND between different field types, OR within arrays
func Matches(selector *ackv1alpha1.IAMRoleSelector, ctx MatchContext) bool {
	// All conditions must match (AND logic between different selectors)
	return matchesNamespace(selector.Spec.NamespaceSelector, ctx.Namespace, ctx.NamespaceLabels) &&
		matchesResourceType(selector.Spec.ResourceTypeSelector, ctx.GVK) &&
		matchesLabels(selector.Spec.ResourceLabelSelector, ctx.ResourceLabels)
}

// matchesLabels checks if the label selector matches the given resource labels
func matchesLabels(labelSelector ackv1alpha1.LabelSelector, resourceLabels map[string]string) bool {
	// If no label selector specified, matches all resources
	if len(labelSelector.MatchLabels) == 0 {
		return true
	}

	// Check if all specified labels match (AND logic within label selector)
	selector := labels.SelectorFromSet(labelSelector.MatchLabels)
	return selector.Matches(labels.Set(resourceLabels))
}

// matchesNamespace checks if the namespace selector matches the given namespace and its labels
func matchesNamespace(nsSelector ackv1alpha1.NamespaceSelector, namespace string, namespaceLabels map[string]string) bool {
	// If no namespace selector specified, matches all namespaces
	if len(nsSelector.Names) == 0 && len(nsSelector.LabelSelector.MatchLabels) == 0 {
		return true
	}

	// Check if namespace name matches (OR within the names array)
	nameMatches := false
	if len(nsSelector.Names) > 0 {
		for _, ns := range nsSelector.Names {
			if ns == namespace {
				nameMatches = true
				break
			}
		}
		// If names are specified but none match, and we have label selectors,
		// the namespace must be in the names list
		if !nameMatches {
			return false
		}
	}

	// Check label selector (AND with name match)
	if len(nsSelector.LabelSelector.MatchLabels) > 0 {
		labelSelector := labels.SelectorFromSet(nsSelector.LabelSelector.MatchLabels)
		if !labelSelector.Matches(labels.Set(namespaceLabels)) {
			return false
		}
	}

	// If we get here:
	// - Either no names were specified, or the namespace is in the names list
	// - Either no labels were specified, or the labels match
	return true
}

func matchesResourceType(rtSelectors []ackv1alpha1.GroupVersionKind, gvk schema.GroupVersionKind) bool {
	// If no resource type selector specified, matches all resources
	if len(rtSelectors) == 0 {
		return true
	}

	// OR within the array - any selector can match
	for _, rts := range rtSelectors {
		groupMatches := rts.Group == "" || rts.Group == gvk.Group
		versionMatches := rts.Version == "" || rts.Version == gvk.Version
		kindMatches := rts.Kind == "" || rts.Kind == gvk.Kind

		// All specified fields must match (AND logic)
		if groupMatches && versionMatches && kindMatches {
			return true
		}
	}

	// If we get here, no selectors matched
	return false
}

// validateSelector checks if an IAMRoleSelector is valid
func validateSelector(selector *ackv1alpha1.IAMRoleSelector) error {
	if selector == nil {
		return fmt.Errorf("selector cannot be nil")
	}

	if selector.Spec.ARN == "" {
		return fmt.Errorf("ARN cannot be empty")
	}

	// parse ARN to ensure it's valid
	if _, err := arn.Parse(selector.Spec.ARN); err != nil {
		return fmt.Errorf("invalid ARN: %w", err)
	}

	// Validate namespace selector
	if err := validateNamespaceSelector(selector.Spec.NamespaceSelector); err != nil {
		return fmt.Errorf("invalid namespace selector: %w", err)
	}

	// Validate resource type selectors
	if err := validateResourceTypeSelectors(selector.Spec.ResourceTypeSelector); err != nil {
		return fmt.Errorf("invalid resource type selector: %w", err)
	}

	// Validate label selector
	if err := validateLabelSelector(selector.Spec.ResourceLabelSelector); err != nil {
		return fmt.Errorf("invalid label selector: %w", err)
	}

	return nil
}

func validateNamespaceSelector(nsSelector ackv1alpha1.NamespaceSelector) error {
	// Check for duplicate namespace names
	seen := make(map[string]bool)
	for _, name := range nsSelector.Names {
		if name == "" {
			return fmt.Errorf("namespace name cannot be empty")
		}
		if seen[name] {
			return fmt.Errorf("duplicate namespace name: %s", name)
		}
		seen[name] = true
	}

	// Validate label selector
	return validateLabelSelector(nsSelector.LabelSelector)
}

// validateLabelSelector checks that the label selector has valid label keys
func validateLabelSelector(labelSelector ackv1alpha1.LabelSelector) error {
	if len(labelSelector.MatchLabels) > 0 {
		for key := range labelSelector.MatchLabels {
			if key == "" {
				return fmt.Errorf("label key cannot be empty")
			}
			// Kubernetes label values can be empty, so we don't validate value
		}
	}
	return nil
}

// validateResourceTypeSelectors checks that each resource type selector has at least one field specified
// and that there are no duplicate selectors
func validateResourceTypeSelectors(rtSelectors []ackv1alpha1.GroupVersionKind) error {
	seen := make(map[string]bool)

	for i, rts := range rtSelectors {
		// at least one field must be specified
		if rts.Group == "" && rts.Version == "" && rts.Kind == "" {
			return fmt.Errorf("at least one of group, version, or kind must be specified at index %d", i)
		}

		// check for duplicates
		key := fmt.Sprintf("%s/%s/%s", rts.Group, rts.Version, rts.Kind)
		if seen[key] {
			return fmt.Errorf("duplicate resource type selector: %s", key)
		}
		seen[key] = true
	}

	return nil
}
