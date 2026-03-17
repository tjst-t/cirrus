# シーケンス図

## テナント作成からVM起動までの全体フロー

```mermaid
sequenceDiagram
    actor Admin
    actor User
    participant API as Controller<br/>API :8080
    participant DB as PostgreSQL
    participant Sched as Scheduler
    participant Agent as Worker Agent<br/>(gRPC)
    participant Compute as Compute<br/>(libvirt)
    participant Storage as Storage<br/>(local/iSCSI)
    participant Net as Network<br/>(OVS)

    rect rgb(230, 245, 255)
        Note over Admin, DB: Phase 1 — テナント(プロジェクト)作成
        Admin->>API: POST /api/v1/projects<br/>{"name": "team-alpha", "quota_vcpus": 20, ...}
        API->>DB: INSERT INTO projects
        DB-->>API: project_id
        API-->>Admin: 201 Created {id, name, quotas}

        Admin->>API: POST /api/v1/projects/{id}/api-keys<br/>{"name": "default"}
        API->>API: ランダムキー生成 + hash
        API->>DB: INSERT INTO api_keys
        API-->>Admin: 201 Created {key: "cirrus_xxxxxxxxxxxx"}
    end

    Note over Admin, User: 管理者がAPI Keyをユーザに渡す

    rect rgb(230, 255, 230)
        Note over User, Net: Phase 2 — テナントネットワーク作成
        User->>API: POST /api/v1/networks<br/>X-API-Key: cirrus_xxx<br/>{"name": "app-net", "cidr": "10.100.0.0/24"}
        API->>API: 認証 → project_id 特定
        API->>API: gateway自動算出 (10.100.0.1)
        API->>DB: 空きVNI取得
        DB-->>API: vni = 100
        API->>DB: INSERT INTO networks
        API-->>User: 201 Created {id, name, cidr, gateway, vni}
    end

    rect rgb(255, 245, 230)
        Note over User, Storage: Phase 3 — イメージ確認
        User->>API: GET /api/v1/images
        API->>DB: SELECT images WHERE project_id = ? OR project_id IS NULL
        DB-->>API: [{id, name: "ubuntu-24.04", ...}]
        API-->>User: 200 OK [images]
    end

    rect rgb(255, 230, 230)
        Note over User, Net: Phase 4 — VM作成 (非同期)

        User->>API: POST /api/v1/vms<br/>{"name": "web-01", "image_id": "...",<br/> "vcpus": 2, "ram_mb": 4096, "disk_gb": 20,<br/> "networks": [{"network_id": "..."}]}

        API->>DB: クォータチェック<br/>SELECT SUM(vcpus), SUM(ram_mb) FROM vms
        DB-->>API: used: 0 vcpus, 0 ram / quota: 20, 51200
        API->>API: MAC生成 (02:xx:xx:xx:xx:xx)
        API->>DB: 空きIP検索
        DB-->>API: 10.100.0.2
        API->>DB: INSERT INTO ports
        API->>DB: INSERT INTO vms (status=scheduling)
        API-->>User: 202 Accepted {id, status: "scheduling", ports: [...]}

        Note over API, Net: 以降 非同期ジョブ

        API->>Sched: Schedule(vm)
        Sched->>DB: SELECT workers + VM集計
        Sched->>Sched: スコアリング → worker-01 選択
        Sched-->>API: worker_id = worker-01
        API->>DB: UPDATE vms SET worker_id, status=building

        API->>Agent: gRPC: CreateVM

        Agent->>Storage: CreateDisk(vm_id, base, 20GB)
        alt local backend
            Storage->>Storage: qemu-img create -b base.qcow2
        else iSCSI backend
            Storage->>Storage: SAN API → ボリューム作成
            Storage-->>Agent: DiskResult{DriverData: {iqn, lun}}
        end
        Storage-->>Agent: OK

        Agent->>Net: AttachPort(port_id, mac, vni=100)
        Net->>Net: local_vlan_tag割り当て
        Net->>Net: ovs-vsctl set port tag
        Net->>Net: ovs-ofctl add-flow (VNI↔VLAN変換)
        Net-->>Agent: OK

        Agent->>Compute: CreateVM(spec)
        Compute->>Compute: cloud-init ISO生成
        Compute->>Compute: domain XML生成
        Compute->>Compute: libvirt define + start
        Compute-->>Agent: OK

        Agent-->>API: gRPC Response: success
        API->>DB: UPDATE vms SET status=active
        API->>DB: UPDATE ports SET status=active
    end

    rect rgb(245, 235, 255)
        Note over User, DB: Phase 5 — ステータス確認
        User->>API: GET /api/v1/vms/{id}
        API->>DB: SELECT vm + ports
        API-->>User: 200 OK {status: "active", ports: [{ip: "10.100.0.2"}]}
    end
```

## VM作成の内部フロー（詳細）

```
User → POST /api/v1/vms
  → API: 認証・クォータチェック
  → Scheduler: worker選定 (worker-01を選択)
  → DB: vm record作成 (status=SCHEDULING)
  → Controller→Worker-01: CreateVM RPC
    → Worker: storage.CreateDisk()
    → Worker: network.AttachPort()
    → Worker: compute.CreateVM()
  → DB: status=ACTIVE, worker=worker-01
  → Response: 201 Created {id, ip, status}
```

## Reconcile（worker起動時）

```
Worker起動
  → controller.GetWorkerState(workerID)
  → libvirtから実際のVM一覧取得
  → OVSから実際のポート一覧取得
  → 差分比較
    → DBにあるがlibvirtにない → エラー報告
    → libvirtにあるがDBにない → 孤立VMとして削除
```
