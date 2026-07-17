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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentifierFieldsFromARNPositional(t *testing.T) {
	require := require.New(t)

	// Single-ID resource (VPC-like): last segment after the type label.
	fields, err := IdentifierFieldsFromARNPositional(
		"arn:aws:ec2:us-west-2:123456789012:vpc/vpc-abc123",
		"vpc",
		[]string{"vpcID"},
	)
	require.NoError(err)
	require.Equal(map[string]string{"vpcID": "vpc-abc123"}, fields)

	// Multi-ID resource (Nodegroup-like): two keys, trailing uuid ignored.
	fields, err = IdentifierFieldsFromARNPositional(
		"arn:aws:eks:us-west-2:123456789012:nodegroup/my-cluster/ng-1/abc-uuid",
		"nodegroup",
		[]string{"clusterName", "name"},
	)
	require.NoError(err)
	require.Equal(map[string]string{"clusterName": "my-cluster", "name": "ng-1"}, fields)

	// Type label absent from ARN: segments consumed as-is.
	fields, err = IdentifierFieldsFromARNPositional(
		"arn:aws:service:us-west-2:123456789012:the-id",
		"widget",
		[]string{"id"},
	)
	require.NoError(err)
	require.Equal(map[string]string{"id": "the-id"}, fields)

	// Not enough segments -> error.
	_, err = IdentifierFieldsFromARNPositional(
		"arn:aws:eks:us-west-2:123456789012:nodegroup/only-one",
		"nodegroup",
		[]string{"clusterName", "name"},
	)
	require.Error(err)

	// Malformed ARN -> error.
	_, err = IdentifierFieldsFromARNPositional(
		"not-an-arn",
		"vpc",
		[]string{"vpcID"},
	)
	require.Error(err)
}

func TestIdentifierFieldsFromARNTemplate(t *testing.T) {
	require := require.New(t)

	// Discard trailing uuid, capture two keys.
	fields, err := IdentifierFieldsFromARNTemplate(
		"arn:aws:eks:us-west-2:123456789012:nodegroup/my-cluster/ng-1/abc-uuid",
		"nodegroup/{clusterName}/{name}/{-}",
	)
	require.NoError(err)
	require.Equal(map[string]string{"clusterName": "my-cluster", "name": "ng-1"}, fields)

	// Literal token mismatch -> error.
	_, err = IdentifierFieldsFromARNTemplate(
		"arn:aws:eks:us-west-2:123456789012:cluster/my-cluster/ng-1/uuid",
		"nodegroup/{clusterName}/{name}/{-}",
	)
	require.Error(err)

	// Segment count mismatch -> error.
	_, err = IdentifierFieldsFromARNTemplate(
		"arn:aws:eks:us-west-2:123456789012:nodegroup/my-cluster",
		"nodegroup/{clusterName}/{name}/{-}",
	)
	require.Error(err)
}
