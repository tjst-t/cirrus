# S021-2 Implementation Log: Error Handling + API Finalization

## Changed Files

### New Files
- `internal/api/validate.go` — common API validation helpers (validateName, validateDescription, parseUUID)
- `internal/api/pagination.go` — cursor-based pagination utilities (encodeCursor, decodeCursor, parsePaginationParams, PagedResponse, zeroTime)
- `internal/state/migrations/000021_pagination_created_at.up.sql` — adds `created_at` to `groups` and `policies` tables
- `internal/state/migrations/000021_pagination_created_at.down.sql` — rollback migration
- `docs/sprint-logs/S021/story-2-implementation.md` — this file

### Modified Files

#### Task S021-2-1: Validation strengthening
- `internal/api/vm_handler.go` — use validateName() for VM name; improve flavor_id UUID error message
- `internal/api/flavor_handler.go` — use validateName(); split VCPUs/RAM_MB validation into separate checks
- `internal/api/storage_handler.go` — use validateName()/validateDescription() for volume type; improve backend validation; split required field checks
- `internal/api/network_handler.go` — (already using validate.Name; unchanged)
- `internal/api/identity_handler.go` — improved UUID error messages

#### Task S021-2-2: Async job cleanup (defer cleanup pattern)
- `internal/compute/orchestrator.go`:
  - `buildVM()`: named return `retErr`, register defer cleanup for port deletion on failure, register defer cleanup for volume unexport+delete on failure
  - `teardownVM()`: collect all step errors into `[]error`, continue all steps regardless of individual failures, log collected errors at end, return first error if any

#### Task S021-2-3: Cursor-based pagination
**Service interface changes:**
- `internal/flavor/service.go` — added `ListPage(ctx, afterCreatedAt, afterID, limit)` to Service interface
- `internal/host/service.go` — added `ListHostsPage(ctx, afterCreatedAt, afterID, limit)` to Service interface
- `internal/compute/service.go` — added `ListVMsPage(ctx, tenantID, afterCreatedAt, afterID, limit)` to Service interface
- `internal/network/service.go` — added `ListNetworksPage`, `ListGroupsPage`, `ListPoliciesPage`
- `internal/storage/service.go` — added `ListVolumesPage`
- `internal/identity/service.go` — added `ListOrganizationsPage`, `ListTenantsPage`

**Store implementations:**
- `internal/flavor/store.go` — implemented `ListPage` with cursor SQL `WHERE (created_at, id) > ($1, $2)`
- `internal/host/store.go` — implemented `ListHostsPage`
- `internal/compute/store.go` — implemented `ListVMsPage`, added `vmCols` constant
- `internal/network/store.go` — implemented `ListNetworksPage`, `ListGroupsPage`, `ListPoliciesPage`; updated Group and Policy queries to include `created_at`
- `internal/network/models.go` — added `CreatedAt time.Time` to `Group` and `Policy` structs
- `internal/storage/store.go` — implemented `ListVolumesByTenantPage`
- `internal/storage/service_impl.go` — implemented `ListVolumesPage`; added `ListVolumesByTenantPage` to `storageStore` interface
- `internal/identity/store.go` — implemented `ListOrganizationsPage`, `ListTenantsPage`

**API handler updates:**
- `internal/api/host_handler.go` — `listHosts` uses `ListHostsPage` + `PagedResponse`
- `internal/api/flavor_handler.go` — `listFlavors` uses `ListPage` + `PagedResponse`
- `internal/api/vm_handler.go` — `listVMs` uses `ListVMsPage` + `PagedResponse`
- `internal/api/network_handler.go` — `listNetworks`, `listGroups`, `listPolicies` use paged variants + `PagedResponse`
- `internal/api/storage_handler.go` — `listVolumes` uses `ListVolumesPage` + `PagedResponse`
- `internal/api/identity_handler.go` — `listOrganizations`, `listTenants` use paged variants + `PagedResponse`

**Test mock updates** (to satisfy updated interfaces):
- `internal/controller/grpc_test.go` — added `ListHostsPage` to `mockHostSvc`
- `internal/identity/authorizer_test.go` — added `ListOrganizationsPage`, `ListTenantsPage` to `mockService`
- `internal/storage/service_impl_test.go` — added `ListVolumesByTenantPage` to `fakeStore`
- `internal/scheduler/scheduler_test.go` — added `ListHostsPage` to `fakeHostSvc`, `ListVolumesPage` to `fakeStorageSvc`

## Task Implementation Summary

### S021-2-1: Validation Strengthening
Added `internal/api/validate.go` with `validateName()` (max 64 chars, blank check), `validateDescription()` (max 256 chars), and `parseUUID()` helpers. Applied `validateName()` to createVM, createFlavor, createStorageBackend, createVolumeType, createVolume handlers.

### S021-2-2: Async Job Cleanup
`buildVM()` now uses named return `retErr` and registers deferred cleanups:
1. After port creation: `defer` that deletes the port if `retErr != nil`
2. After volume creation: `defer` that unexports and deletes the volume if `retErr != nil`

`teardownVM()` now collects errors into a slice, continues all cleanup steps regardless of per-step failures, and returns the first error (with count) only after all steps have been attempted. Individual step errors are still logged at WARN level.

### S021-2-3: Cursor-based Pagination
- Cursor format: `base64(created_at_rfc3339nano:uuid)`
- Query params: `?after=<cursor>&limit=<n>` (default 20, max 100)
- Response format: `{"items": [...], "next_cursor": "..."}`
- SQL: `WHERE (created_at, id) > ($1, $2) ORDER BY created_at, id LIMIT $3`
- A DB migration (000021) adds `created_at` columns to `groups` and `policies` tables, which previously lacked them.
- All 9 target list endpoints now return `PagedResponse` instead of bare arrays.

## Build Results

```
go build ./...   # Exit: 0
go vet ./...     # Exit: 0
go test ./...    # All pass (no integration tag)
```

## Notes and Decisions

1. **Pagination response format change**: List endpoints now return `{"items": [...], "next_cursor": "..."}` instead of bare arrays. This is a breaking change for existing clients. Since this is still in active development, the change was made as specified.

2. **State-filtered `listHosts` bypass pagination**: When `?state=` query param is present, the old non-paginated path is used. Adding cursor pagination to state-filtered queries would require additional store methods and was deferred.

3. **Group/Policy `created_at`**: The migration `000021` adds these columns with `DEFAULT now()`. Existing rows will get `now()` as their `created_at`, which means existing groups/policies are ordered by insertion time rather than historical order. This is acceptable for the pagination feature.

4. **`teardownVM` now returns errors**: Previously always returned `nil`. The updated version returns the first error if any steps fail. The caller (`DeleteVM` goroutine) now sets the VM to error state when teardown fails, which is the correct behavior.

5. **`parseUUID` in validate.go**: Defined as a utility but not yet wired to all existing UUID parse calls (those are inline). Can be adopted incrementally.
