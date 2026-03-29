# API設計

## 認証

外部IdP（Keycloak/Okta等）とOIDC連携。開発初期は静的設定ファイルかAPIトークン。

リクエストヘッダに `Authorization: Bearer <token>` でJWTトークンを渡す。トークンからユーザIDを取得し、ロール割り当てに基づいて認可判定を行う。

## 認可

全てのAPIエンドポイントで `authorize(user, action, resource) -> allow/deny` を通す。ロールに基づくアクセス制御。

## エンドポイント

### 組織管理（インフラ管理者）

```
POST   /api/v1/organizations
GET    /api/v1/organizations
GET    /api/v1/organizations/{id}
PUT    /api/v1/organizations/{id}
DELETE /api/v1/organizations/{id}
```

### テナント管理（組織管理者）

```
POST   /api/v1/organizations/{org_id}/tenants
GET    /api/v1/organizations/{org_id}/tenants
GET    /api/v1/tenants/{id}
PUT    /api/v1/tenants/{id}
DELETE /api/v1/tenants/{id}
```

### ロール割り当て（組織管理者/テナント管理者）

```
POST   /api/v1/tenants/{id}/role-assignments
GET    /api/v1/tenants/{id}/role-assignments
DELETE /api/v1/tenants/{id}/role-assignments/{assignment_id}
```

### ホスト管理（インフラ管理者）

```
POST   /api/v1/hosts
GET    /api/v1/hosts
GET    /api/v1/hosts/{id}
PUT    /api/v1/hosts/{id}
DELETE /api/v1/hosts/{id}
POST   /api/v1/hosts/{id}/actions            # maintenance, drain, activate
```

### ホストプロファイル（インフラ管理者）

```
POST   /api/v1/host-profiles
GET    /api/v1/host-profiles
GET    /api/v1/host-profiles/{id}
PUT    /api/v1/host-profiles/{id}
POST   /api/v1/host-profiles/{id}/rollout     # ロールアウト開始
```

### ストレージドメイン・バックエンド（インフラ管理者）

```
POST   /api/v1/storage-domains
GET    /api/v1/storage-domains

POST   /api/v1/storage-backends
GET    /api/v1/storage-backends
GET    /api/v1/storage-backends/{id}
PUT    /api/v1/storage-backends/{id}
POST   /api/v1/storage-backends/{id}/actions  # drain, retire
```

### ボリュームタイプ（インフラ管理者）

```
POST   /api/v1/volume-types
GET    /api/v1/volume-types
GET    /api/v1/volume-types/{id}
PUT    /api/v1/volume-types/{id}
```

### VM（テナント操作）

```
POST   /api/v1/vms
GET    /api/v1/vms
GET    /api/v1/vms/{id}
DELETE /api/v1/vms/{id}
POST   /api/v1/vms/{id}/actions               # start, stop, reboot
```

### ボリューム（テナント操作）

```
POST   /api/v1/volumes
GET    /api/v1/volumes
GET    /api/v1/volumes/{id}
DELETE /api/v1/volumes/{id}
PUT    /api/v1/volumes/{id}                    # リサイズ
POST   /api/v1/volumes/{id}/attach             # VMにアタッチ
POST   /api/v1/volumes/{id}/detach             # VMからデタッチ
```

### スナップショット（テナント操作）

```
POST   /api/v1/volumes/{volume_id}/snapshots
GET    /api/v1/snapshots
GET    /api/v1/snapshots/{id}
DELETE /api/v1/snapshots/{id}
POST   /api/v1/snapshots/{id}/clone            # クローンボリューム作成
```

### ネットワーク（テナント操作）

```
POST   /api/v1/networks
GET    /api/v1/networks
GET    /api/v1/networks/{id}
DELETE /api/v1/networks/{id}
```

### ポート（内部API — Computeモジュールが使用。テナントはGETのみ）

```
GET    /api/v1/networks/{network_id}/ports     # テナント: 自Network内のポート一覧
GET    /api/v1/ports/{id}                       # テナント: ポート詳細
```

ポートの作成・削除はVM作成・削除時にComputeモジュールが内部的に行う。テナントが直接操作するAPIではない。

### Group（テナント操作）

```
POST   /api/v1/networks/{network_id}/groups
GET    /api/v1/networks/{network_id}/groups
GET    /api/v1/groups/{id}
DELETE /api/v1/groups/{id}
```

### Policy（テナント操作）

```
POST   /api/v1/networks/{network_id}/policies
GET    /api/v1/networks/{network_id}/policies
GET    /api/v1/policies/{id}
DELETE /api/v1/policies/{id}
```

### Egress（テナント操作）

```
POST   /api/v1/networks/{network_id}/egresses
GET    /api/v1/networks/{network_id}/egresses
GET    /api/v1/egresses/{id}
DELETE /api/v1/egresses/{id}
```

### Ingress（テナント操作）

```
POST   /api/v1/networks/{network_id}/ingresses
GET    /api/v1/networks/{network_id}/ingresses
GET    /api/v1/ingresses/{id}
DELETE /api/v1/ingresses/{id}
```

### Service Insertion（テナント操作）

```
POST   /api/v1/networks/{network_id}/service-insertions
GET    /api/v1/networks/{network_id}/service-insertions
DELETE /api/v1/service-insertions/{id}
```

### Load Balancer（テナント操作）

```
POST   /api/v1/networks/{network_id}/load-balancers
GET    /api/v1/networks/{network_id}/load-balancers
DELETE /api/v1/load-balancers/{id}
```

### ゲートウェイノード管理（インフラ管理者）

```
POST   /api/v1/gateway-nodes
GET    /api/v1/gateway-nodes
PUT    /api/v1/gateway-nodes/{id}
DELETE /api/v1/gateway-nodes/{id}
```

### テンプレート

```
POST   /api/v1/templates
GET    /api/v1/templates
GET    /api/v1/templates/{id}
DELETE /api/v1/templates/{id}
PUT    /api/v1/templates/{id}                  # 公開範囲変更等
```

## VM作成

### リクエスト

```http
POST /api/v1/vms
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "web-01",
  "flavor_id": "...",
  "az": "tokyo-1",
  "network": "my-app",
  "group": "api",
  "volume_type_id": "...",
  "boot_volume_size_gb": 50,
  "user_data": "..."
}
```

### レスポンス（202 Accepted）

```json
{
  "id": "550e8400-...",
  "name": "web-01",
  "status": "scheduling",
  "flavor_id": "...",
  "az": "tokyo-1",
  "volumes": [
    {
      "id": "...",
      "volume_type_id": "...",
      "size_gb": 50,
      "status": "creating"
    }
  ],
  "ports": [
    {
      "id": "...",
      "network_id": "...",
      "group_id": "...",
      "mac_address": "02:ab:cd:ef:01:23",
      "ip_address": "10.100.0.5"
    }
  ],
  "created_at": "2026-03-26T..."
}
```

### 設計判断: 非同期API

VM作成は非同期（202 Accepted → ステータスpolling）。理由:

- ボリューム作成やテンプレートキャッシュコピーに時間がかかる
- ポート（IP/MAC）はAPI応答時点で確定させる（ユーザはVM起動完了を待たずにIPアドレスを知れる）
- ボリュームIDもAPI応答時点で確定

## テナントスコープ

テナント操作のAPIは、認証トークンから取得したユーザIDとリクエストパスまたはヘッダで指定されたテナントIDに基づき、ロール割り当てを検証してからリソースにアクセスする。

テナントIDの指定は `X-Tenant-ID` ヘッダまたはクエリパラメータ `tenant_id` で行う。
