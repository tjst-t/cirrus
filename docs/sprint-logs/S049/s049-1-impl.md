# Sprint S049-1 実装ログ

## 実装内容

### Task S049-1-0: API 型修正

**`web/src/api/egress.ts`**
- `EgressGateway` → `Egress` にリネーム（`name`, `network_name`, `status`, `created_at` を削除）
- `EgressConfig` インターフェース追加（`public_ip?: string`）
- `CreateEgressGatewayRequest` → `CreateEgressRequest` にリネーム（`type`, `config` フィールドに変更）
- API パス・メソッドシグネチャはそのまま維持

**`web/src/api/ingress.ts`**
- `IngressEndpoint` → `Ingress` にリネーム（L4LB 用フィールド `port`, `protocol`, `public_port`, `name`, `vm_id`, `status` を削除）
- `IngressConfig` インターフェース追加（`target_vm_id`, `target_ip`）
- `IpPool` インターフェース追加
- `CreateIngressEndpointRequest` → `CreateIngressRequest` にリネーム（`type`, `public_ip`, `ip_pool_id`, `config` フィールドに変更）
- `ingressApi.listIpPools()` メソッド追加（`GET /admin/ip-pools`）

### Task S049-1-1: EgressPage 実装

`web/src/pages/tenant/EgressPage.tsx` を完全再実装:
- ネットワークセレクター（`networksApi.list()` の items から選択）
- ネットワーク未存在時に `egress-no-network-message` 表示
- Egress 一覧テーブル（type, config.public_ip を表示）
- `CreateEgressDialog` コンポーネント（type=nat_gateway 固定の select、`egress-create-dialog` / `egress-type-select` / `egress-create-submit`）
- インライン削除確認（`egress-delete-button-{id}` / `egress-delete-confirm-{id}` / `egress-delete-cancel-{id}`）
- エラー表示（`egress-error-message`）

### Task S049-1-2: IngressPage 実装

`web/src/pages/tenant/IngressPage.tsx` を完全再実装:
- ネットワークセレクター
- `CreateIngressDialog` コンポーネント（IP プール選択、パブリック IP 入力、ターゲット VM 選択）
- VM 取得は `GET /tenants/{tenantId}/vms`（tenant スコープ）を使用
- public_ip 未入力時のフロントエンドバリデーション
- インライン削除確認

### 重要な修正点

- `vmsApi.list()` は `/vms` を呼ぶが、テストは `/tenants/{tenantId}/vms` をモックしていた
  → IngressPage では `api.list<Vm>('/tenants/${tenantId}/vms')` で直接呼ぶよう修正
  → これにより 401 redirect が解消された

## テスト結果

```
npx playwright test e2e/s049-egress-ingress.spec.ts
15 passed (10.4s)
```

全 15 テスト通過。
