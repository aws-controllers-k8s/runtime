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

package compare

import (
	"encoding/json"
	"fmt"

	awsiampolicy "github.com/micahhausler/aws-iam-policy/policy"
)

// IAMPolicyDocumentEqual returns true if two IAM policy document JSON strings
// are semantically equivalent. This handles IAM-specific semantics like:
//   - Statement ordering independence
//   - Action/Resource as string or array ("s3:Get*" vs ["s3:Get*"])
//   - Principal variations ("*" vs {"AWS": "*"})
//   - Condition value comparisons
//
// Returns an error if either string cannot be parsed as a valid IAM policy document.
func IAMPolicyDocumentEqual(desired, latest string) (bool, error) {
	if desired == latest {
		return true, nil
	}

	var desiredPolicy, latestPolicy awsiampolicy.Policy

	if err := json.Unmarshal([]byte(desired), &desiredPolicy); err != nil {
		return false, fmt.Errorf("failed to parse desired policy document: %w", err)
	}
	if err := json.Unmarshal([]byte(latest), &latestPolicy); err != nil {
		return false, fmt.Errorf("failed to parse latest policy document: %w", err)
	}

	return desiredPolicy.Equal(&latestPolicy), nil
}
