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
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
)

// ValidateCrossNamespaceReference inspects a user-supplied reference
// namespace and decides which namespace the controller should use when
// resolving the reference.
//
// Parameters:
//   - enableCrossNamespace: the value of Config.EnableCrossNamespace
//   - ownerNamespace: the namespace of the resource containing the
//     reference (never empty)
//   - refNamespace: the Namespace field on the AWSResourceReference
//     (may be nil or empty)
//   - refName: the Name field on the AWSResourceReference; used only for
//     error message context
//
// Return values:
//   - resolvedNamespace: the namespace to pass to apiReader.Get
//   - isCrossNamespace: true if the reference targets a different namespace
//     (callers should emit a deprecation warning when flag=true)
//   - err: nil on success; a ResourceReferenceCrossNamespaceNotAllowed
//     error when cross-namespace refs are disabled and the reference
//     targets a different namespace
//
// Same-namespace behavior (nil, empty, or equal refNamespace) is
// unaffected by the flag.
func ValidateCrossNamespaceReference(
	enableCrossNamespace bool,
	ownerNamespace string,
	refNamespace *string,
	refName string,
) (string, bool, error) {
	// Nil or empty: always use owner namespace.
	if refNamespace == nil || *refNamespace == "" {
		return ownerNamespace, false, nil
	}
	// Same namespace: always allowed, not cross-namespace.
	if *refNamespace == ownerNamespace {
		return ownerNamespace, false, nil
	}
	// Differing namespace: gated by CLI flag.
	if enableCrossNamespace {
		return *refNamespace, true, nil
	}
	return "", false, ackerr.ResourceReferenceCrossNamespaceNotAllowedFor(
		ownerNamespace, *refNamespace, refName,
	)
}

// ValidateCrossNamespaceReferenceString is a convenience wrapper for callers
// that have a string namespace (e.g., SecretKeyReference.Namespace,
// FieldExportTarget.Namespace) rather than a *string.
func ValidateCrossNamespaceReferenceString(
	enableCrossNamespace bool,
	ownerNamespace string,
	refNamespace string,
	refName string,
) (string, bool, error) {
	if refNamespace == "" {
		return ownerNamespace, false, nil
	}
	return ValidateCrossNamespaceReference(
		enableCrossNamespace, ownerNamespace, &refNamespace, refName,
	)
}
