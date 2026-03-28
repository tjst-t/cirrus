# テナント向けリソースモデル

## 背景と目的

Cirrus はインフラ管理者向けのモデル（Location, Network Domain, Storage Domain, Compute Pool）が充実している一方、テナントユーザー向けの抽象レイヤーが欠けている。テナントユーザーが物理インフラの詳細（どの OVN クラスタか、どのストレージバックエンドか）を意識せずにリソースを操作できるよう、適切な抽象化を設計する。

### 設計原則

AWS / Azure / OpenStack の共通パターンに従う:

1. **管理者がインフラをカテゴリ化**し、テナント向け抽象（AZ, Flavor, Volume Type）を作成する
2. **テナントはカテゴリ名だけで操作**し、物理詳細を知る必要がない
3. **スケジューラが実行時にマッチング**し、抽象から物理への解決を行う
4. **AZ が唯一の橋渡し概念** — テナントに見せる最小限のインフラヒント

### AZ と Fault Domain の関係

Cirrus には似た名前の2つの概念がある。これらは異なるレイヤーに属し、用途が異なる:

| 概念 | 対象 | 性質 | 用途 |
|---|---|---|---|
| **Availability Zone (AZ)** | テナント | 独立エンティティ（管理者が作成） | リソース配置先の選択。ND/SD と紐付く |
| **Fault Domain** | 管理者 | 導出ビュー（Location ツリーから動的集計） | ロールアウト計画、障害影響分析 |

AZ は Fault Domain の上位概念ではなく、別レイヤーの概念。AZ はテナント向け API に公開し、Fault Domain は管理者向け運用ツールとして内部に留める。

## テナントに見えるもの vs 見えないもの

| テナントに見える | テナントに見えない（管理者のみ） |
|---|---|
| Availability Zone | Location (site/floor/rack/unit), Fault Domain |
| Flavor | Host Capability, NUMA topology |
| Volume Type | Storage Backend, Storage Domain |
| Network（名前 + AZ で作成） | Network Domain (OVN cluster) |
| Subnet, Port, Router, SG, FIP | OVN 内部構造 |
| Quota（使用量の確認） | Compute Pool (SD x ND 導出) |
| VM, Volume, Snapshot, Template | Host, OperationalState, Profile |

## Availability Zone (AZ)

### 概要

AZ はテナントが「どこにリソースを配置するか」を選択するための概念。物理的な障害分離境界を抽象化する。

### 設計判断

AZ は**独立エンティティ**として設計し、Location を参照する。Location（site レベル）に 1:1 で対応するのが一般的だが、大規模サイトで分割する柔軟性を持つ。

```
小〜中規模（数百〜数千台/サイト）:
  1 サイト = 1 AZ = 1 OVN クラスタ

大規模（数千〜数万台/サイト）:
  1 サイト = N AZ = N OVN クラスタ
  AZ 内はフル L2 接続可能
  AZ 間は OVN-IC で L3 ルーティング
```

### データモデル

```sql
CREATE TABLE availability_zones (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(63) UNIQUE NOT NULL,  -- "tokyo-1a" (テナントに見える名前)
    description       TEXT,
    location_id       UUID NOT NULL REFERENCES locations(id),
    network_domain_id UUID NOT NULL UNIQUE REFERENCES network_domains(id),  -- 1:1 対応（UNIQUE制約）
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- AZ と Storage Domain は N:M（同一 AZ 内に複数バックエンドタイプ）
CREATE TABLE az_storage_domains (
    az_id             UUID NOT NULL REFERENCES availability_zones(id) ON DELETE CASCADE,
    storage_domain_id UUID NOT NULL REFERENCES storage_domains(id) ON DELETE CASCADE,
    PRIMARY KEY (az_id, storage_domain_id)
);
```

### AZ と OVN の関係

1 AZ = 1 OVN クラスタ（Network Domain）が原則。`network_domain_id` に UNIQUE 制約を付与し、複数 AZ が同じ OVN クラスタを共有することを防ぐ。

- **AZ 内**: 全ホストが同一 OVN クラスタに所属。L2 接続（同一 Logical Switch）が可能。
- **AZ 間**: OVN-IC を使った L3 ルーティングのみ。テナントは Router を作成して AZ 間を接続する。同一ネットワークが複数 AZ にまたがることはできない。
- **根拠**: OVN の Logical Switch は単一クラスタ内でのみ L2 を提供する。同じ AZ 内のVM が L2 で繋がる、という暗黙の期待に合致する。

OVN 1 クラスタの実用上限は約 500〜1,000 chassis（OVSDB Relay 利用で数千台まで）。オンプレ IaaS の一般的な規模では 1 サイト = 1 AZ で十分。

### ホストと AZ のマッピング

ホストは AZ に**直接紐付くカラムを持たない**。代わりに Location の親子関係から導出する:

- AZ は `location_id`（例: site "tokyo-dc"）を持つ
- ホストは `location_id`（例: rack "tokyo-dc/floor-1/rack-01"）を持つ
- ホストの location が AZ の location の**子孫**であれば、そのホストはその AZ に属する

```sql
-- AZ "tokyo-1" に属するホストを取得
WITH RECURSIVE subtree AS (
    SELECT id FROM locations WHERE id = <az.location_id>
    UNION ALL
    SELECT l.id FROM locations l JOIN subtree s ON l.parent_id = s.id
)
SELECT * FROM hosts WHERE location_id IN (SELECT id FROM subtree)
```

### AZ と Storage Domain の関係

1 AZ に対して N 個の Storage Domain が紐づく:

```
AZ "tokyo-1"
├── Network Domain: ovn-tokyo (1:1)
├── Storage Domain: tokyo-ssd
├── Storage Domain: tokyo-hdd
└── Hosts [100台] (location の子孫から導出)
```

テナントは Volume Type で「SSD か HDD か」を選び、AZ で「どの拠点か」を選ぶ。スケジューラが AZ 内の適切な Storage Domain + Backend を自動選定する。

### VM / Volume への AZ 記録

VM と Volume の作成時に配置先 AZ を確定し、テーブルに `az_id` カラムとして記録する。テナントが「この VM はどの AZ にあるか」を確認できるようにするため。

```sql
-- vms テーブルに追加（Sprint 7 のマイグレーションで）
ALTER TABLE vms ADD COLUMN az_id UUID REFERENCES availability_zones(id);

-- volumes テーブルに追加（Sprint 6 のマイグレーションで）
ALTER TABLE volumes ADD COLUMN az_id UUID REFERENCES availability_zones(id);
```

### デフォルト AZ

テナントが AZ を指定しなかった場合の挙動:

- **Phase 1**（single domain）: システム全体でデフォルト AZ を1つ設定（controller 設定 or DB フラグ）。AZ が1つしかないので常にそれを使用。
- **Phase 3**（マルチ AZ、Sprint 20）: テナントごとのデフォルト AZ 設定に拡張。`tenants` テーブルに `default_az_id` カラムを追加。未設定の場合は AZ 指定を必須とする。

### テナント向け API

```
GET  /api/v1/availability-zones          -- AZ 一覧（テナントが利用可能な AZ）
GET  /api/v1/availability-zones/{id}     -- AZ 詳細（名前、説明、リソース概要）
```

### 管理者向け API

```
POST   /api/v1/availability-zones        -- AZ 作成
PUT    /api/v1/availability-zones/{id}   -- AZ 更新
DELETE /api/v1/availability-zones/{id}   -- AZ 削除
POST   /api/v1/availability-zones/{id}/storage-domains  -- SD 紐付け
DELETE /api/v1/availability-zones/{id}/storage-domains/{sd_id}
```

## Flavor

### 概要

Flavor は VM のスペックテンプレート。管理者が作成し、テナントに公開する。

### 設計判断

OpenStack 方式（管理者が作成・公開）を採用。テナントは利用可能な Flavor の一覧から選択する。

### データモデル

```sql
CREATE TABLE flavors (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(63) UNIQUE NOT NULL,  -- "m1.large"
    description       TEXT,
    vcpus             INT NOT NULL,
    memory_mb         INT NOT NULL,
    root_disk_gb      INT NOT NULL DEFAULT 0,      -- 0 = ブートボリューム別途指定
    extra_specs       JSONB NOT NULL DEFAULT '{}',  -- capability マッチング用（GPU含む）
    public            BOOLEAN NOT NULL DEFAULT true,
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 非公開 Flavor のテナントアクセス制御
CREATE TABLE flavor_access (
    flavor_id UUID NOT NULL REFERENCES flavors(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    PRIMARY KEY (flavor_id, tenant_id)
);
```

### Flavor と Host Capability のマッチング

Flavor の `extra_specs` はフラットな key-value で、Host Capability のネスト構造をドット記法のパスで参照する:

```json
// Flavor extra_specs（フラットな key-value）
{
  "gpu.model": "A100",
  "gpu.min_count": 1,
  "local_ssd": true
}

// Host Capability（既存のネスト構造）
{
  "gpu": [{"model": "A100", "count": 4}],
  "local_ssd": true,
  ...
}
```

スケジューラの `CapabilityFilter` が extra_specs の各キーを Capability JSON 内のパスとして解釈し、値を比較する。

**Phase 1 では vCPU と RAM のみ**（Flavor テーブルの専用カラム）を使用し、`extra_specs` は空 `{}`。GPU 等の Capability マッチングは該当 Sprint で実装する。

### テナント向け API

```
GET /api/v1/flavors            -- 利用可能な Flavor 一覧
GET /api/v1/flavors/{id}       -- Flavor 詳細
```

### 管理者向け API

```
POST   /api/v1/flavors                          -- Flavor 作成
PUT    /api/v1/flavors/{id}                     -- Flavor 更新
DELETE /api/v1/flavors/{id}                     -- Flavor 削除
POST   /api/v1/flavors/{id}/access              -- テナントアクセス付与
DELETE /api/v1/flavors/{id}/access/{tenant_id}  -- テナントアクセス削除
```

## Volume Type

### 概要

Volume Type はストレージのパフォーマンス特性を抽象化する。管理者が作成し、Backend の Capability とマッチングする。

### 既存設計との関係

database.md に `volume_types` テーブルのスキーマが既に定義されている（`required_capabilities` JSONB, `qos_policy` JSONB）。Sprint 6 で実装予定。テナント向けの抽象化としてはこの既存設計で十分だが、以下を追加する:

- テナント向け一覧 API（利用可能な Volume Type のみ返却）
- AZ との組み合わせでバックエンド自動選定

### テナント向け API

```
GET /api/v1/volume-types          -- 利用可能な Volume Type 一覧
GET /api/v1/volume-types/{id}     -- Volume Type 詳細（名前、説明、性能特性）
```

テナントは Volume 作成時に Volume Type を指定:

```json
POST /api/v1/volumes
{
  "name": "data-vol",
  "size_gb": 100,
  "volume_type_id": "<ssd-type-uuid>",
  "az": "tokyo-1"          // optional: AZ 未指定なら VM の AZ に従う
}
```

## Network の変更

### 現状の問題

テナントがネットワーク作成時に `network_domain_id` を指定する必要がある。これは物理インフラの詳細がテナントに漏れている。

### 修正方針

`network_domain_id` をテナント API のリクエストパラメータから除去し、AZ から自動解決する。DB カラム `networks.network_domain_id` は内部で自動設定されるため残す。

```json
// Before (現状)
POST /api/v1/networks
{
  "name": "app-net",
  "network_domain_id": "<ovn-cluster-uuid>"  // テナントが知るべきでない
}

// After (修正後)
POST /api/v1/networks
{
  "name": "app-net",
  "az": "tokyo-1"           // optional: AZ 指定
}
```

- `az` 指定あり → その AZ の Network Domain を使用
- `az` 未指定 → デフォルト AZ を使用（Phase 1 では AZ が 1 つなので常にデフォルト）

### クロス AZ ネットワーク制約

1つのネットワーク（Logical Switch）は単一の AZ（= 単一の OVN クラスタ）内にのみ存在する。AZ をまたいだ L2 接続はできない。AZ 間通信が必要な場合は Router を作成し、OVN-IC 経由で L3 ルーティングする（Sprint 21）。

### 既存リソースの移行

Sprint 5 で `network_domain_id` 直接指定で作成されたネットワークは、Sprint 5.5 の AZ 導入時にマイグレーションで対応する。デフォルト AZ 作成後、既存ネットワークの `network_domain_id` に一致する AZ を紐付ける。

### Phase 1 での実装

Phase 1 は single domain なので:
- AZ は 1 つ（`make serve` で自動作成）
- `az` パラメータは省略可能（デフォルト AZ を使用）
- `network_domain_id` はテナント API のリクエストから除去、内部で AZ から自動設定

## VM 作成の変更

テナントが VM を作成する際のリクエスト:

```json
POST /api/v1/vms
{
  "name": "web-1",
  "flavor_id": "<m1.large-uuid>",
  "az": "tokyo-1",              // optional
  "network_id": "<app-net-uuid>",
  "volume_type_id": "<ssd-uuid>",
  "boot_volume_size_gb": 50,
  "user_data": "..."
}
```

スケジューラの解決フロー:

```
1. AZ → Network Domain + Storage Domains を取得
2. Flavor → vCPU/RAM でホストフィルタ（Phase 1）、extra_specs で Capability フィルタ（将来）
3. Volume Type → required_capabilities で Backend フィルタ
4. AZ 内の到達可能ホスト（Location 子孫）でスコアリング
5. → (host_id, backend_id) を決定
```

## テナント API まとめ

### リソース作成時の指定

| リソース | テナントが指定 | テナントが指定不要（自動解決） |
|---|---|---|
| Network | name, az(optional) | network_domain_id |
| Subnet | cidr, gateway, dhcp_range, dns | - |
| Port | network_id | subnet(自動選択), IP/MAC(IPAM) |
| VM | name, flavor, az(optional), network, volume_type, size | host, backend, compute_pool |
| Volume | name, volume_type, size, az(optional) | backend, storage_domain |
| Router | name | - |
| Security Group | name, rules | - |
| Floating IP | - | external IP pool |

### 全テナント向け API 一覧

```
# 読み取り専用（テナント参照）
GET /api/v1/availability-zones
GET /api/v1/flavors
GET /api/v1/volume-types

# ネットワーク CRUD
POST/GET/DELETE /api/v1/networks
POST/GET/DELETE /api/v1/networks/{id}/subnets
GET/DELETE      /api/v1/subnets/{id}
POST/GET/DELETE /api/v1/ports
POST/GET/DELETE /api/v1/routers
POST/GET/DELETE /api/v1/security-groups
POST/GET/DELETE /api/v1/floating-ips

# コンピュート CRUD
POST/GET/DELETE /api/v1/vms
POST            /api/v1/vms/{id}/actions

# ストレージ CRUD
POST/GET/DELETE /api/v1/volumes
POST/GET/DELETE /api/v1/snapshots

# クォータ（読み取り）
GET /api/v1/tenants/{id}/quota
```

## 実装ロードマップ

| Sprint | テナントモデルの追加内容 |
|---|---|
| **5.5** (新規) | AZ エンティティ導入 + Network API から network_domain_id 除去 |
| **6** | Volume Type テナント向け一覧 API、Volume 作成時に az(optional) 指定 |
| **7** | Flavor エンティティ + VM 作成時の AZ/Flavor 指定 |
| **10** | Quota のテナント向け使用量 API |
| **20** | マルチ AZ 対応（複数 OVN クラスタ）、テナント単位のデフォルト AZ |
