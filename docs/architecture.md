# アーキテクチャ

## 概要

Cirrusは単一のGoバイナリで、起動時のロール指定によりcontrollerまたはworkerとして動作します。

```bash
cirrus controller --listen=0.0.0.0:8080 --db=postgres://...
cirrus worker --controller=controller:9090 --name=worker-01
cirrus worker --controller=controller:9090 --name=worker-02
```

## コンポーネント配置

```
┌─────────────────────────────────────────────┐
│  Controller (1台)                            │
│                                              │
│  ┌──────────┐  ┌───────────┐  ┌──────────┐ │
│  │ API (HTTP)│  │ Scheduler │  │ Network  │ │
│  │ :8080    │  │           │  │ Manager  │ │
│  └────┬─────┘  └─────┬─────┘  └────┬─────┘ │
│       │               │             │        │
│  ┌────┴───────────────┴─────────────┴─────┐ │
│  │         State Store (PostgreSQL)        │ │
│  └─────────────────────────────────────────┘ │
│       │ gRPC :9090                           │
└───────┼──────────────────────────────────────┘
        │
   ┌────┴────┐
   │         │
┌──▼──────────┐  ┌───────────────┐
│ Worker-01   │  │ Worker-02     │
│             │  │               │
│ ┌─────────┐│  │ ┌─────────┐  │
│ │Compute  ││  │ │Compute  │  │
│ │(libvirt)││  │ │(libvirt)│  │
│ └─────────┘│  │ └─────────┘  │
│ ┌─────────┐│  │ ┌─────────┐  │
│ │Network  ││  │ │Network  │  │
│ │(OVS)    ││  │ │(OVS)    │  │
│ └─────────┘│  │ └─────────┘  │
│ ┌─────────┐│  │ ┌─────────┐  │
│ │Agent    ││  │ │Agent    │  │
│ │(gRPC)   ││  │ │(gRPC)   │  │
│ └─────────┘│  │ └─────────┘  │
└─────────────┘  └───────────────┘
```

## コンポーネント責務

### Controller側

| パッケージ | 責務 |
|---|---|
| `api` | REST API。テナント認証(API Key)、VM/Network/Image CRUD |
| `scheduler` | 配置決定。workerのリソース(CPU, RAM, 残容量)を見てスコアリング |
| `netcontroller` | 論理ネットワークの状態管理。VNI割り当て、サブネットCIDR管理、workerへのOVS設定指示 |
| `state` | PostgreSQL上のモデル定義。projects, vms, networks, workers, images |

### Worker側

| パッケージ | 責務 |
|---|---|
| `agent` | gRPCサーバ。controllerからの指示を受けてcompute/netに委譲 |
| `compute` | libvirt経由でQEMU/KVM操作。VM定義XML生成、起動、停止、削除 |
| `netagent` | OVSブリッジ/ポート操作。VXLANトンネル作成、VMのtapデバイス接続、フロー設定 |

## gRPC インターフェース（controller → worker）

gRPCの向きは **controller → worker** 。worker側がgRPCサーバになり、controllerがworkerに指示を出す。

```protobuf
service WorkerAgent {
  // Lifecycle
  rpc Register(RegisterReq) returns (RegisterResp);
  rpc Heartbeat(HeartbeatReq) returns (HeartbeatResp);

  // Compute
  rpc CreateVM(CreateVMReq) returns (CreateVMResp);
  rpc DeleteVM(DeleteVMReq) returns (DeleteVMResp);
  rpc GetVMStatus(GetVMStatusReq) returns (VMStatus);

  // Network
  rpc ConfigurePort(ConfigurePortReq) returns (ConfigurePortResp);
  rpc ConfigureTunnel(ConfigureTunnelReq) returns (ConfigureTunnelResp);
}
```

Heartbeatはworkerからcontrollerへ定期的にHTTP POSTで送る方式（controllerがworkerのIPを管理しなくてよい）。

## ディレクトリ構成

```
cirrus/
├── cmd/cirrus/
│   └── main.go                # controller/worker サブコマンド
├── internal/
│   ├── api/                    # REST API handlers
│   ├── scheduler/              # 配置ロジック
│   ├── netcontroller/          # ネットワーク状態管理
│   ├── state/                  # DB models, migrations
│   ├── agent/                  # worker側 gRPC server
│   │
│   ├── compute/
│   │   ├── driver.go           # interface定義
│   │   └── libvirt/
│   │       └── libvirt.go      # Driver実装
│   │
│   ├── network/
│   │   ├── provider.go         # interface定義
│   │   ├── ovs/
│   │   │   └── ovs.go          # Provider実装
│   │   └── linuxbridge/        # 将来
│   │
│   ├── storage/
│   │   ├── backend.go          # interface定義
│   │   ├── local/
│   │   │   └── local.go        # ローカルqcow2
│   │   ├── ceph/               # 将来
│   │   └── nfs/                # 将来
│   │
│   └── image/
│       ├── store.go            # interface定義
│       ├── localfs/
│       │   └── localfs.go      # ローカルファイル保存
│       └── s3/                 # 将来
│
├── proto/
│   └── agent.proto             # gRPC定義
├── go.mod
└── go.sum
```

## 設定ファイル

```yaml
# cirrus.yaml
role: worker
controller: 192.168.1.10:9090
name: worker-01

compute:
  driver: libvirt

network:
  driver: ovs
  ovs:
    bridge: br-int
    local_ip: 192.168.1.11

storage:
  driver: local
  local:
    disk_dir: /var/lib/cirrus/disks
    image_dir: /var/lib/cirrus/images
```

## 依存注入

DIフレームワークは使用せず、main.goでのコンストラクタ呼び出しで組み立てる。

```go
func runWorker(cfg WorkerConfig) {
    var networkProvider network.Provider
    switch cfg.Network.Driver {
    case "ovs":
        networkProvider = ovs.New(cfg.Network.OVS)
    case "linuxbridge":
        networkProvider = linuxbridge.New(cfg.Network.Bridge)
    }

    var storageBackend storage.Backend
    switch cfg.Storage.Driver {
    case "local":
        storageBackend = local.New(cfg.Storage.Local)
    case "ceph":
        storageBackend = ceph.New(cfg.Storage.Ceph)
    }

    computeDriver := libvirt.New(libvirt.Config{URI: "qemu:///system"})
    agent := agent.New(computeDriver, networkProvider, storageBackend)
    agent.Serve(cfg.ListenAddr)
}
```
