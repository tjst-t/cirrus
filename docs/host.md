# ホスト

## ホストの属性モデル

ホストを中心エンティティとし、複数の関心事を属性・関連として表現する。

- **所属（到達性）** — どのストレージドメイン、ネットワークドメインに到達可能か → コンピュートプール導出
- **能力（capability）** — CPU世代、GPU種類・数、SR-IOV対応NIC、NUMAトポロジ（ノードごとのリソース構成）等
- **状態（ホストプロファイル）** — ソフトウェア構成のdesired state
- **ロケーション** — 障害トポロジツリー上のパス
- **稼働状態** — 正常、メンテナンス中、ドレイン中、障害中、退役予定等

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
