# アーキテクチャ

## 概要

Cirrusは単一のGoバイナリで、起動時のロール指定によりcontrollerまたはworkerとして動作するモジュラーモノリスである。

- **Controller** — API、スケジューラ、ネットワーク制御（OVS コントローラ）、ストレージ制御を担う。1台以上。
- **Worker** — 物理ホストごとに1プロセス。libvirtによるVM操作、ボリュームのホスト側アタッチ、およびcirrus-agentによるOVS制御・DNS・DHCP・メタデータサービスを担う。

```bash
cirrus controller --config=/etc/cirrus/controller.yaml
cirrus worker --config=/etc/cirrus/worker.yaml
```

## コンポーネント配置

```
┌──────────────────────────────────────────────────────────────────────┐
│  Controller                                                          │
│                                                                      │
│  ┌────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐             │
│  │  API   │  │ Identity │  │  Quota   │  │   Hook   │             │
│  │ (HTTP) │  │ (認証認可) │  │(階層クォータ)│  │ (AWX等)  │             │
│  └───┬────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘             │
│      │            │             │              │                     │
│  ┌───┴────────────┴─────────────┴──────────────┴───────────────┐   │
│  │                     Domain Services                          │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌───────────────┐ │   │
│  │  │ Compute │  │ Network │  │ Storage │  │   Template    │ │   │
│  │  │         │  │(OVS Ctl)│  │         │  │               │ │   │
│  │  └────┬────┘  └────┬────┘  └────┬────┘  └───────┬───────┘ │   │
│  └───────┼─────────────┼───────────┼────────────────┼─────────┘   │
│          │             │           │                │               │
│  ┌───────┴─────────────┴───────────┴────────────────┴───────────┐ │
│  │                   Infrastructure Services                     │ │
│  │  ┌───────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │ │
│  │  │ Scheduler │  │ Topology │  │   Host   │  │   State    │  │ │
│  │  │           │  │          │  │          │  │ (PostgreSQL)│  │ │
│  │  └───────────┘  └──────────┘  └──────────┘  └────────────┘  │ │
│  └───────────────────────────────────────────────────────────────┘ │
│         │ gRPC                                                      │
└─────────┼──────────────────────────────────────────────────────────┘
          │
     ┌────┴────┐
     │         │
┌────▼─────┐ ┌─▼──────────┐
│ Worker-01│ │ Worker-02  │    ...
│          │ │            │
│ ┌──────┐ │ │ ┌──────┐   │
│ │Agent │ │ │ │Agent │   │
│ └──┬───┘ │ │ └──┬───┘   │
│    │     │ │    │       │
│ ┌──▼───┐ │ │ ┌──▼───┐   │
│ │Hyper-│ │ │ │Hyper-│   │
│ │visor │ │ │ │visor │   │
│ └──────┘ │ │ └──────┘   │
│ ┌──────┐ │ │ ┌──────┐   │
│ │Block │ │ │ │Block │   │
│ │Dev   │ │ │ │Dev   │   │
│ └──────┘ │ │ └──────┘   │
│ ┌──────┐ │ │ ┌──────┐   │
│ │Net   │ │ │ │Net   │   │
│ │Agent │ │ │ │Agent │   │
│ │(OVS) │ │ │ │(OVS) │   │
│ └──────┘ │ │ └──────┘   │
└──────────┘ └────────────┘
```

## モジュール一覧

### Controller側

| モジュール | パッケージ | 責務 |
|---|---|---|
| API | `internal/api` | HTTPルーティング、リクエスト/レスポンス変換、ミドルウェア |
| Identity | `internal/identity` | 認証（OIDC）、認可（RBAC）、組織・テナント・ユーザ管理 |
| Compute | `internal/compute` | VMライフサイクルのオーケストレーション（作成〜削除の一連の流れを調整） |
| Network | `internal/network` | Network/Group/Policy/Egress/Ingress管理、HostNetworkState計算・配信、IPアドレス採番 |
| Storage | `internal/storage` | ボリューム/スナップショット/クローン管理、バックエンドドライバ呼び出し |
| Template | `internal/template` | テンプレート管理、キャッシュコピーのLRU管理 |
| Scheduler | `internal/scheduler` | プレースメント判断（フィルタリング、スコアリング、DRS） |
| Topology | `internal/topology` | 到達性ドメイン、ロケーションツリー、コンピュートプール導出、ゾーン導出 |
| Host | `internal/host` | ホスト登録、Capability管理、プロファイル管理、稼働状態管理 |
| Quota | `internal/quota` | 階層化クォータ（組織→テナント）の検査と管理 |
| Hook | `internal/hook` | 外部システム連携フレームワーク（AWXジョブ実行、NetBoxトポロジ同期） |
| State | `internal/state` | PostgreSQLアクセス、マイグレーション、トランザクション管理 |

### Worker側

| モジュール | パッケージ | 責務 |
|---|---|---|
| Agent | `internal/agent` | gRPCサーバ。controllerからの指示を受けてhypervisor/blockdevに委譲 |
| Hypervisor | `internal/hypervisor` | libvirt経由のVM操作（define, start, stop, destroy, migrate） |
| BlockDev | `internal/blockdev` | ボリュームのホスト側アタッチ/デタッチ（RBDマップ、iSCSIログイン等） |
| NetworkAgent | `internal/network/agent` | OVS OpenFlowフロー管理、DHCP応答、DNS応答、メタデータサービス |

## モジュール間インターフェース

各モジュールは自身のインターフェースを公開し、他のモジュールはインターフェース経由でのみアクセスする。具体的な実装への直接依存は禁止。

### Identity

```go
package identity

// Authenticator はリクエストからユーザを特定する。
type Authenticator interface {
    Authenticate(ctx context.Context, token string) (*User, error)
}

// Authorizer は操作の認可判定を行う。
type Authorizer interface {
    Authorize(ctx context.Context, user *User, action Action, resource Resource) (Decision, error)
}

// TenantService は組織・テナントのCRUDを提供する。
type TenantService interface {
    CreateOrganization(ctx context.Context, spec OrganizationSpec) (*Organization, error)
    CreateTenant(ctx context.Context, orgID string, spec TenantSpec) (*Tenant, error)
    AssignRole(ctx context.Context, assignment RoleAssignment) error
    // ...
}
```

### Compute

```go
package compute

// Service はVMライフサイクルを統括する。
// 内部でScheduler, Network, Storage, Host, Quotaを協調させる。
type Service interface {
    CreateVM(ctx context.Context, tenantID string, spec VMSpec) (*VM, error)
    DeleteVM(ctx context.Context, tenantID string, vmID string) error
    StartVM(ctx context.Context, tenantID string, vmID string) error
    StopVM(ctx context.Context, tenantID string, vmID string) error
    RebootVM(ctx context.Context, tenantID string, vmID string) error
    MigrateVM(ctx context.Context, vmID string, reason MigrateReason) error
    GetVM(ctx context.Context, tenantID string, vmID string) (*VM, error)
    ListVMs(ctx context.Context, tenantID string, opts ListOpts) ([]*VM, error)
}
```

### Network

```go
package network

// Service はテナント向けネットワーク操作を提供する。
type Service interface {
    // Network
    CreateNetwork(ctx context.Context, tenantID string, spec NetworkSpec) (*Network, error)
    DeleteNetwork(ctx context.Context, tenantID string, networkID string) error

    // Group
    CreateGroup(ctx context.Context, networkID string, spec GroupSpec) (*Group, error)
    DeleteGroup(ctx context.Context, groupID string) error

    // Policy
    CreatePolicy(ctx context.Context, networkID string, spec PolicySpec) (*Policy, error)
    DeletePolicy(ctx context.Context, policyID string) error

    // Port（内部API、Computeモジュールから呼び出し）
    CreatePort(ctx context.Context, spec PortSpec) (*Port, error)
    DeletePort(ctx context.Context, portID string) error

    // Egress
    CreateEgress(ctx context.Context, networkID string, spec EgressSpec) (*Egress, error)
    DeleteEgress(ctx context.Context, egressID string) error

    // Ingress
    CreateIngress(ctx context.Context, networkID string, spec IngressSpec) (*Ingress, error)
    DeleteIngress(ctx context.Context, ingressID string) error
}
```

### Storage

```go
package storage

// Service はテナント向けボリューム操作を提供する。
type Service interface {
    // ボリューム
    CreateVolume(ctx context.Context, tenantID string, spec VolumeSpec) (*Volume, error)
    DeleteVolume(ctx context.Context, tenantID string, volumeID string) error
    ResizeVolume(ctx context.Context, tenantID string, volumeID string, newSizeGB int) error
    AttachVolume(ctx context.Context, volumeID string, vmID string, device string) error
    DetachVolume(ctx context.Context, volumeID string) error

    // スナップショット
    CreateSnapshot(ctx context.Context, tenantID string, volumeID string, spec SnapshotSpec) (*Snapshot, error)
    DeleteSnapshot(ctx context.Context, tenantID string, snapshotID string) error
    CloneFromSnapshot(ctx context.Context, tenantID string, snapshotID string, spec VolumeSpec) (*Volume, error)

    // バックエンド管理（インフラ管理者）
    RegisterBackend(ctx context.Context, spec BackendSpec) (*Backend, error)
    DrainBackend(ctx context.Context, backendID string) error
}

// BackendDriver はストレージバックエンドとの通信を抽象化する。
// バックエンドの種類（Ceph, iSCSI, NFS等）ごとに実装。
type BackendDriver interface {
    CreateVolume(ctx context.Context, spec DriverVolumeSpec) (*DriverVolume, error)
    DeleteVolume(ctx context.Context, volumeID string) error
    ResizeVolume(ctx context.Context, volumeID string, newSizeGB int) error
    ExportVolume(ctx context.Context, volumeID string, hostID string) (*ExportInfo, error)
    UnexportVolume(ctx context.Context, volumeID string, hostID string) error
    CreateSnapshot(ctx context.Context, volumeID string, snapID string) error
    DeleteSnapshot(ctx context.Context, snapID string) error
    CloneSnapshot(ctx context.Context, snapID string, newVolID string) error
    Capabilities() BackendCapabilities
}
```

### Template

```go
package template

// Service はテンプレートの管理とキャッシュを提供する。
type Service interface {
    Create(ctx context.Context, spec TemplateSpec) (*Template, error)
    Get(ctx context.Context, templateID string) (*Template, error)
    List(ctx context.Context, opts ListOpts) ([]*Template, error)
    Delete(ctx context.Context, templateID string) error
    // EnsureCached はテンプレートが指定バックエンドにキャッシュされていることを保証する。
    // キャッシュがなければバックグラウンドでコピーを開始する。
    EnsureCached(ctx context.Context, templateID string, backendID string) (*CacheStatus, error)
}
```

### Scheduler

```go
package scheduler

// Scheduler はVMとボリュームの配置先を決定する。
type Scheduler interface {
    // Schedule は新規VMの配置先（ホスト, バックエンド）ペアを決定する。
    Schedule(ctx context.Context, req ScheduleRequest) (*ScheduleResult, error)
    // Reschedule はライブマイグレーション先のホストを決定する。
    Reschedule(ctx context.Context, req RescheduleRequest) (*RescheduleResult, error)
}

type ScheduleRequest struct {
    VM          VMRequirements
    Volumes     []VolumeRequirements
    Affinity    []AffinityRule
    TenantID    string
}

type ScheduleResult struct {
    HostID    string
    // ボリュームごとの配置先バックエンド
    Backends  map[string]string // volumeKey -> backendID
}
```

### Topology

```go
package topology

// Service は到達性ドメインとロケーション階層を管理する。
type Service interface {
    // 到達性ドメイン
    GetComputePool(ctx context.Context, storageDomainID string) (*ComputePool, error)
    ListReachableHosts(ctx context.Context, backendID string) ([]string, error)
    ListReachableBackends(ctx context.Context, hostID string) ([]string, error)

    // ロケーション
    GetLocationPath(ctx context.Context, locationID string) ([]*Location, error)
    GetFaultDomains(ctx context.Context, level string) ([]*FaultDomain, error)

    // ストレージドメイン
    CreateStorageDomain(ctx context.Context, spec StorageDomainSpec) (*StorageDomain, error)
}
```

### Host

```go
package host

// Service はホストの登録・状態管理を提供する。
type Service interface {
    Register(ctx context.Context, spec HostSpec) (*Host, error)
    UpdateCapability(ctx context.Context, hostID string, cap Capability) error
    SetOperationalState(ctx context.Context, hostID string, state OperationalState) error
    Heartbeat(ctx context.Context, hostID string, report ResourceReport) error
    GetHost(ctx context.Context, hostID string) (*Host, error)
    ListHosts(ctx context.Context, opts ListOpts) ([]*Host, error)

    // プロファイル
    CreateProfile(ctx context.Context, spec ProfileSpec) (*Profile, error)
    ApplyProfile(ctx context.Context, hostID string, profileID string) error
    StartRollout(ctx context.Context, profileID string, policy RolloutPolicy) (*Rollout, error)

    // リソース照会（スケジューラが使用）
    GetAllocatable(ctx context.Context, hostID string) (*AllocatableResources, error)
    GetAllocated(ctx context.Context, hostID string) (*AllocatedResources, error)
}
```

### Quota

```go
package quota

// Service は階層化クォータの検査と管理を提供する。
type Service interface {
    // Check はリソース要求がクォータ内に収まるか検査する。
    // 組織クォータとテナントクォータの両方を検査。
    Check(ctx context.Context, tenantID string, request ResourceRequest) error
    // Reserve はリソースを予約する（VM作成開始時）。
    Reserve(ctx context.Context, tenantID string, request ResourceRequest) (ReservationID, error)
    // Commit は予約を確定する（VM作成成功時）。
    Commit(ctx context.Context, reservationID ReservationID) error
    // Release は予約を解放する（VM作成失敗時またはリソース削除時）。
    Release(ctx context.Context, reservationID ReservationID) error
    // SetQuota はテナントのクォータを設定する。
    SetQuota(ctx context.Context, tenantID string, quota QuotaSpec) error
}
```

### Hook

```go
package hook

// Executor は外部システムへのジョブ実行を抽象化する。
type Executor interface {
    // Execute はhookを実行し、完了を待つ。
    Execute(ctx context.Context, hook HookSpec, params map[string]any) (*Result, error)
}

type HookSpec struct {
    Endpoint      string
    TemplateID    string            // AWXジョブテンプレートID
    ParamMapping  map[string]string // Cirrus内部データ → AWX変数
    Timeout       time.Duration
    RetryCount    int
    RollbackHook  *HookSpec         // 失敗時のロールバック用hook
}
```

### Agent（Worker側 gRPCサーバ）

```go
package agent

// Agent はworker側のgRPCサーバ。
// controllerからの指示を受けてHypervisorとBlockDevに委譲する。
type Agent interface {
    Serve(listenAddr string) error
    Stop()
}
```

### Hypervisor（Worker側）

```go
package hypervisor

// Driver はlibvirt操作を抽象化する。
type Driver interface {
    DefineVM(ctx context.Context, spec VMSpec) error
    StartVM(ctx context.Context, vmID string) error
    StopVM(ctx context.Context, vmID string) error
    DestroyVM(ctx context.Context, vmID string) error
    UndefineVM(ctx context.Context, vmID string) error
    GetVMState(ctx context.Context, vmID string) (VMState, error)
    ListVMs(ctx context.Context) ([]VMInfo, error)
    MigrateVM(ctx context.Context, vmID string, destURI string, params MigrateParams) error
}
```

### BlockDev（Worker側）

```go
package blockdev

// Manager はホスト側のボリュームアタッチ/デタッチを担う。
type Manager interface {
    Attach(ctx context.Context, info AttachInfo) (*DevicePath, error)
    Detach(ctx context.Context, info AttachInfo) error
    List(ctx context.Context) ([]AttachedDevice, error)
}

type AttachInfo struct {
    Protocol string            // "rbd", "iscsi", "nfs"
    Params   map[string]string // protocol固有のパラメータ
}
```

## gRPCインターフェース

gRPCは3つのサービスで構成される:

- **ControllerService** — controller側がgRPCサーバ。workerが接続してheartbeatを送信する
- **WorkerAgent** — worker側がgRPCサーバ。controllerがVM操作等の指示を送る（Sprint 7以降で実装）
- **NetworkStateService** — controller側がgRPCサーバ。workerのネットワークエージェントにHostNetworkStateをストリーミング配信する

### ControllerService（Worker → Controller）

workerがcontrollerに接続し、登録・heartbeatを行う。

```protobuf
service ControllerService {
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
}
```

### NetworkStateService（Controller → Worker）

controllerからworkerのネットワークエージェントへHostNetworkStateをストリーミング配信する。

```protobuf
service NetworkStateService {
    rpc StreamHostNetworkState(StreamRequest) returns (stream HostNetworkState);
}

message HostNetworkState {
  repeated PortState ports = 1;
  repeated PolicyRule policies = 2;
  repeated RemotePort remote_ports = 3;
  repeated EgressRule egresses = 4;
  repeated DnsRecord dns_records = 5;
}

message PortState {
  string port_id = 1;
  string vm_id = 2;
  string network_id = 3;
  string group_id = 4;
  string mac_address = 5;
  string ip_address = 6;
  string gateway_ip = 7;
}

message PolicyRule {
  string network_id = 1;
  string src_group_id = 2;
  string dst_group_id = 3;
  string protocol = 4;
  int32 dst_port = 5;
  int32 priority = 6;
  string action = 7;
}

message RemotePort {
  string network_id = 1;
  string group_id = 2;
  string ip_address = 3;
  string host_ip = 4;
}

message DnsRecord {
  string name = 1;
  string ip = 2;
  string network_id = 3;
}
```

### WorkerAgent（Controller → Worker）

controllerからworkerへVM操作等の指示を行う。

```protobuf
service WorkerAgent {
  // VM操作
  rpc CreateVM(CreateVMRequest) returns (CreateVMResponse);
  rpc DeleteVM(DeleteVMRequest) returns (DeleteVMResponse);
  rpc StartVM(StartVMRequest) returns (StartVMResponse);
  rpc StopVM(StopVMRequest) returns (StopVMResponse);
  rpc RebootVM(RebootVMRequest) returns (RebootVMResponse);
  rpc GetVMState(GetVMStateRequest) returns (GetVMStateResponse);

  // ライブマイグレーション
  rpc PrepareMigration(PrepareMigrationRequest) returns (PrepareMigrationResponse);
  rpc StartMigration(StartMigrationRequest) returns (StartMigrationResponse);

  // ボリューム（ホスト側）
  rpc AttachVolume(AttachVolumeRequest) returns (AttachVolumeResponse);
  rpc DetachVolume(DetachVolumeRequest) returns (DetachVolumeResponse);
}

message CreateVMRequest {
  string vm_id = 1;
  string name = 2;
  int32 vcpus = 3;
  int32 ram_mb = 4;
  repeated DiskSpec disks = 5;
  repeated PortSpec ports = 6;
  bytes cloud_init = 7;
}

message DiskSpec {
  string volume_id = 1;
  string device = 2;       // "vda", "vdb"
  bool boot = 3;
  ExportInfo export = 4;   // バックエンドドライバが返した接続情報
}

message PortSpec {
  string port_id = 1;
  string group_id = 2;
  string mac_address = 3;
  string ip_address = 4;
  string gateway_ip = 5;
}

message ExportInfo {
  string protocol = 1;             // "rbd", "iscsi"
  map<string, string> params = 2;  // protocol固有パラメータ
}

message HeartbeatRequest {
  string host_id = 1;
  ResourceReport resources = 2;
}

message ResourceReport {
  int32 used_vcpus = 1;
  int64 used_ram_mb = 2;
  repeated VMInfo running_vms = 3;
}
```

## VM作成時のモジュール間呼び出しフロー

```
API
 │  POST /api/v1/vms
 ▼
Identity.Authenticate → Identity.Authorize
 │
 ▼
Compute.CreateVM
 ├─→ Quota.Check → Quota.Reserve
 ├─→ Network.CreatePort（IP/MAC払い出し、Group割り当て）
 ├─→ Storage.CreateVolume（バックエンドドライバ経由）
 ├─→ Template.EnsureCached（テンプレートキャッシュ確認）
 ├─→ Scheduler.Schedule
 │    ├─→ Topology.ListReachableHosts
 │    ├─→ Topology.ListReachableBackends
 │    ├─→ Host.GetAllocatable（各候補ホスト）
 │    └─→ スコアリング → (host_id, backend_id)
 ├─→ Storage.ExportVolume（バックエンドからホストへのエクスポート）
 ├─→ Agent.CreateVM (gRPC → worker)
 │    ├─→ BlockDev.Attach（ホスト側ボリュームマウント）
 │    ├─→ Hypervisor.DefineVM（domain XML生成、libvirt define）
 │    └─→ Hypervisor.StartVM（libvirt start）
 ├─→ HostNetworkState更新 → エージェントにストリーミング配信
 └─→ Quota.Commit
```

## ディレクトリ構成

```
cirrus/
├── cmd/cirrus/
│   └── main.go                  # controller/worker サブコマンド
├── internal/
│   ├── api/                     # HTTP handlers, middleware, routing
│   │   ├── handler.go
│   │   ├── middleware.go
│   │   └── router.go
│   │
│   ├── identity/                # 認証・認可・テナント管理
│   │   ├── service.go           # interface定義
│   │   ├── authenticator.go     # OIDC実装
│   │   ├── authorizer.go        # RBAC実装
│   │   └── tenant.go            # 組織・テナントCRUD
│   │
│   ├── compute/                 # VMライフサイクルオーケストレーション
│   │   ├── service.go           # interface定義
│   │   └── orchestrator.go      # 実装（他モジュールの協調）
│   │
│   ├── network/                 # ネットワーク管理
│   │   ├── service.go           # interface定義
│   │   ├── model.go             # Network, Group, Policy
│   │   ├── controller.go        # 状態計算、エージェントへの配信
│   │   ├── ipam.go              # IPアドレス採番（/30ブロック）
│   │   └── agent.go             # OVS制御、OpenFlow変換、DNS、DHCP、メタデータ
│   │
│   ├── storage/                 # ボリューム管理
│   │   ├── service.go           # interface定義
│   │   ├── manager.go           # 実装
│   │   └── driver/              # バックエンドドライバ
│   │       ├── driver.go        # BackendDriver interface定義
│   │       ├── ceph/
│   │       ├── iscsi/
│   │       └── nfs/
│   │
│   ├── template/                # テンプレート管理
│   │   ├── service.go           # interface定義
│   │   └── manager.go           # 実装（キャッシュLRU含む）
│   │
│   ├── scheduler/               # プレースメント
│   │   ├── scheduler.go         # interface定義
│   │   ├── filter.go            # フィルタリングロジック
│   │   └── scorer.go            # スコアリングロジック
│   │
│   ├── topology/                # 到達性ドメイン・ロケーション
│   │   ├── service.go           # interface定義
│   │   └── manager.go           # 実装
│   │
│   ├── host/                    # ホスト管理
│   │   ├── service.go           # interface定義
│   │   ├── manager.go           # 実装
│   │   └── profile.go           # プロファイル管理
│   │
│   ├── quota/                   # クォータ管理
│   │   ├── service.go           # interface定義
│   │   └── manager.go           # 実装
│   │
│   ├── hook/                    # 外部システム連携
│   │   ├── executor.go          # interface定義
│   │   └── awx/                 # AWX実装
│   │       └── awx.go
│   │
│   ├── state/                   # データベースアクセス
│   │   ├── db.go                # DB接続、トランザクション
│   │   ├── models.go            # モデル定義
│   │   └── migrations/          # スキーママイグレーション
│   │
│   ├── agent/                   # Worker側 gRPCサーバ
│   │   └── agent.go
│   │
│   ├── hypervisor/              # Worker側 libvirt操作
│   │   ├── driver.go            # interface定義
│   │   └── libvirt/
│   │       └── libvirt.go       # go-libvirt実装
│   │
│   └── blockdev/                # Worker側 ボリュームアタッチ
│       ├── manager.go           # interface定義
│       ├── rbd/
│       │   └── rbd.go
│       └── iscsi/
│           └── iscsi.go
│
├── proto/
│   └── agent.proto              # gRPC定義
├── test/
│   ├── sim/                 # シミュレータ群
│   │   ├── libvirtd/        # libvirtdシミュレータ
│   │   ├── storage/         # ストレージAPIシミュレータ
│   │   └── awx/             # AWXシミュレータ
│   ├── mock/
│   │   └── ovs/             # OVSモッククライアント
│   └── integration/         # 結合テスト
│       ├── docker-compose.yml
│       ├── Dockerfile.worker
│       └── testcases/
├── docs/
├── go.mod
└── go.sum
```

## 依存関係のルール

1. **上位モジュール → 下位モジュールの方向のみ依存可能**
   - API → Domain Services → Infrastructure Services
   - 逆方向の依存は禁止

2. **モジュール間はインターフェース経由のみ**
   - `compute`パッケージは`network.Service`インターフェースに依存するが、`network`の内部実装には依存しない

3. **Stateモジュールの位置づけ**
   - 各モジュールがStateを直接使う（リポジトリパターン）
   - モジュール間でDBトランザクションを共有する場合は`context`経由で渡す

4. **Worker側モジュールはController側モジュールに依存しない**
   - gRPC protobufの型定義のみ共有

## 設定ファイル

```yaml
# controller.yaml
role: controller
listen: 0.0.0.0:8080
grpc_listen: 0.0.0.0:9090

db:
  dsn: postgres://cirrus:xxx@localhost:5432/cirrus

identity:
  oidc:
    issuer: https://keycloak.example.com/realms/cirrus
    client_id: cirrus-api
  # 開発用: 静的トークン
  # static_tokens:
  #   - token: "dev-token-001"
  #     user_id: "user-001"
  #     tenant_id: "tenant-001"

storage:
  drivers:
    ceph:
      monitors: [192.168.100.20:6789]
    # iscsi:
    #   portal: 192.168.100.10:3260

hooks:
  awx:
    endpoint: https://awx.example.com
    token_env: CIRRUS_AWX_TOKEN

topology_sync:
  netbox:
    endpoint: https://netbox.example.com
    token_env: CIRRUS_NETBOX_TOKEN
    interval: 5m
```

```yaml
# worker.yaml
role: worker
controller: controller.example.com:9090
host_id: host-001

hypervisor:
  driver: libvirt
  uri: qemu:///system

blockdev:
  drivers:
    rbd:
      monitors: [192.168.100.20:6789]
    # iscsi: {}

network_agent:
  ovs_bridge: br-int
  geneve_local_ip: auto   # アンダーレイIPを自動検出
```

## リソースモデルの全体構造

### 到達性ドメインとコンピュートプール

**ストレージドメイン** — 同一のストレージバックエンド群にアクセス可能なホストの集合。一つのホストが複数ドメインに属し得る。

**ネットワーク** — ネットワークはオーバーレイ（Geneve）のため、L3到達性があれば任意のホスト間で通信可能。管理ドメインの概念は不要。

**コンピュートプール** — ストレージドメインと物理ファブリックの接続性から導出される。ネットワークドメインの制約がないため、同一ストレージドメインに属し物理ファブリックで接続されたホスト群がコンピュートプールとなる。ライブマイグレーション可能な範囲はこの導出結果に一致する。

```
┌─────────────────────────────────────────────────┐
│             ストレージドメイン A                    │
│  ┌─────────────────────────────────────────┐    │
│  │   コンピュートプール A                    │    │
│  │   (導出: ストレージドメイン A のホスト群)  │    │
│  │                                          │    │
│  │  [Host-1] [Host-2] [Host-3] [Host-7]     │    │
│  └─────────────────────────────────────────┘    │
│                                                   │
└───────────────────────────────────────────────────┘
```

### ドメインと到達性の分離

管理プレーンの境界（ドメイン）とデータプレーンの到達性は別概念として扱う。

- **ストレージドメイン ⊆ ストレージ到達性** — レプリケーションによりドメインを跨いだデータ到達が可能
- **ネットワーク到達性** — Geneveオーバーレイにより、L3ファブリックで接続された全ホスト間で通信可能。専用の管理ドメインは不要
- **コンピュートプール** — ストレージドメインの範囲で通常のライブマイグレーションとDRSが動作する
- **拡張コンピュートプール** — ストレージドメイン跨ぎ。動作するが遅くリスクが高い。明示的な運用操作として扱う

### 障害トポロジ（ロケーション）

物理ロケーションの階層をツリー構造でモデル化する。

```
サイト（データセンター）
├── フロア/ホール
│   ├── ラック列
│   │   ├── ラック
│   │   │   ├── ユニット位置 → Host
│   │   │   └── ユニット位置 → Host
│   │   └── ラック
│   └── ラック列
└── フロア/ホール
```

各ノードに障害共有の属性（電源系統ID、上位スイッチID等）を持たせる。

**ゾーン**（ユーザに見せる障害ドメイン）はこのツリーのある階層をグルーピングしたものとして導出する。独立概念として定義するのではなく、ロケーションツリーから導出する。

ファシリティの詳細はNetBox等の外部CMDBに委ね、Cirrusは障害共有関係のツリーだけを同期アダプタ経由でインポートする。
