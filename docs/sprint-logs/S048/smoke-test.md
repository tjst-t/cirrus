# Smoke Test — Sprint S048
実施日時: 2026-04-11

## 環境情報

- Controller HTTP port: 8273 (portman 自動割り当て)
- 認証: `Authorization: Bearer dev-token`
- テナント: `X-Tenant-ID: 4af01cf9-7325-4742-bf30-f1852368c1e8`

## エンドポイント確認

| Endpoint | Method | Status | Response Shape | フロントエンド型との一致 |
|---|---|---|---|---|
| `/healthz` | GET | 200 | `{"status":"ok"}` | N/A |
| `/api/v1/networks` | GET | 200 | `{items:[...], next_cursor:""}` | 一致 (`ListResponse<Network>`) |
| `/api/v1/volumes` | GET | 200 | `{items:[...], next_cursor:""}` | ほぼ一致（後述） |
| `/api/v1/volume-types` | GET | 200 | raw array `[...]` | 一致（`api.get<VolumeType[]>` を使用） |
| `/api/v1/networks/{id}/groups` (実在ID) | GET | 200 | `{items:[], next_cursor:""}` | 一致 (`ListResponse<NetworkGroup>`) |
| `/api/v1/networks/{id}/policies` (実在ID) | GET | 200 | `{items:[], next_cursor:""}` | 一致 (`ListResponse<NetworkPolicy>`) |
| `/api/v1/networks/{dummy_uuid}/groups` | GET | 404 | `{"error":"network not found"}` | N/A |
| `/api/v1/networks/{dummy_uuid}/policies` | GET | 404 | `{"error":"network not found"}` | N/A |

### 実際のレスポンス例

**GET /api/v1/networks**
```json
{
  "items": [{
    "id": "474ae94e-6045-46a5-b956-6cdbd69f84c2",
    "tenant_id": "4af01cf9-...",
    "name": "persist-test-net",
    "cidr": "192.168.1.0/24",
    "vni": 1,
    "status": "active",
    "created_at": "2026-04-10T14:54:50.902328Z",
    "updated_at": "2026-04-10T14:54:50.902328Z"
  }],
  "next_cursor": ""
}
```

**GET /api/v1/volumes**
```json
{
  "items": [{
    "id": "607ffe70-...",
    "tenant_id": "4af01cf9-...",
    "name": "vm-21563045",
    "backend_id": "abb9e38b-...",
    "size_gb": 10,
    "state": "in_use",
    "exported_host_id": "17871665-...",
    "export_info": { "Params": {...}, "Protocol": "sim" },
    "created_at": "2026-04-10T22:34:00.844309Z",
    "updated_at": "2026-04-10T22:34:00.850006Z"
  }],
  "next_cursor": ""
}
```

**GET /api/v1/volume-types**
```json
[{
  "id": "68640473-944f-4446-bb0f-f6bf3171c109",
  "name": "default",
  "description": "Default sim volume type",
  "required_capabilities": [],
  "qos_policy": null,
  "is_public": true,
  "created_at": "2026-04-10T14:54:28.331668Z",
  "updated_at": "2026-04-10T14:54:28.331668Z"
}]
```

## 発見した問題

### 問題 1: `AdminVolumeType` インターフェースとレスポンスの不一致（storage.ts）

`web/src/api/storage.ts` の `AdminVolumeType` インターフェース:
```typescript
export interface AdminVolumeType {
  id: string
  name: string
  backend_id: string  // ← 実際のレスポンスに存在しない
  created_at: string
}
```

実際の `/api/v1/volume-types` レスポンスには `backend_id` フィールドが存在せず、代わりに `description`, `required_capabilities`, `qos_policy`, `is_public`, `updated_at` が含まれる。

**影響**: `StoragePage.tsx` の `VolumeTypesSection` でボリュームタイプ一覧を表示する際、`backend_id` が `undefined` になる。現在の UI でボリュームタイプ行に backend_id を表示していなければ表示上の問題は発生しないが、型定義としては不正確。

### 問題 2: `Volume` インターフェースにないフィールドが返される（マイナー）

実際の `/api/v1/volumes` レスポンスには `backend_id`, `exported_host_id`, `export_info` が含まれるが、`web/src/api/volumes.ts` の `Volume` インターフェースには定義されていない。TypeScript 側で無視されるため実行時の問題はないが、型が不完全。

### 問題 3: 認証要件（動作上の注意）

- テナントスコープの全エンドポイント（`/api/v1/networks`, `/api/v1/volumes`）は `Authorization: Bearer <token>` に加えて `X-Tenant-ID` ヘッダーが必須
- `X-Tenant-ID` 省略時は `400 {"error":"X-Tenant-ID header required"}` が返る
- フロントエンドの `client.ts` は `localStorage` から `X-Tenant-ID` を自動付与するため、ログイン後は問題ない

## 修正実施

`web/src/api/storage.ts` の `AdminVolumeType` インターフェースを実際のレスポンスと一致するよう修正した。

## 結論

主要エンドポイントは全て正常に動作している。`{items:[], next_cursor:""}` のページネーション形状はフロントエンドの `ListResponse<T>` と一致する。`/volume-types` は raw array を返すが、フロントエンドが `api.get` で呼んでいるため一致している。`AdminVolumeType` の `backend_id` フィールド不一致を修正済み。
