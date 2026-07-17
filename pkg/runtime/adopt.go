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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

// arnResourceSplit splits the resource portion of an ARN into its segments.
// ARNs delimit the resource portion with either '/' or ':' (and sometimes a
// mix, e.g. "db:my-instance" or "nodegroup/cluster/ng/uuid"), so we split on
// both.
func arnResourceSplit(resource string) []string {
	return strings.FieldsFunc(resource, func(r rune) bool {
		return r == '/' || r == ':'
	})
}

// IdentifierFieldsFromARNPositional derives the ReadOne identifier fields of a
// resource from its ARN by mapping the ARN's resource-segment values
// positionally onto orderedKeys.
//
// resourceTypeLabel is the expected leading label of the ARN's resource portion
// (e.g. "nodegroup" for an EKS Nodegroup, "vpc" for an EC2 VPC). When the first
// resource segment equals this label it is stripped before mapping, so it does
// not consume an identifier key. orderedKeys are the identifier field keys, in
// the order they map onto the remaining value segments; these are the same keys
// (and order) that PopulateResourceFromAnnotation consumes. Extra trailing
// segments (e.g. an EKS Nodegroup's unique-id) are ignored.
//
// It returns an error if the ARN cannot be parsed or does not carry enough
// value segments to fill every key.
func IdentifierFieldsFromARNPositional(
	arnStr string,
	resourceTypeLabel string,
	orderedKeys []string,
) (map[string]string, error) {
	parsed, err := arn.Parse(arnStr)
	if err != nil {
		return nil, fmt.Errorf("parsing ARN %q: %v", arnStr, err)
	}

	segments := arnResourceSplit(parsed.Resource)
	// Strip the leading resource-type label if present, so it does not consume
	// an identifier key.
	if resourceTypeLabel != "" && len(segments) > 0 &&
		strings.EqualFold(segments[0], resourceTypeLabel) {
		segments = segments[1:]
	}

	if len(segments) < len(orderedKeys) {
		return nil, fmt.Errorf(
			"ARN %q does not encode enough segments to derive identifier fields %v",
			arnStr, orderedKeys,
		)
	}

	fields := make(map[string]string, len(orderedKeys))
	for i, key := range orderedKeys {
		fields[key] = segments[i]
	}
	return fields, nil
}

// IdentifierFieldsFromARNTemplate derives the ReadOne identifier fields of a
// resource from its ARN using an explicit template matched against the ARN's
// resource portion. The template is split on '/' and ':' into tokens; each
// token is either a literal (which must match the corresponding ARN segment), a
// "{key}" capture (which stores the segment under "key"), or a "{-}" discard
// (which matches any segment without capturing it). This is the override used
// for the rare ARNs whose identifier cannot be derived positionally.
//
// It returns an error if the ARN cannot be parsed, its segment count does not
// match the template, or a literal token does not match.
func IdentifierFieldsFromARNTemplate(
	arnStr string,
	template string,
) (map[string]string, error) {
	parsed, err := arn.Parse(arnStr)
	if err != nil {
		return nil, fmt.Errorf("parsing ARN %q: %v", arnStr, err)
	}

	tmplTokens := arnResourceSplit(template)
	segments := arnResourceSplit(parsed.Resource)
	if len(tmplTokens) != len(segments) {
		return nil, fmt.Errorf(
			"ARN %q resource segments (%d) do not match template %q (%d tokens)",
			arnStr, len(segments), template, len(tmplTokens),
		)
	}

	fields := map[string]string{}
	for i, token := range tmplTokens {
		switch {
		case token == "{-}" || token == "{_}":
			// discard
		case strings.HasPrefix(token, "{") && strings.HasSuffix(token, "}"):
			key := token[1 : len(token)-1]
			fields[key] = segments[i]
		default:
			// literal token must match
			if token != segments[i] {
				return nil, fmt.Errorf(
					"ARN %q segment %q does not match template literal %q",
					arnStr, segments[i], token,
				)
			}
		}
	}
	return fields, nil
}
