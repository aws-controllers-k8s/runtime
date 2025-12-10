# Integration Tests

Separate Go module for integration tests with Ginkgo + envtest.

## Structure

```
tests/integration/
  core/                  # Core reconciliation tests (manual Reconcile)
    suite_test.go       # envtest setup, Table CRD
    reconciler_test.go  # CREATE/UPDATE/DELETE tests

  testutil/             # Test utilities
    scenarios.go        # Behavior framework for error scenarios
    fake_resource_manager.go  # In-memory AWS backend
    table_resource.go   # Test DynamoDB Table resource
    test_builders.go    # Fake descriptor/controller builders

  go.mod               # Separate module (keeps dependencies isolated)
```

## Setup

Install envtest binaries:

```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use
```

## Running Tests

```bash
export KUBEBUILDER_ASSETS="$(setup-envtest use -p path)"
go test -v ./core/
```

With Ginkgo:

```bash
ginkgo -v ./core/
```

## Future Suites

- [] `core/` - Core reconciliation tests
  - [x] CRUD tests
  - [] Conditions tests
  - [] Namespace annotationt tests
  - [] Late Initialization tests
- [] `adoption/` - Adoption tests
- [] `irs/` - IAMRoleSelector tests
- [] `carm/` - CARM tests
- [] `fieldexport/` - Field Exporter tests
