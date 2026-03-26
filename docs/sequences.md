# シーケンス図

## テナント作成からVM起動までの全体フロー

```mermaid
sequenceDiagram
    actor InfraAdmin as インフラ管理者
    actor OrgAdmin as 組織管理者
    actor User as テナントユーザ
    participant API as Controller API
    participant Auth as Authorizer
    participant DB as PostgreSQL
    participant Sched as Scheduler
    participant Host as Host Agent
    participant OVN as OVN Northbound DB
    participant StorageDrv as Storage Driver

    rect rgb(230, 245, 255)
        Note over InfraAdmin, DB: Phase 1 — 組織・テナント作成
        InfraAdmin->>API: POST /api/v1/organizations<br/>{"name": "ACME Corp", "quota_vcpus": 500, ...}
        API->>Auth: authorize(infra_admin, create, organization)
        Auth-->>API: allow
        API->>DB: INSERT INTO organizations
        API-->>InfraAdmin: 201 Created

        OrgAdmin->>API: POST /api/v1/organizations/{org_id}/tenants<br/>{"name": "dev", "quota_vcpus": 100, ...}
        API->>Auth: authorize(org_admin, create, tenant)
        Auth-->>API: allow
        API->>DB: INSERT INTO tenants
        API-->>OrgAdmin: 201 Created

        OrgAdmin->>API: POST /api/v1/tenants/{id}/role-assignments<br/>{"user_id": "...", "role": "tenant_admin"}
        API->>DB: INSERT INTO role_assignments
        API-->>OrgAdmin: 201 Created
    end

    rect rgb(230, 255, 230)
        Note over User, OVN: Phase 2 — ネットワーク作成
        User->>API: POST /api/v1/networks<br/>X-Tenant-ID: {tenant_id}<br/>{"name": "app-net"}
        API->>Auth: authorize(user, create, network)
        Auth-->>API: allow
        API->>DB: INSERT INTO networks
        API->>OVN: Create Logical Switch
        API-->>User: 201 Created

        User->>API: POST /api/v1/networks/{id}/subnets<br/>{"cidr": "10.100.0.0/24", "gateway": "10.100.0.1",<br/>"dhcp_range_start": "10.100.0.10", "dhcp_range_end": "10.100.0.254"}
        API->>OVN: Create DHCP Options
        API->>DB: INSERT INTO subnets
        API-->>User: 201 Created

        User->>API: POST /api/v1/security-groups<br/>{"name": "web-sg"}
        API->>DB: INSERT INTO security_groups
        API-->>User: 201 Created

        User->>API: POST /api/v1/security-groups/{id}/rules<br/>{"direction": "ingress", "protocol": "tcp",<br/>"port_range_min": 80, "port_range_max": 80, "remote_ip_prefix": "0.0.0.0/0"}
        API->>OVN: Create ACL
        API->>DB: INSERT INTO security_group_rules
        API-->>User: 201 Created
    end

    rect rgb(255, 230, 230)
        Note over User, StorageDrv: Phase 3 — VM作成 (非同期)

        User->>API: POST /api/v1/vms<br/>{"name": "web-01", "vcpus": 2, "ram_mb": 4096,<br/>"volumes": [{"volume_type_id": "...", "size_gb": 20,<br/>"boot": true, "template_id": "..."}],<br/>"networks": [{"network_id": "...", "security_group_ids": ["..."]}]}

        API->>Auth: authorize(user, create, vm)
        API->>DB: クォータチェック（テナント + 組織）
        DB-->>API: quota OK
        API->>DB: INSERT INTO volumes (status=creating)
        API->>DB: INSERT INTO ports (IP/MAC払い出し)
        API->>OVN: Create Logical Switch Port
        API->>DB: INSERT INTO vms (status=scheduling)
        API-->>User: 202 Accepted {id, status: "scheduling", volumes, ports}

        Note over API, StorageDrv: 以降 非同期ジョブ

        API->>Sched: Schedule(vm)
        Sched->>DB: ボリュームタイプ要件からバックエンド列挙
        Sched->>DB: バックエンド到達可能なホスト列挙
        Sched->>Sched: Capabilityマッチ
        Sched->>Sched: プロファイル状態チェック
        Sched->>Sched: ロケーション制約（アンチアフィニティ等）
        Sched->>Sched: (ホスト, バックエンド)ペア スコアリング
        Sched-->>API: host_id, backend_id

        API->>DB: UPDATE vms SET host_id, status=building
        API->>DB: UPDATE volumes SET backend_id

        API->>StorageDrv: CreateVolume(from template)
        alt テンプレートキャッシュあり
            StorageDrv->>StorageDrv: clone from cache
        else テンプレートキャッシュなし
            StorageDrv->>StorageDrv: copy template to backend
            StorageDrv->>StorageDrv: clone from copied template
        end
        StorageDrv-->>API: volume ready

        API->>Host: CreateVM(spec, volumes, ports)
        Host->>Host: domain XML生成 + cloud-init
        Host->>Host: libvirt define + start
        Host-->>API: success

        API->>DB: UPDATE vms SET status=active
        API->>DB: UPDATE volumes SET status=in_use
        API->>DB: UPDATE ports SET status=active
    end

    rect rgb(245, 235, 255)
        Note over User, DB: Phase 4 — ステータス確認
        User->>API: GET /api/v1/vms/{id}
        API->>DB: SELECT vm + volumes + ports
        API-->>User: 200 OK {status: "active", volumes: [...], ports: [{ip: "10.100.0.5"}]}
    end
```

## スケジューラの処理フロー

```
VM配置要求
│
├─ 1. コンピュートプール絞り込み
│     ├─ ボリュームタイプ → バックエンド候補
│     ├─ バックエンド到達可能ホスト
│     └─ ネットワークドメイン内ホスト
│
├─ 2. Capabilityマッチ
│     ├─ CPU要件（世代、命令セット）
│     ├─ GPU要件
│     ├─ NUMAトポロジ要件
│     └─ SR-IOV要件
│
├─ 3. 状態フィルタ
│     ├─ operational_state = active のみ
│     └─ profile_status = in_sync のみ
│
├─ 4. ロケーション制約
│     ├─ アンチアフィニティルール
│     └─ アフィニティルール
│
├─ 5. スコアリング
│     ├─ ホスト: リソース空き率
│     ├─ ホスト: NUMAノード空き
│     ├─ バックエンド: 容量空き率
│     ├─ バックエンド: IOPS余裕
│     └─ テンプレートキャッシュの有無
│
└─ 6. 最終決定 → (host_id, backend_id) ペア
```

## ライブマイグレーション

```mermaid
sequenceDiagram
    participant Trigger as トリガー<br/>(DRS/HA/Drain/手動)
    participant Sched as Scheduler
    participant DB as PostgreSQL
    participant SrcHost as 移行元ホスト
    participant DstHost as 移行先ホスト
    participant OVN as OVN NB DB

    Trigger->>Sched: MigrateVM(vm_id, reason)
    Sched->>DB: VM情報 + ボリューム情報取得
    Sched->>Sched: 移行先候補の選定<br/>(同一コンピュートプール内)
    Sched-->>DB: UPDATE vms SET status=migrating, target_host_id

    Sched->>DstHost: PrepareMigration(vm_spec)
    DstHost-->>Sched: ready

    Sched->>SrcHost: StartMigration(vm_id, dst_host)
    SrcHost->>DstHost: libvirt live migration
    Note over SrcHost, DstHost: メモリ転送（iterative pre-copy）
    DstHost-->>Sched: migration complete

    Sched->>OVN: Update port bindings<br/>(chassis → dst_host)
    Sched->>DB: UPDATE vms SET host_id=dst_host, status=active
```

## ストレージドレイン

```
ドレイン開始
│
├─ バックエンドstatus → draining
├─ 新規ボリューム作成を停止
│
├─ 依存関係を考慮した移行順序算出
│   ├─ 依存なしボリューム → 即座に移行
│   ├─ クローン元スナップショット → フラット化 → 移行
│   └─ 親ボリューム → 子のフラット化完了後に移行
│
├─ ボリュームごとに:
│   ├─ 移行先バックエンド選定 (スケジューラ)
│   ├─ ストレージライブマイグレーション実行
│   └─ 帯域制限の適用
│
├─ 進捗可視化
│   ├─ 残りボリューム数
│   ├─ 残りデータ量
│   └─ 推定完了時間
│
└─ 全ボリューム移行完了 → readonly → retired
```
