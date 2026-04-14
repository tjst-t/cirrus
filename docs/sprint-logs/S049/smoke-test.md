# S049 スモークテスト結果

実行日時: 2026-04-12
サーバー: make serve (controller port 8273)

## エンドポイント確認

| エンドポイント | Method | HTTP | レスポンス形式 | フロントエンド型との一致 |
|---|---|---|---|---|
| `/healthz` | GET | 200 | — | ✓ |
| `/api/v1/me/tenants` | GET | 200 | `{items: [...], next_cursor: ""}` | ✓ |
| `/api/v1/networks` | GET | 200 | `{items: [], next_cursor: ""}` | ✓ |
| `/api/v1/tenants/{id}/quota` | GET | 200 | `{limits: {...}, usage: {...}}` — 全フィールド確認 | ✓ |
| `/api/v1/admin/ip-pools` | GET | 200 | `[]`（配列直返し） | ✓ |
| `/api/v1/tenants/{id}/networks/{nid}/egresses` | GET | 200 | `[]`（配列直返し） | ✓ |
| `/api/v1/networks/{nid}/ingresses` | GET | 200 | `[]`（配列直返し） | ✓ |
| `/api/v1/me` | GET | 404 | — | N/A（フロントエンドは未使用、テストモックのみ） |

## レスポンス形式の確認

### Quota API レスポンス（実際）
```json
{
  "limits": {
    "vcpus": 0, "memory_mb": 0, "volume_gb": 0, "vm_count": 0,
    "volumes": 0, "snapshots": 0, "networks": 0, "egresses": 0, "ingresses": 0
  },
  "usage": {
    "tenant_id": "...", "vcpus_used": 0, "memory_mb_used": 0, "volume_gb_used": 0,
    "vm_count_used": 0, "volumes_used": 0, "snapshots_used": 0,
    "networks_used": 0, "egresses_used": 0, "ingresses_used": 0
  }
}
```
→ `web/src/api/quota.ts` の `TenantQuota` 型と完全一致 ✓

### Egress/Ingress API
- バックエンドは配列を直返し（PagedResponse ラッパーなし）
- フロントエンドは `api.get<T[]>` を使用 → 正しい ✓

## 問題なし
すべてのエンドポイントが期待通りのレスポンス形式で応答。フロントエンドとバックエンドの型不整合なし。
