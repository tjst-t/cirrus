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
  - `NetworkReconciler` — OVN 状態との乖離検出・修復（5 分間隔）。オプショナルな `OVSFlowVerifier` インターフェース経由で OVS フローレベルの検証も実施（欠損時は `flow_missing` DriftEvent を発火）
  - `StorageReconciler` — Storage 状態との乖離検出・修復（5 分間隔）
  - `HeartbeatMonitor` — Worker heartbeat 監視、3 回連続タイムアウトで `active`/`draining` → `faulty` 自動遷移、`draining` + VM 数 0 → `maintenance` 自動遷移（30 秒間隔）
  - `HostFaultyHandler` — faulty 遷移時に同ホスト上の全 VM を `error`、関連ポートを `down` にカスケード更新
  - `FailoverTrigger` — `HostFaultyHandler` をラップする `FaultyHandler` 実装。faulty 遷移時に: ① カスケード実行 → ② `FencingAgent.Fence`（IPMI 電源断）→ ③ フェンシング成功時のみ `compute.Orchestrator.FailoverVM` を最大 4 並列で実行。フェンシング失敗時は `DriftEvent(critical)` を発火して failover を中止（safe-mode）。ホスト単位の `inFlight` map で重複 failover を防止（非ブロッキング）
  - `FencingAgent` — IPMI 電源断の抽象インターフェース（`internal/controller/fencing/`）。本番: IPMI ドライバ（未実装）; 開発・テスト: `SimFencingAgent`（cirrus-sim の `POST /sim/hosts/{id}/power-off` を呼び出し）

### Worker

- **Responsibility**: libvirt VM 操作、ボリュームのホスト側アタッチ、OVS/OpenFlow 制御、DHCP/DNS/メタデータサービス
- **Location**: `internal/agent/`, `internal/hypervisor/`, `internal/network/agent/`
- **Key interfaces**: `WorkerService` gRPC サーバ（Controller から VM 作成・削除指示を受信）
- **Depends on**: hypervisor, blockdev, netcontroller
- **gRPC**: Controller → Worker 方向（`WorkerService.CreateVM` / `DeleteVM` / `StartVM` / `StopVM` / `ForceStopVM` / `RebootVM` / `GetVMState` / `PrepareMigration` / `StartMigration` / `AcceptMigratedVM`）; Worker → Controller 方向（`ControllerService.Heartbeat`）

### Identity (`internal/identity`)

- 認証（OIDC）、RBAC 認可、組織・テナント・ユーザ・ロール管理
- `Authenticator`, `Authorizer`, `TenantService` インターフェースを公開

### Network (`internal/network`)

- VPC/Group/Policy/Egress/Ingress の CRUD、HostNetworkState 計算・配信、IPAM
- **Gateway Node 管理**: `gateway_nodes` テーブルで管理。`hosts.node_roles` に `gateway` を持つホストを GW ノードとして登録し、Network 単位で 1 台割り当て（Active-Standby HA は S033 で対応）。Worker の `gw_uplink_port` (cirrus.yaml) を起動時に Controller へ通知し `gateway_nodes.uplink_port` に保存
- **IP Pool 管理**: 管理者が公開 IP プール（CIDR）を登録し、Direct IP Ingress の公開 IP をここから払い出す
- **NAT Gateway Egress** (`type=nat_gateway`): テナントネットワーク発の外部通信を GW ノードで SNAT
- **VPN IPsec Egress** (`type=vpn_ipsec`): IKEv2 IPsec トンネル。PSK は AES-GCM 暗号化して DB 保存（`controller.secrets_key`）。GW ノードへ復号済み設定を配信し `VPNManager.ConfigureIPsec` で適用
- **VPN WireGuard Egress** (`type=vpn_wireguard`): WireGuard トンネル。Controller が Curve25519 キーペアを生成し秘密鍵を AES-GCM 暗号化保存。公開鍵は API レスポンスで返却。GW ノードへ復号済み設定を配信し `VPNManager.ConfigureWireGuard` で適用
- **Direct Connect Egress** (`type=direct_connect`): 専用線 VLAN trunk。VLAN ID をテナントが指定し、`uplink_port` は GW ノード登録情報から自動設定。GW ノードへ配信し `DirectConnectManager.ConfigureVLANTrunk` で適用
- **Direct IP Ingress** (`type=direct_ip`): 公開 IP → VM プライベート IP の DNAT ルールを GW ノードで適用
- **L4 LB Ingress** (`type=l4_lb`): 公開 IP:ポート に届いた TCP トラフィックを複数バックエンド VM へ分散。OVS OpenFlow `select` group（5-tuple ハッシュ or 送信元 IP アフィニティ）で実装。バックエンドの健全性は Worker Agent が TCP probe し `ReportBackendHealth` gRPC で Controller へ報告。不健全バックエンドは `l4lb_backend_health` テーブルで管理され次回 HostNetworkState 配信時に除外される
- **内部 LB** (`load_balancers` テーブル): テナント Network 内部の VIP（CIDR から IPAM が /30 境界を考慮して払い出し）への TCP トラフィックを複数バックエンド VM へ分散。GW ノード不要で **全ホスト** の OVS に OVS `select` group + `ct(commit,nat(dst=))` フローを適用。グループ ID は `0x80000000 XOR FNV(lbID)` で外部 L4LB と衝突しないよう分離。健全バックエンドの管理は `lb_backend_health` テーブル。REST API: `POST/GET/DELETE /api/v1/tenants/{tid}/networks/{nid}/load-balancers`; `cirrusctl load-balancer` サブコマンド
- **FallbackRoute（ライブマイグレーション中）**: マイグレーション中の VM のトラフィックを移行元ホストで Geneve トンネル経由で移行先ホストへ転送する OVS フロー。`migration_fallback_routes` テーブルから `ComputeHostNetworkState` が取得し `HostNetworkState.fallback_routes` で移行元ホストへ配信。`TableDstHostResolution`（table 4）で priority=200 のフローとして適用（通常フロー priority=100 を上書き）。マイグレーション完了または失敗時に DB から削除し、次のポーリングで OVS フローが消える
- `StateController.ComputeHostNetworkState`: 各 Worker へ配信する `HostNetworkState` を計算（local ports / remote ports / policies / DNS / egress rules / ingress rules / internal_lb_rules / fallback_routes）。GW ホストでローカル VM がなくても、担当ネットワークの remote port ルーティングを含める。VPN/DC egress では暗号化シークレットを復号してから配信。l4_lb ingress では `l4lb_backend_health` を LEFT JOIN し健全バックエンドのみ配信。内部 LB はネットワークに VM を持つ全ホストへ配信
- `GRPCStateServer.WatchHostNetworkState`: gRPC server-streaming で Worker に `HostNetworkState` を Push（2 秒ポーリング差分検出、`TriggerRefresh` で強制再配信）
- `internal/network/agent/` — Worker 側 OVS OpenFlow フロー生成・適用（Table 0–7）、SNAT/DNAT フロー含む。`VPNManager` / `DirectConnectManager` インターフェースで VPN・VLAN 設定を抽象化（本番: strongSwan/wgctrl、シム: no-op ログ）。`HealthChecker` が 10 秒間隔で l4_lb バックエンドを TCP probe し状態変化時のみ `ReportBackendHealth` gRPC を呼ぶ

### Storage (`internal/storage`)

- StorageDomain / Backend / VolumeType / Volume の CRUD
- `Driver` インターフェースで Sim / iSCSI / RBD を差し替え可能
- `internal/storage/driver/sim/` — テスト用シムドライバ
- `internal/storage/driver/iscsi/` — iSCSI BackendDriver（cirrus-iscsi-server 経由）
- `internal/storage/driver/rbd/` — Ceph RBD BackendDriver（cirrus-rbd-server 経由）

### Compute (`internal/compute`)

- VM ライフサイクル管理（作成・起動・停止・強制停止・再起動・削除）
- `Orchestrator`: 非同期 VM 作成パイプライン（Network.CreatePort → Storage.CreateVolume → Scheduler.Schedule → Storage.ExportVolume → WorkerService.CreateVM）
- `Service` インターフェース: CreateVM/GetVM/ListVMs/DeleteVM/StartVM/StopVM/ForceStopVM/RebootVM/RepairVM/MigrateVM/FailoverVM
- VM ステータス状態機械: `pending` → `building` → `running` ↔ `stopped`; `running` → `migrating` → `running`（ライブマイグレーション）; `error` → `failing_over` → `running`（HA Failover）; いずれも `error` へ; `stopped`/`error` → `deleting`
- **FailoverVM**: `error` 状態の VM を別ホストでコールドスタート。フロー: status→`failing_over`（重複防止）→ Reschedule（障害ホスト除外）→ Storage.ExportVolume（全ボリューム）→ Worker.CreateVM（新ホスト）→ Network.UpdatePortHost → status→`running`。失敗時は status→`error` に戻す（defer）。`failing_over`/`migrating` 中の VM は HeartbeatReconciler の stable 状態チェック対象外
- 操作ガード: 遷移中状態 (`building`/`deleting`/`pending`/`migrating`) での全操作は 409; `running` 中の delete は 409
- 管理者修復 API: `error` → `stopped` 強制遷移（`RepairVM`）
- **ライブマイグレーション** (`MigrateVM`): `running` VM を別ホストへ無停止移行。フロー: status→migrating → Reschedule（または explicit target） → PrepareMigration（dest worker）→ FallbackRoute 挿入→ 3 秒待機 → StartMigration（src worker）→ AcceptMigratedVM（dest worker、HostInstance sim モードで dest に domain 登録）→ DB host_id 更新 → status→running → FallbackRoute 削除（defer）。エラー時は status→error かつ FallbackRoute を defer で削除
- DB: `vms`, `vm_volumes` テーブル

### Quota (`internal/quota`)

- テナントおよび組織単位のリソースクォータ管理（階層: 組織 → テナント）
- **対象リソース**: vCPU, RAM(MB), ボリューム容量(GB), VM数, ボリューム数, スナップショット数, ネットワーク数, Egress ルール数, Ingress ルール数
- **予約パターン**: `Reserve` → 作成成功時 `Commit` / 失敗時 `Release`; 削除時 `Decommit`
- **0 = 無制限**: 全ディメンションで limit=0 は無制限扱い
- **DB**: `quota_usage`（確定使用量）、`quota_reserves`（in-flight 予約）テーブル; テナント/組織の limit はそれぞれ `tenants`, `organizations` テーブルのカラム
- **インジェクション**: compute/storage/network に `quota.Service` インターフェース経由で注入（nil 許容）

### Scheduler (`internal/scheduler`)

- `Scheduler.Schedule(spec) → (host_id, backend_id)` でプレースメントを決定
- `Scheduler.Reschedule(spec) → (host_id)` でライブマイグレーション先ホストを選定（現在ホストを除外）
- 内部ヘルパー `candidateHosts`（AZ 経由のストレージドメイン到達可能ホスト集合取得）と `selectHost`（active 状態・Flavor 充足フィルタ + スコアリング）を `Schedule` / `Reschedule` が共用
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

### JobQueue (`internal/jobqueue`)

- PostgreSQL バックの非同期ジョブキュー（`jobs` テーブル）
- `Queue` インターフェース: `Enqueue` / `Dequeue` (FOR UPDATE SKIP LOCKED CTE) / `Complete` / `Fail` / `ListStuck` / `Get`
- `Dispatcher`: N 本の worker goroutine がポーリング、`HandlerFunc` レジストリでジョブタイプ → ハンドラを解決
- 起動時リカバリ: `RecoverAllRunningJobs` が `status=running` を `pending` にリセット
- **ジョブタイプ**: `vm_create`, `vm_delete`（compute）、`volume_create`, `volume_delete`（storage）
- **認可**: `GET /api/v1/jobs/{id}` — tenant_member は自分のジョブのみ、tenant_admin はテナント内全ジョブ、infra_admin は全ジョブ
- **将来拡張**: `parent_job_id` / `depends_on` によるサブジョブ依存グラフ（docs/todo.md 参照）

### State (`internal/state`)

- PostgreSQL アクセス、golang-migrate マイグレーション（`internal/state/migrations/`）

### WebUI (`web/`)

- Vite + React + Tailwind CSS + shadcn/ui
- design-system のデザイントークンを Tailwind theme に適用
- 開発時: Vite dev server が `/api/*` を controller にプロキシ
- 本番: `web/dist/` を controller の chi FileServer で配信（単一プロセス）
- **WebUI でできることはすべて REST API でも実行可能**（API ファースト原則）

**管理者 UI** (`/admin/*`):
- 組織・テナント・ロール割り当て管理（OrganizationsPage）
- ホスト管理・状態遷移（HostsPage）
- ストレージ Backend / Volume Type / Flavor 管理（StoragePage）
- ゲートウェイノード・IP プール管理（NetworkInfraPage）
- テナント別 Quota 設定（QuotasPage）
- Drift Event ビューア・解決操作（DriftEventsPage）

**テナント UI** (`/`):
- ダッシュボード（Quota 使用量・VM 一覧）
- VM / ネットワーク / ボリューム / Egress / Ingress 管理

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

### Async Job Flow (S045〜)

```
POST /vms (or /volumes)
  → Handler enqueues job (jobs テーブル, status=pending)
  → 202 Accepted + {"job_id": "..."}

Dispatcher (background, 4 workers)
  → Dequeue: SELECT … FOR UPDATE SKIP LOCKED
  → HandlerFunc(ctx, job) — vm_create / volume_create など
  → Complete (status=completed) or Fail (status=failed)

クライアント
  → GET /api/v1/jobs/{id} でステータスをポーリング
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

### NAT Gateway / Ingress フロー

```
[NAT Gateway Egress: VM → 外部]
VM (worker-N)
  → OVS Table 2 (DstGroup): nw_dst=外部IP → resubmit(,7) (EgressTable)
  → OVS Table 7 on worker-N: nw_src=VM-IP → GW ノードへ Geneve 転送
  → GW ホスト OVS Table 7: ct(commit,nat(src=公開IP)), NORMAL
  → 外部ネットワーク

[Direct IP Ingress: 外部 → VM (1:1 DNAT)]
外部クライアント → GW ホスト
  → OVS Table 0 on GW (priority=300): ip,nw_dst=公開IP → ct(commit,nat(dst=VM-IP)), resubmit(,4)
  → OVS Table 4: ip,nw_dst=VM-IP → Geneve encap → worker-N
  → worker-N: OVS → VM tap

[L4 LB Ingress: 外部 → 複数VM (select group)]
外部クライアント → GW ホスト
  → OVS Table 0 on GW (priority=310): ip,tcp,nw_dst=公開IP,tp_dst=ポート → group:<id>
  → OVS select group: 5-tuple hash (or src-IP hash) → bucket → ct(commit,nat(dst=VM-IP:ポート)), resubmit(,4)
  → OVS Table 4 → Geneve encap → worker-N

[L4 LB ヘルスチェックループ]
Worker HealthChecker (10秒間隔)
  → TCP probe → ローカルバックエンド VM
  → 状態変化時のみ: ControllerService.ReportBackendHealth gRPC
  → Controller: l4lb_backend_health テーブル更新
  → 次回 WatchHostNetworkState 配信で不健全バックエンドを除外

[HostNetworkState 配信]
Controller StateController
  → ComputeHostNetworkState (local/remote ports, policies, egress/ingress rules)
  → GRPCStateServer.WatchHostNetworkState (server-streaming, 2秒ポーリング)
  → Worker NetworkAgent.ApplyState
      → OVS flows + groups の差分適用
```

### HA Failover フロー

```
HeartbeatMonitor: ホストの heartbeat 途絶検出
  → HostFaultyHandler.Handle (VMs → error, ports → down)  ← FailoverTrigger がラップ
  → FailoverTrigger.Handle (非ブロッキング goroutine)
      1. HostFaultyHandler.Handle (カスケード)
      2. FencingAgent.Fence → POST /sim/hosts/{id}/power-off
         失敗時: DriftEvent(critical, host, fencing_failed) → 中止
      3. listErrorVMsOnHost → error 状態 VM 一覧
      4. FailoverVM × N (最大 4 並列, best-effort)
           → status → failing_over (重複防止)
           → Scheduler.Reschedule (障害ホスト除外)
           → Storage.ExportVolume (全ボリューム → 新ホスト)
           → WorkerService.CreateVM gRPC (新ホスト)
           → Network.UpdatePortHost (port の host_id 更新)
           → status → running
           失敗時: status → error (defer)
```

### VM 作成フロー (Compute Orchestrator)

```
POST /api/v1/vms
  → compute.Orchestrator.CreateVM
      1. flavor 解決
      2. quota.Reserve (vcpus/ram/vms)
      3. DB に pending VM 挿入
  → goroutine で非同期実行:
      4. scheduler.Schedule → (host_id, backend_id)
      5. network.CreatePort (OVN LSP 作成)
      6. storage.CreateVolume + ExportVolume
      7. WorkerClientPool.Get(host.worker_grpc_addr)
      8. WorkerService.CreateVM gRPC
           → blockdev.Attach (protocol 判定)
           → hypervisor.DefineVM (cloud-init ISO + domain XML → libvirtd-sim)
      9. quota.Commit → VM status → running
         (失敗時: quota.Release → VM status → error)
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
│   ├── quota/        # クォータ管理（予約パターン、階層チェック）
│   ├── scheduler/    # VM プレースメント
│   ├── flavor/       # VM スペックテンプレート
│   ├── blockdev/     # Worker 側 ボリュームアタッチ
│   ├── topology/     # トポロジー・AZ
│   ├── host/         # ホスト管理
│   ├── agent/        # Worker gRPC サーバ (WorkerService)
│   ├── hypervisor/   # libvirt (DefineVM/StartVM 等)
│   ├── netcontroller/# OVS OpenFlow
│   ├── controller/   # Controller gRPC サーバ + WorkerClientPool
│   ├── controller/fencing/   # FencingAgent インターフェース + SimFencingAgent
│   ├── controller/reconcile/ # DriftHandler + HeartbeatReconciler + FailoverTrigger + Network/StorageReconciler
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
