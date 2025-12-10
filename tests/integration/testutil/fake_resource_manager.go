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

package testutil

import (
	"context"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// FakeResourceManager implements AWSResourceManager for testing.
// It stores resources in memory instead of calling AWS APIs.
type FakeResourceManager struct {
	mu        sync.RWMutex
	resources map[string]acktypes.AWSResource // keyed by ARN

	// Hooks for test customization
	OnCreate  func(context.Context, acktypes.AWSResource) (acktypes.AWSResource, error)
	OnReadOne func(context.Context, acktypes.AWSResource) (acktypes.AWSResource, error)
	OnUpdate  func(context.Context, acktypes.AWSResource, acktypes.AWSResource, *ackcompare.Delta) (acktypes.AWSResource, error)
	OnDelete  func(context.Context, acktypes.AWSResource) (acktypes.AWSResource, error)

	// Call tracking for assertions
	CreateCalls  []acktypes.AWSResource
	ReadCalls    []acktypes.AWSResource
	UpdateCalls  []UpdateCall
	DeleteCalls  []acktypes.AWSResource

	// Operation history tracks all operations in order
	Operations []Operation
}

type Operation struct {
	Type string // "Create", "ReadOne", "Update", "Delete"
	Resource acktypes.AWSResource
	Delta *ackcompare.Delta // only for Update
}

type UpdateCall struct {
	Desired acktypes.AWSResource
	Latest  acktypes.AWSResource
	Delta   *ackcompare.Delta
}

func NewFakeResourceManager() *FakeResourceManager {
	return &FakeResourceManager{
		resources: make(map[string]acktypes.AWSResource),
	}
}

// SeedResource pre-populates the fake AWS backend with a resource
func (f *FakeResourceManager) SeedResource(arn string, res acktypes.AWSResource) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources[arn] = res
}

// GetResourceByARN retrieves a resource from the fake backend
func (f *FakeResourceManager) GetResourceByARN(arn string) (acktypes.AWSResource, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	res, ok := f.resources[arn]
	return res, ok
}

// ReadOne implements AWSResourceManager
func (f *FakeResourceManager) ReadOne(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error) {
	f.mu.Lock()
	f.ReadCalls = append(f.ReadCalls, res)
	f.Operations = append(f.Operations, Operation{Type: "ReadOne", Resource: res})
	f.mu.Unlock()

	if f.OnReadOne != nil {
		return f.OnReadOne(ctx, res)
	}

	// Default behavior: look up by ARN
	arn := res.Identifiers().ARN()
	if arn == nil {
		return nil, ackerr.NotFound
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	stored, exists := f.resources[string(*arn)]
	if !exists {
		return nil, ackerr.NotFound
	}

	return stored.DeepCopy(), nil
}

// Create implements AWSResourceManager
func (f *FakeResourceManager) Create(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error) {
	f.mu.Lock()
	f.CreateCalls = append(f.CreateCalls, res)
	f.Operations = append(f.Operations, Operation{Type: "Create", Resource: res})
	f.mu.Unlock()

	if f.OnCreate != nil {
		return f.OnCreate(ctx, res)
	}

	// Default behavior: generate ARN and store
	name := res.MetaObject().GetName()
	namespace := res.MetaObject().GetNamespace()
	arn := ackv1alpha1.AWSResourceName(fmt.Sprintf("arn:aws:test:us-west-2:123456789012:book/%s/%s", namespace, name))

	created := res.DeepCopy()
	// Set the ARN in identifiers (implementation specific to your resource type)
	identifiers := &ackv1alpha1.AWSIdentifiers{
		ARN: &arn,
	}
	_ = created.SetIdentifiers(identifiers)

	f.mu.Lock()
	f.resources[string(arn)] = created
	f.mu.Unlock()

	return created, nil
}

// Update implements AWSResourceManager
func (f *FakeResourceManager) Update(
	ctx context.Context,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
	delta *ackcompare.Delta,
) (acktypes.AWSResource, error) {
	f.mu.Lock()
	f.UpdateCalls = append(f.UpdateCalls, UpdateCall{
		Desired: desired,
		Latest:  latest,
		Delta:   delta,
	})
	f.Operations = append(f.Operations, Operation{Type: "Update", Resource: desired, Delta: delta})
	f.mu.Unlock()

	if f.OnUpdate != nil {
		return f.OnUpdate(ctx, desired, latest, delta)
	}

	// Default behavior: update stored resource
	arn := latest.Identifiers().ARN()
	if arn == nil {
		return nil, ackerr.NotFound
	}

	updated := desired.DeepCopy()
	updated.SetStatus(latest)

	f.mu.Lock()
	f.resources[string(*arn)] = updated
	f.mu.Unlock()

	return updated, nil
}

// Delete implements AWSResourceManager
func (f *FakeResourceManager) Delete(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error) {
	f.mu.Lock()
	f.DeleteCalls = append(f.DeleteCalls, res)
	f.Operations = append(f.Operations, Operation{Type: "Delete", Resource: res})
	f.mu.Unlock()

	if f.OnDelete != nil {
		return f.OnDelete(ctx, res)
	}

	// Default behavior: remove from storage
	arn := res.Identifiers().ARN()
	if arn != nil {
		f.mu.Lock()
		delete(f.resources, string(*arn))
		f.mu.Unlock()
	}

	return res, nil
}

// ARNFromName implements AWSResourceManager
func (f *FakeResourceManager) ARNFromName(name string) string {
	return fmt.Sprintf("arn:aws:test:us-west-2:123456789012:book/%s", name)
}

// LateInitialize implements AWSResourceManager
func (f *FakeResourceManager) LateInitialize(ctx context.Context, res acktypes.AWSResource) (acktypes.AWSResource, error) {
	// Default: no late initialization
	return res, nil
}

// IsSynced implements AWSResourceManager
func (f *FakeResourceManager) IsSynced(ctx context.Context, res acktypes.AWSResource) (bool, error) {
	// Default: always synced
	return true, nil
}

// EnsureTags implements AWSResourceManager
func (f *FakeResourceManager) EnsureTags(ctx context.Context, res acktypes.AWSResource, metadata acktypes.ServiceControllerMetadata) error {
	// Default: no-op
	return nil
}

// FilterSystemTags implements AWSResourceManager
func (f *FakeResourceManager) FilterSystemTags(res acktypes.AWSResource, keys []string) {
	// Default: no-op
}

// ResolveReferences implements ReferenceManager
func (f *FakeResourceManager) ResolveReferences(ctx context.Context, reader client.Reader, res acktypes.AWSResource) (acktypes.AWSResource, bool, error) {
	// Default: no references to resolve
	return res, false, nil
}

// ClearResolvedReferences implements ReferenceManager
func (f *FakeResourceManager) ClearResolvedReferences(res acktypes.AWSResource) acktypes.AWSResource {
	// Default: return as-is
	return res
}

// Reset clears all state - useful between tests
func (f *FakeResourceManager) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources = make(map[string]acktypes.AWSResource)
	f.CreateCalls = nil
	f.ReadCalls = nil
	f.UpdateCalls = nil
	f.DeleteCalls = nil
	f.Operations = nil
}
