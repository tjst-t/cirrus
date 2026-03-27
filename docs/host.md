# ホスト

## ホストの属性モデル

ホストを中心エンティティとし、複数の関心事を属性・関連として表現する。

- **所属（到達性）** — どのストレージドメイン、ネットワークドメインに到達可能か → コンピュートプール導出
- **能力（capability）** — CPU世代、GPU種類・数、SR-IOV対応NIC、NUMAトポロジ（ノードごとのリソース構成）等
- **状態（ホストプロファイル）** — ソフトウェア構成のdesired state
- **ロケーション** — 障害トポロジツリー上のパス
- **稼働状態** — 正常、メンテナンス中、ドレイン中、障害中、退役予定等

## ホストライフサイクル

### 登録フロー

workerがcontrollerに自己登録する push 型の方式を採用する。

```
1. 管理者が物理サーバに worker をインストール
   → 設定ファイルに controller アドレス + 登録トークンを記述
   → トポロジ情報（network-domain, storage-domains, location）も設定

2. worker 起動
   → controller に gRPC 接続
   → RegisterHost RPC で自ホスト情報 + トポロジ申告を送信
   → controller がトークン検証 → DB に registering 状態で登録
   → 初回登録時のみ、申告されたトポロジを自動関連付け（best-effort）
   → 割り当てられた UUID を worker に返却

3. worker は UUID で heartbeat 送信を開始

4. 管理者が承認
   → cirrusctl admin host activate <name>
   → registering → active（VM配置対象になる）
```

- 登録トークンは共有シークレット（将来的にmTLS移行可能な設計）
- 同一ホスト名での重複登録は冪等（既存UUIDを返す）
- 未登録・未承認のホストはスケジューラの配置対象外
- **トポロジ申告は初回登録時のみ適用**。再登録（worker再起動）時は既存の関連付けを保持し、管理者がAPIで修正した設定を上書きしない
- 管理者はいつでもAPI/CLIでトポロジ関連付けを修正可能

### 実運用でのデプロイフロー

実運用では NetBox（CMDB）をトポロジの信頼源とし、AWX/Ansible でホストをデプロイする。

```
NetBox (Source of Truth)
  ├─ サイト・ラック定義        → Cirrus locations（定期同期 or webhook）
  ├─ ストレージクラスタ定義    → Cirrus storage_domains
  └─ ネットワーク区画定義      → Cirrus network_domains

AWX (デプロイ自動化)
  1. 物理サーバーをプロビジョニング
  2. ネットワーク・ストレージを接続設定
  3. cirrus worker をデプロイ・起動
     └─ NetBoxから取得したトポロジ情報をworker起動パラメータに注入
        --network-domain=nd-tokyo-1
        --storage-domains=ceph-ssd-tokyo,ceph-hdd-tokyo
        --location=rack-a-03-u12
  4. Worker が自動登録 + トポロジ申告
  5. 管理者が activate（または将来的な自動承認ポリシー）
```

ドメイン・ロケーション（インフラの「枠」）は管理者が定義し、ホストは起動時にどの枠に属するかを自己申告する。これにより:

- ドメインの作成は少数かつ低頻度で、管理者が明示的に制御
- ホストの追加は頻繁で、デプロイ自動化パイプラインに組み込める
- トポロジの修正は管理者がいつでもAPI/CLIで上書き可能

### 状態一覧

| 状態 | 意味 | VM配置 | 既存VM |
|------|------|--------|--------|
| `registering` | 登録直後、初期セットアップ待ち | 不可 | なし |
| `active` | 正常稼働中 | 可 | 稼働中 |
| `draining` | 新規配置停止、既存VMの退避中 | 不可 | ライブマイグレーションで退避 |
| `maintenance` | メンテナンス中（VM不在が前提） | 不可 | なし |
| `faulty` | 障害検出（heartbeat途絶等） | 不可 | HA failover対象 |
| `retiring` | 廃止予定、復帰不可 | 不可 | なし（VM不在が前提） |

### 状態遷移図

```
                   POST /hosts
                       │
                       ▼
               ┌──────────────┐
               │ registering  │
               └──────┬───────┘
                      │ activate
                      ▼
               ┌──────────────┐
          ┌───→│    active    │←──┐
          │    └──┬───────┬───┘   │
          │       │       │       │
          │  drain│       │       │ activate
          │       ▼       │       │
          │  ┌──────────┐ │  ┌────┴────────┐
          │  │ draining │ └─→│ maintenance │
          │  └────┬─────┘    └─────────────┘
          │       │ maintenance              ▲
          │       │  (VM数=0で遷移)          │
          │       └──────────────────────────┘
          │
          │ activate
          │
     ┌────┴─────┐
     │  faulty  │  ← heartbeat途絶で自動遷移
     └──────────┘

               ┌──────────────┐
               │   retiring   │  ← maintenance からのみ遷移可
               └──────────────┘
```

### 遷移ルール

| 遷移元 | 遷移先 | 条件 |
|--------|--------|------|
| `registering` | `active` | 初回heartbeat受信済み、プロファイル適用済み |
| `active` | `draining` | 管理者操作 |
| `active` | `maintenance` | 管理者操作（VMが0台の場合のみ） |
| `active` | `faulty` | heartbeat途絶（3回連続タイムアウト） |
| `draining` | `maintenance` | 稼働VM数が0になった時点で自動遷移 |
| `draining` | `active` | 管理者操作（ドレイン取り消し） |
| `draining` | `faulty` | heartbeat途絶 |
| `maintenance` | `active` | 管理者操作 |
| `maintenance` | `retiring` | 管理者操作 |
| `faulty` | `active` | 管理者操作（障害復旧後） |
| `faulty` | `maintenance` | 管理者操作（手動修理のため） |
| `retiring` | ―（終端状態） | 復帰不可。VM配置・activate禁止 |

### 制約

- **draining中のVM配置禁止**: スケジューラがdraining状態のホストを候補から除外する
- **maintenance遷移はVM不在が前提**: active→maintenanceは稼働VMが0台の場合のみ許可。VMがある場合はまずdrainを経由する
- **retiring遷移はmaintenanceからのみ**: VMが確実に不在の状態からのみ廃止に移行できる
- **faulty自動遷移**: controllerがheartbeat監視し、3回連続タイムアウト（デフォルト30秒無応答）で自動的にfaultyに遷移。faulty遷移時にHA failoverをトリガーする
- **retiring は終端**: 一度retiringに入ったホストはactiveに戻せない。物理的な撤去後にDBから削除する

## Capability

ホストのハードウェア能力を構造化データとして宣言する。VMの要件とcapability-based matchingで対応。

- CPU型番、世代、命令セット拡張（AVX-512等）
- メモリ容量
- GPUの種類と数
- NVMe/SSDの有無
- SR-IOV対応NICの有無
- NUMAトポロジ（ノードごとのCPU、メモリ、GPU、NICの配置）

NUMAトポロジはフラットな属性ではなく構造化データとして持つ。「GPU 4枚」ではなく「NUMAノード0にGPU 2枚 + メモリ128GB、NUMAノード1にGPU 2枚 + メモリ128GB」の形でスケジューラがNUMA-awareな配置を行う。

### Capability構造の例

```json
{
  "cpu": {
    "model": "Intel Xeon Platinum 8480+",
    "generation": "sapphire_rapids",
    "extensions": ["avx512", "amx"]
  },
  "numa_nodes": [
    {
      "id": 0,
      "cpus": 56,
      "memory_mb": 131072,
      "gpus": [
        {"model": "NVIDIA H100", "vram_mb": 81920},
        {"model": "NVIDIA H100", "vram_mb": 81920}
      ],
      "nics": [
        {"model": "ConnectX-7", "sriov": true, "bandwidth_gbps": 200}
      ]
    },
    {
      "id": 1,
      "cpus": 56,
      "memory_mb": 131072,
      "gpus": [
        {"model": "NVIDIA H100", "vram_mb": 81920},
        {"model": "NVIDIA H100", "vram_mb": 81920}
      ],
      "nics": [
        {"model": "ConnectX-7", "sriov": true, "bandwidth_gbps": 200}
      ]
    }
  ],
  "storage": {
    "nvme": true,
    "local_ssd_gb": 3200
  }
}
```

## リソース管理

CPU、メモリ等のオーバーコミット可能なリソースとGPU等の排他リソースを「物理量 × オーバーコミット率 = 割当可能量」で統一的に扱う。GPUはオーバーコミット率1.0のリソース。リソース種別ごとに別の割当ロジックを持たず、パラメータの違いで吸収する。

| リソース | 物理量 | 典型的なオーバーコミット率 | 割当可能量 |
|----------|--------|---------------------------|------------|
| vCPU | 56コア | 4.0 | 224 vCPU |
| メモリ | 128 GB | 1.5 | 192 GB |
| GPU | 4枚 | 1.0（排他） | 4枚 |
| ローカルSSD | 3.2 TB | 1.0 | 3.2 TB |

## ホストプロファイル

ソフトウェア構成のdesired stateを定義する。

- カーネルバージョン
- ハイパーバイザーバージョン
- エージェント群のバージョン（ovn-controller含む）
- カーネルパラメータ
- ドライババージョン

同一capabilityのホスト群に同一プロファイルを適用する傾向が強い（GPU搭載サーバとCPUのみのサーバではドライバが異なる）。

プロファイルの適用はhookでAWX経由。ファームウェア（BIOS/UEFI、BMC、NIC、GPU）もプロファイルのレイヤーとして認識しておく。

### プロファイルの例

```yaml
name: gpu-host-v2.3
target_capability_match:
  gpu_model: "NVIDIA H100"

software:
  kernel: "6.1.94"
  hypervisor: "qemu-8.2.2"
  ovn_controller: "24.03.2"
  kernel_params:
    - "intel_iommu=on"
    - "hugepages=1024"

drivers:
  nvidia: "550.54.15"
  mlx5_core: "24.01"

firmware:
  bios: "2.8.1"
  bmc: "1.15.0"
```

## ロールアウト

プロファイルを新バージョンに更新する際の展開戦略。

- プロファイルグループ（同一capabilityのホスト群）にロールアウトポリシーを適用
- ゾーン単位でカナリア的に展開（ゾーン1に適用→問題なければゾーン2）
- OVNの場合は中央クラスタを先にアップデートし、ovn-controllerを順次更新する順序制約あり
- メンテナンス操作の種類ごとに影響範囲を到達性ドメインから導出する

```
ロールアウトポリシー例:
  strategy: canary
  batch_size: 10%
  pause_between_batches: 30m
  rollback_on:
    - host_health_check_failure
    - vm_error_rate > 1%
  zone_order: [zone-a, zone-b, zone-c]
```

## プレースメントとDRS

初回配置、HA failover、DRSは本質的に同じプレースメント問題。トリガーと時間制約が異なるだけで同一のスケジューラを通す。

### スケジューラの処理順

1. コンピュートプール（到達性フィルタ）
2. Capability（要件マッチング）
3. プロファイル状態（異常ホスト除外）
4. ロケーション制約（アフィニティ/アンチアフィニティルール）
5. DRSポリシー
6. （ホスト, バックエンド）ペアのスコアリングで最終決定

### アンチアフィニティ

アンチアフィニティの指定はロケーション階層に対して行う:

- **soft anti-affinity** — 可能なら別ラックに、無理なら同一ラックでも許容
- **hard anti-affinity at rack level** — 必ず別ラックに配置
- **hard anti-affinity at site level** — 必ず別サイトに配置

### DRS

DRS有効なコンピュートプールはユーザに集約リソースとして見せるが、スケジューラ内部ではホスト単位・NUMAノード単位の物理制約を保持する。DRSは「複数VMを複数ホストにどう再配分するか」の最適化問題で、初回配置やHAとは判断ロジックが別レイヤーになる。
