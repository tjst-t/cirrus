# Sprint S047 Smoke Test

Server: `make serve` → controller port 8273

## Login gate
- `GET /healthz` → `{"status":"ok"}` ✓
- Auth token `dev-token` accepted

## S047-1 Endpoint checks

| Endpoint | Method | Status | Response shape | Frontend type | Match |
|----------|--------|--------|----------------|---------------|-------|
| `/api/v1/availability-zones` | GET | 200 | bare array | `api.get<AvailabilityZone[]>` | ✓ |
| `/api/v1/vms` | GET | 200 | `{items,next_cursor}` | `api.list<Vm>` | ✓ |
| `/api/v1/flavors` | GET | 200 | `{items,next_cursor}` | `api.list<Flavor>` | ✓ |
| `/api/v1/volume-types` | GET | 200 | bare array | `api.get<VolumeType[]>` | ✓ |
| `/api/v1/networks` | GET | 200 | `{items,next_cursor}` | `api.list<Network>` | ✓ |
| `/api/v1/ports?network_id=` | GET | 404/error json | bare array on success | `api.get<Port[]>` | ✓ |

## AZ field name verification
Backend response: `{ id, name, description?, location_id, enabled, created_at, updated_at }`
Frontend `AvailabilityZone` interface: `{ id, name, description?, location_id?, enabled, created_at, updated_at }` ✓

## Notes
- `POST /api/v1/vms` returns `{"job_id":"..."}` (202) — frontend ignores body, calls list refresh ✓
- `DELETE /api/v1/vms/{id}` returns `{"job_id":"..."}` (202) — frontend typed as `void`, body ignored ✓
- No mismatches found
