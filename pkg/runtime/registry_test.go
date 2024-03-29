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

package runtime_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ackrt "github.com/aws-controllers-k8s/runtime/pkg/runtime"

	mocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
)

func TestRegistry(t *testing.T) {
	require := require.New(t)

	rd := &mocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(
		schema.GroupVersionKind{
			Group: "bookstore.services.k8s.aws",
			Kind:  "Book",
		},
	)

	rmf := &mocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)

	reg := ackrt.NewRegistry()

	rmfs := reg.GetResourceManagerFactories()
	require.Empty(rmfs)

	reg.RegisterResourceManagerFactory(rmf)
	rmfs = reg.GetResourceManagerFactories()
	require.NotEmpty(rmfs)
	require.Contains(rmfs, rmf)

	rmf.AssertCalled(t, "ResourceDescriptor")
	rd.AssertCalled(t, "GroupVersionKind")
}
