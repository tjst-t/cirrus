# Architecture: Cirrus

## Overview

Cirrus は Go で実装された IaaS プラットフォーム。単一バイナリが起動ロールで controller / worker に分岐するモジュラーモノリス。Controller が API・スケジューリング・ネットワーク/ストレージ制御を担い、Worker が各物理ホストで VM・ボリューム・OVS を直接操作する。

## Components

### Controller

- **Responsibility**: HTTP API、認証認可、VM/Network/Storage のオーケストレーション、Reconciler ループ、Heartbeat 監視
- **Location**: `cmd/cirrus/` (entry), `internal/api/`, `internal/controller/`
- **Key interfaces**: `api.Router` — HTTP ルーティング全体
- **Depends on**: identity, compute, network, storage, topology, host, az, state
- **Background loops**:
  - `NetworkReconciler` — OVN 状態との乖離検出・修復（5 分間隔）
  - `StorageReconciler` — Storage 状態との乖離検出・修復（5 分間隔）
  - `HeartbeatMonitor` — Worker heartbeat 監視、3 回連続タイムアウトで `active`/`draining` → `faulty` 自動遷移、`draining` + VM 数 0 → `maintenance` 自動遷移（30 秒間隔）
  - `HostFaultyHandler` — faulty 遷移時に同ホスト上の全 VM を `error`、関連ポートを `down` にカスケード更新

### Worker

- **Responsibility**: libvirt VM 操作、ボリュームのホスト側アタッチ、OVS/OpenFlow 制御、DHCP/DNS/メタデータサービス
- **Location**: `internal/agent/`, `internal/hypervisor/`, `internal/network/agent/`
- **Key interfaces**: `WorkerService` gRPC サーバ（Controller から VM 作成・削除指示を受信）
- **Depends on**: hypervisor, blockdev, netcontroller
- **gRPC**: Controller → Worker 方向（`WorkerService.CreateVM` / `DeleteVM` / `StartVM` / `StopVM` / `ForceStopVM` / `RebootVM` / `GetVMState`）; Worker → Controller 方向（`ControllerService.Heartbeat`）

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

### Compute (`internal/compute`)

- VM ライフサイクル管理（作成・起動・停止・強制停止・再起動・削除）
- `Orchestrator`: 非同期 VM 作成パイプライン（Network.CreatePort → Storage.CreateVolume → Scheduler.Schedule → Storage.ExportVolume → WorkerService.CreateVM）
- `Service` インターフェース: CreateVM/GetVM/ListVMs/DeleteVM/StartVM/StopVM/ForceStopVM/RebootVM/RepairVM
- VM ステータス状態機械: `pending` → `building` → `running` ↔ `stopped`; いずれも `error` へ; `stopped`/`error` → `deleting`
- 操作ガード: 遷移中状態 (`building`/`deleting`/`pending`) での全操作は 409; `running` 中の delete は 409
- 管理者修復 API: `error` → `stopped` 強制遷移（`RepairVM`）
- DB: `vms`, `vm_volumes` テーブル

### Scheduler (`internal/scheduler`)

- `Scheduler.Schedule(spec) → (host_id, backend_id)` でプレースメントを決定
- AZ フィルタ → Flavor 充足フィルタ → リソース空き率スコアリング

### Flavor (`internal/flavor`)

- VM スペック（vCPU/RAM/Disk）のテンプレート管理
- infra_admin が作成・削除、テナントが参照

### BlockDev (`internal/blockdev`)

- Worker 側: ExportInfo の protocol に応じてボリュームを OS ブロックデバイスとしてアタッチ/デタッチ
- Protocol: `rbd` (rbd device map), `iscsi` (iscsiadm login), `sim` (no-op)

### Topology (`internal/topology`)

- 到達性ドメイン、ロケーションツリー、コンピュートプール、AZ 管理
- Scheduler のプレースメント判断に利用される

### Host (`internal/host`)

- ホスト登録・Capability・プロファイル・稼働状態管理
- `SetOperationalState` に遷移ルール適用（host.md 遷移表準拠）、`active`/`draining` → `maintenance` は稼働 VM 数 0 を原子チェック（NOT EXISTS サブクエリ付き UPDATE）
- `missed_heartbeat_count` カラム（DB 永続カウンタ）で heartbeat 途絶を検出
- 状態: `registering` → `active` → `draining`/`maintenance`/`faulty`; `maintenance` → `retiring`（終端）

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
  → reconcile.NetworkReconciler.Run (goroutine, 5分ごと)
  → reconcile.StorageReconciler.Run (goroutine, 5分ごと)
  → 各 Reconciler が DriftEvent を検出 → DriftHandler.Handle へ送信

gRPC Heartbeat 受信
  → reconcile.HeartbeatReconciler.Reconcile (goroutine, 非同期)
  → DB stable VMs vs heartbeat VMInfo を比較
  → 乖離があれば DriftEvent 発火 → DriftHandler.Handle へ送信

DriftHandler.Handle
  → 重複抑制 (インメモリ TTL キャッシュ, resource_id:type キー)
  → AutoHealer 呼び出し (auto_heal_enabled 時のみ):
      compute: VMHealer.HealVM → DB status = error（楽観的ロック）
      network: NetworkHealer.TriggerRefresh → 次回 poll で state 再配信
  → slog.Warn でログ出力
  → drift_events テーブルに永続化
```

### Worker Control Flow

```
Controller → gRPC → Agent (internal/agent)
  → Hypervisor (libvirt) — VM操作
  → BlockDev — ボリュームアタッチ
  → NetworkAgent (OVS) — OpenFlowフロー設定
```

### VM 作成フロー (Compute Orchestrator)

```
POST /api/v1/vms
  → compute.Orchestrator.CreateVM (DB に pending VM 挿入)
  → goroutine で非同期実行:
      1. flavor 解決
      2. scheduler.Schedule → (host_id, backend_id)
      3. network.CreatePort (OVN LSP 作成)
      4. storage.CreateVolume + ExportVolume
      5. WorkerClientPool.Get(host.worker_grpc_addr)
      6. WorkerService.CreateVM gRPC
           → blockdev.Attach (protocol 判定)
           → hypervisor.DefineVM (cloud-init ISO + domain XML → libvirtd-sim)
      7. VM status → running
```

### VM 削除フロー (teardownVM)

```
DELETE /api/v1/vms/{id}  (stopped/error のみ許可)
  → status → deleting
  → goroutine で非同期実行:
      1. WorkerService.DeleteVM gRPC
           → hypervisor.DestroyVM (強制停止)
           → blockdev.Detach (ディスクデタッチ、domain 定義が残っている間に実行)
           → hypervisor.UndefineVM (domain 定義削除)
      2. network.DeletePort
      3. storage.UnexportVolume + DeleteVolume
      4. DB レコード削除
```

### VM 操作フロー (start/stop/force-stop/reboot)

```
POST /api/v1/vms/{id}/actions  {"action": "start"|"stop"|"force-stop"|"reboot"}
  → 状態ガード確認 (IsTransitional → 409, Can* → 409)
  → resolveWorker: host record → WorkerClientPool.Get
  → WorkerService.{Start|Stop|ForceStop|Reboot}VM gRPC
       → hypervisor.{StartVM|ShutdownVM(ACPI)|DestroyVM|RebootVM}
  → VM status 更新
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
│   ├── compute/      # VM ライフサイクル (Orchestrator)
│   ├── scheduler/    # VM プレースメント
│   ├── flavor/       # VM スペックテンプレート
│   ├── blockdev/     # Worker 側 ボリュームアタッチ
│   ├── topology/     # トポロジー・AZ
│   ├── host/         # ホスト管理
│   ├── agent/        # Worker gRPC サーバ (WorkerService)
│   ├── hypervisor/   # libvirt (DefineVM/StartVM 等)
│   ├── netcontroller/# OVS OpenFlow
│   ├── controller/   # Controller gRPC サーバ + WorkerClientPool
│   ├── controller/reconcile/ # DriftHandler + HeartbeatReconciler + Network/StorageReconciler
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
