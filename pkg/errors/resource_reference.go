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

package errors

import (
	"fmt"
	"strings"
)

var (
	// ResourceReferenceOrIDRequired indicates that the user failed to specify
	// either the actual referenced identifier or an AWSResourceReferenceWrapper
	ResourceReferenceOrIDRequired = fmt.Errorf(
		"resource reference wrapper or ID required",
	)
	// ResourceReferenceAndIDNotSupported indicates that the user specified
	// both the actual referenced identifier and an AWSResourceReferenceWrapper
	ResourceReferenceAndIDNotSupported = fmt.Errorf(
		"both resource reference wrapper and ID cannot be used together",
	)
	// ResourceReferenceTerminal indicates that the resource referred from
	// AWSResourceReferenceWrapper is in Terminal state and cannot be referred
	ResourceReferenceTerminal = fmt.Errorf(
		"the referenced resource has 'ACK.Terminal' condition 'True'." +
			" Cannot be referenced",
	)
	// ResourceReferenceNotSynced indicates that the resource referred from
	// AWSResourceReferenceWrapper is still being reconciled and cannot be
	// referred
	ResourceReferenceNotSynced = fmt.Errorf(
		"the referenced resource is not synced yet",
	)
	// ResourceReferenceMissingTargetField indicates that the resource referred
	// from AWSResourceReferenceWrapper does not contain the target field which
	// needs to be referred
	ResourceReferenceMissingTargetField = fmt.Errorf(
		"the referenced resource is missing the target field",
	)
)

// ResourceReferenceOrIDRequiredFor returns a ResourceReferenceOrIDRequired error
// for one or more supplied fields
func ResourceReferenceOrIDRequiredFor(fields ...string) error {
	return fmt.Errorf("%w: %s", ResourceReferenceOrIDRequired,
		strings.Join(fields, ","))
}

// ResourceReferenceAndIDNotSupportedFor returns a ResourceReferenceAndIDNotSupported
// error for one or more supplied fields
func ResourceReferenceAndIDNotSupportedFor(fields ...string) error {
	return fmt.Errorf("%w: %s", ResourceReferenceAndIDNotSupported,
		strings.Join(fields, ","))
}

// ResourceReferenceTerminalFor returns a ResourceReferenceTerminal for supplied
// resource
func ResourceReferenceTerminalFor(resource string, namespace string,
	name string,
) error {
	return fmt.Errorf("%w. resource:%s, namespace:%s, name:%s",
		ResourceReferenceTerminal, resource, namespace, name)
}

// ResourceReferenceNotSyncedFor returns a ResourceReferenceNotSynced for
// supplied resource
func ResourceReferenceNotSyncedFor(resource string, namespace string,
	name string,
) error {
	return fmt.Errorf("%w. resource:%s, namespace:%s, name:%s",
		ResourceReferenceNotSynced, resource, namespace, name)
}

// ResourceReferenceMissingTargetFieldFor returns a ResourceReferenceMissingTargetField
// for supplied resource
func ResourceReferenceMissingTargetFieldFor(resource string, namespace string,
	name string, targetField string,
) error {
	return fmt.Errorf("%w. resource:%s, namespace:%s, name:%s"+
		", targetField:%s", ResourceReferenceMissingTargetField,
		resource, namespace, name, targetField)
}
