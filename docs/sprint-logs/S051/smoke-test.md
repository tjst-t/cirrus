# S051 Smoke Test Results

## Server

- Controller: `http://localhost:8273`
- Auth token: `dev-token`

## Endpoint checks

| Endpoint | Method | Status | Response shape | Result |
|---|---|---|---|---|
| `/healthz` | GET | 200 | `{"status":"ok"}` | ✓ |
| `/api/v1/vms` | POST (no token) | 401 | `{"code":"ERR_UNAUTHORIZED","message":"..."}` | ✓ |
| `/api/v1/vms` | POST (no tenant) | 400 | `{"code":"ERR_BAD_REQUEST","message":"X-Tenant-ID header required"}` | ✓ |
| `/api/v1/vms` | GET | 200 | `{"items":[],"next_cursor":""}` | ✓ |
| `/api/v1/flavors` | GET | 200 | `{"items":[...],"next_cursor":""}` | ✓ |
| `/api/v1/networks` | GET | 200 | `{"items":[],"next_cursor":""}` | ✓ |
| `/api/v1/availability-zones` | GET | 200 | `[...]` (plain array) | ✓ |
| `/api/v1/volume-types` | GET | 200 | `[...]` (plain array) | ✓ |

## Findings

- `availability-zones` と `volume-types` はプレーン配列を返す（他エンドポイントは PagedResponse）
  - `azApi.list()` は両形式を正規化済み
  - `vmsApi.listVolumeTypes()` は `api.get<VolumeType[]>` で正しくプレーン配列を期待
  - Playwright テストモックをプレーン配列形式に修正済み

## Error format verification

新形式 `{"code":"...","message":"...","detail":...}` が全エラーケースで正しく返されることを確認。
