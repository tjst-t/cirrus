# Architecture: Cirrus

## Overview

Cirrus は Go で実装された IaaS プラットフォーム。単一バイナリが起動ロールで controller / worker に分岐するモジュラーモノリス。Controller が API・スケジューリング・ネットワーク/ストレージ制御を担い、Worker が各物理ホストで VM・ボリューム・OVS を直接操作する。

## Components

### Controller

- **Responsibility**: HTTP API、認証認可、VM/Network/Storage のオーケストレーション、Reconciler ループ
- **Location**: `cmd/cirrus/` (entry), `internal/api/`, `internal/controller/`
- **Key interfaces**: `api.Router` — HTTP ルーティング全体
- **Depends on**: identity, compute, network, storage, topology, host, az, state

### Worker

- **Responsibility**: libvirt VM 操作、ボリュームのホスト側アタッチ、OVS/OpenFlow 制御、DHCP/DNS/メタデータサービス
- **Location**: `internal/agent/`, `internal/hypervisor/`, `internal/network/agent/`
- **Key interfaces**: gRPC サーバ（Controller から指示受信）
- **Depends on**: hypervisor, blockdev, netcontroller

### Identity (`internal/identity`)

- 認証（OIDC）、RBAC 認可、組織・テナント・ユーザ・ロール管理
- `Authenticator`, `Authorizer`, `TenantService` インターフェースを公開

### Network (`internal/network`)

- VPC/Group/Policy/Egress/Ingress の CRUD、HostNetworkState 計算・配信、IPAM
- `internal/netcontroller` — OVS OpenFlow コントローラ（antrea-io/ofnet）

### Storage (`internal/storage`)

- StorageDomain / Backend / VolumeType / Volume の CRUD
- `Driver` インターフェースで Sim / iSCSI / RBD を差し替え可能
- `internal/storage/driver/sim/` — テスト用シムドライバ
- `internal/storage/driver/iscsi/` — iSCSI BackendDriver（cirrus-iscsi-server 経由）
- `internal/storage/driver/rbd/` — Ceph RBD BackendDriver（cirrus-rbd-server 経由）

### Topology (`internal/topology`)

- 到達性ドメイン、ロケーションツリー、コンピュートプール、AZ 管理
- Scheduler のプレースメント判断に利用される

### Host (`internal/host`)

- ホスト登録・Capability・プロファイル・稼働状態管理

### State (`internal/state`)

- PostgreSQL アクセス、golang-migrate マイグレーション（`internal/state/migrations/`）

### CLI (`cmd/cirrusctl/`, `internal/client/`)

- cobra ベース。利用者向けトップレベル + `admin` サブコマンドで管理者向けを分離
- `internal/client/` に Resolve\* メソッド（UUID/名前の両方解決）

## Data Flow

### API Request Flow

```
HTTP Request
  → api.Router (chi)
  → Authenticator.Authenticate → Authorizer.Authorize
  → Handler (internal/api/*_handler.go)
  → Service (internal/{domain}/service_impl.go)
  → Store (internal/{domain}/store.go)
  → PostgreSQL
```

### Reconciler Loop

```
Controller起動
  → reconcile.{Domain}Reconciler.Run (goroutine)
  → 定期ポーリング (5分ごと)
  → desired state (DB) vs actual state (Driver) を比較
  → 乖離があれば Driver を呼び出して修復
```

### Worker Control Flow

```
Controller → gRPC → Agent (internal/agent)
  → Hypervisor (libvirt) — VM操作
  → BlockDev — ボリュームアタッチ
  → NetworkAgent (OVS) — OpenFlowフロー設定
```

## Directory Structure

```
cirrus/
├── cmd/
│   ├── cirrus/              # controller/worker エントリポイント
│   ├── cirrusctl/           # CLI エントリポイント
│   ├── cirrus-iscsi-server/ # iSCSI target 管理サーバ（tgtadm ラッパー、開発用）
│   └── cirrus-rbd-server/   # Ceph RBD 管理サーバ（rbd/ceph CLI ラッパー、開発用）
├── internal/
│   ├── api/          # HTTP ハンドラ・ルータ
│   ├── identity/     # 認証認可・マルチテナント
│   ├── network/      # VPC・OVS制御
│   ├── storage/      # ボリューム・バックエンド管理
│   ├── topology/     # トポロジー・AZ
│   ├── host/         # ホスト管理
│   ├── agent/        # Worker gRPC サーバ
│   ├── hypervisor/   # libvirt
│   ├── netcontroller/# OVS OpenFlow
│   ├── controller/reconcile/ # Reconciler ループ群
│   ├── client/       # cirrusctl API クライアント
│   └── state/        # DB・マイグレーション
├── docs/             # 設計ドキュメント
├── proto/            # gRPC Protobuf 定義
└── test/sim/         # シミュレータ（cirrus-sim）
```

## Infrastructure

- **Database**: PostgreSQL（golang-migrate, `internal/state/migrations/`）
- **Hypervisor**: libvirt（Worker のみ）
- **Network**: OVS + OpenFlow（antrea-io/ofnet）
- **Storage（ローカル開発用）**: iSCSI target（tgt / `docker-compose.storage.yml`）、Ceph RBD（`quay.io/ceph/demo`）— `make serve-storage` で起動
- **External**: AWX/hook 経由で物理インフラ委譲（ラック配線、NetBox等）
