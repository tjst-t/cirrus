# Sprint S047 Review Log

## Story S047-1: VM WebUI 管理

### Implementation Summary

**Files created:**
- `web/src/api/az.ts` — AZ API client (`GET /api/v1/availability-zones` → bare array)
- `web/e2e/vm-lifecycle.spec.ts` — 3 new lifecycle Playwright tests

**Files modified:**
- `web/src/api/vms.ts` — Added `az_id?: string` to `Vm` and `CreateVmRequest`
- `web/src/api/networks.ts` — Added `Port` interface
- `web/src/pages/tenant/VmsPage.tsx` — Flavor/AZ columns, AZ selector in create form, `data-testid` attributes
- `web/src/pages/tenant/VmDetailPage.tsx` — Ports section, volumes placeholder section, `data-testid` attributes
- `web/e2e/vms.spec.ts` — Added `/availability-zones` mock to beforeEach

### Review Findings & Fixes Applied

| # | Severity | Issue | Fix Applied |
|---|----------|-------|-------------|
| 1 | Bug | `azApi.list()` used paginated wrapper for bare-array endpoint | Fixed: use `api.get<AvailabilityZone[]>` |
| 2 | Bug | Port fetch used paginated wrapper for bare-array endpoint | Fixed: use `api.get<Port[]>` |
| 3 | Bug | e2e mocks used `{items:[...]}` shape for bare-array endpoints | Fixed: bare arrays in mocks |
| 4 | Minor | `az_id: ''` initial state should be `undefined` | Fixed |
| 5 | Minor | Missing `data-testid` on VmsPage delete confirm button | Fixed: `vm-list-delete-confirm-button` |
| 6 | Minor | `Port` interface defined inline in page component | Fixed: moved to `api/networks.ts` |

### Sprint-level Review (Phase 2)

| # | Severity | Issue | Fix Applied |
|---|----------|-------|-------------|
| 1 | Bug | `az_id: undefined` initial state causes React uncontrolled-component warning on `<select>` | Fixed: `az_id: ''` to match default `<option value="">` |

Notes:
- Flavors + AZs re-fetched on every `load()` call (after each VM action) — acceptable for MVP
- `Port` interface has defensively optional fields — correct, backend always populates them but safe to keep

### Design Notes (deferred)

- **API envelope inconsistency**: Some endpoints return `{items:[],next_cursor:""}`, others return bare arrays. This is a broader concern for a future cleanup sprint.
- **Client-side port filtering**: Fetching all network ports and filtering by `vm_id` works but scales poorly. A `GET /vms/{id}/ports` endpoint would be cleaner.
- **Volumes placeholder**: No VM→volumes API exists. Detail page shows placeholder pointing to storage navigation.

### Test Results

```
16 passed (7.5s)
- e2e/vms.spec.ts: 5 tests ✓
- e2e/vm-detail.spec.ts: 8 tests ✓
- e2e/vm-lifecycle.spec.ts: 3 tests ✓
```
