# API設計

## 認証

リクエストヘッダに `X-API-Key: <key>` で認証。keyからproject_idを引いて、以降の全操作をそのproject内にスコープする。

Phase 1ではJWTは過剰なため不使用。

## エンドポイント

### プロジェクト管理（管理者用）

```
POST   /api/v1/projects
GET    /api/v1/projects
POST   /api/v1/projects/{id}/api-keys
```

### イメージ

```
GET    /api/v1/images
POST   /api/v1/images
```

### ネットワーク

```
POST   /api/v1/networks
GET    /api/v1/networks
GET    /api/v1/networks/{id}
DELETE /api/v1/networks/{id}
```

### VM

```
POST   /api/v1/vms
GET    /api/v1/vms
GET    /api/v1/vms/{id}
DELETE /api/v1/vms/{id}
POST   /api/v1/vms/{id}/actions    # start, stop, reboot
```

### ポート

```
GET    /api/v1/ports
GET    /api/v1/ports/{id}
```

### ワーカー（管理者用）

```
GET    /api/v1/workers
GET    /api/v1/workers/{id}
```

## VM作成

### リクエスト

```http
POST /api/v1/vms
X-API-Key: cirrus_xxxxxxxxxxxx
Content-Type: application/json

{
  "name": "web-01",
  "image_id": "550e8400-...",
  "vcpus": 2,
  "ram_mb": 4096,
  "disk_gb": 20,
  "networks": [
    { "network_id": "660e8400-..." }
  ]
}
```

### レスポンス（202 Accepted）

```json
{
  "id": "550e8400-...",
  "name": "web-01",
  "status": "scheduling",
  "vcpus": 2,
  "ram_mb": 4096,
  "disk_gb": 20,
  "ports": [
    {
      "id": "...",
      "network_id": "660e8400-...",
      "mac_address": "02:ab:cd:ef:01:23",
      "ip_address": "10.100.0.5"
    }
  ],
  "created_at": "2026-03-06T..."
}
```

### 設計判断: 非同期API

VM作成は非同期（202 Accepted → ステータスpolling）。理由:

- libvirtのVM起動に数秒かかる
- イメージコピーがあればさらに時間がかかる
- ポート（IP/MAC）はAPI応答時点で確定させる（ユーザはVM起動完了を待たずにIPアドレスを知れる）

## 内部フロー

```go
func (h *Handler) CreateVM(w http.ResponseWriter, r *http.Request) {
    // 1. パース・バリデーション
    // 2. クォータチェック（現在使用量 + 要求 <= quota）
    // 3. ポート作成（IP/MAC払い出し）
    // 4. DBにvm作成 status=scheduling
    // 5. 非同期ジョブ投入（channelベースのワーカープール）
    // 6. 202レスポンス返却
}
```

非同期ジョブは `chan VMCreateJob` のワーカープールで同時作成数を制御。
