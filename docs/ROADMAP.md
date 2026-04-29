# Project Roadmap: Cirrus

> IaaS プラットフォーム — VM作成・ネットワーク・ストレージ・マルチテナントを提供するモジュラーモノリス

## Progress

- Total: 51 Sprints | Done: 35 | In Progress: 0 | Remaining: 16
- [██████████████░░░░░░] 69%
- Next: S025 (DRS)

## Execution Order

S001 → S002 → S003 → S004 → S005 → S006 → S007 → S008 → S009 → S010 → S011 → S012 → S013 → S014 → S015 → S016 → S017 → S018 → S019 → S020 → S021 → S045 → S042 → S043 → S044 → S022 → S046 → S047 → S048 → S049 → S050 → S051 → S023 → S024 → S025 → S026 → S027 → S028 → S029 → S030 → S031 → S032 → S033 → S034 → S035 → S036 → S037 → S038 → S039 → S040 → S041
                                                                                                                                                                                                                                    ↑ next

---

## Sprint S001: プロジェクト骨格 [DONE]

バイナリがビルドでき、controller/worker として起動し、cirrus-sim に接続できる。Go モジュール初期化からgRPC heartbeat まで。

### Story S001-1: プロジェクト初期化 [x]

- [x] **Task S001-1-1**: Go module 初期化、ディレクトリ構成作成
- [x] **Task S001-1-2**: `cmd/cirrus/main.go` controller/worker サブコマンド（cobra）
- [x] **Task S001-1-3**: `internal/config` 設定構造体（CLIフラグで注入）
- [x] **Task S001-1-4**: Makefile: build, test, lint, proto, serve, stop, logs ターゲット
- [x] **Task S001-1-5**: `.gitignore`, `cirrus.yaml.example`

### Story S001-2: State モジュール基盤 [x]

- [x] **Task S001-2-1**: `internal/state/db.go` PostgreSQL接続（pgx）、コネクションプール
- [x] **Task S001-2-2**: `internal/state/migrations/` golang-migrate 導入、初回マイグレーション（hosts テーブル）

### Story S001-3: API 骨格 [x]

- [x] **Task S001-3-1**: `internal/api/router.go` chi HTTP ルーター
- [x] **Task S001-3-2**: `internal/api/middleware.go` RequestID、Logger、Recovery
- [x] **Task S001-3-3**: `GET /healthz` エンドポイント（DB接続チェック）

### Story S001-4: gRPC 骨格 [x]

- [x] **Task S001-4-1**: `proto/agent.proto` ControllerService（Heartbeat RPC）
- [x] **Task S001-4-2**: protoc 生成（protoc-gen-go / protoc-gen-go-grpc）
- [x] **Task S001-4-3**: `internal/controller/grpc.go` controller 側 gRPC サーバ
- [x] **Task S001-4-4**: `internal/agent/agent.go` worker 側 gRPC クライアント（10秒間隔 heartbeat）

### Story S001-5: cirrus-sim 接続確認 [x]

- [x] **Task S001-5-1**: worker 起動時に cirrus-sim へ TCP 接続確認
- [x] **Task S001-5-2**: `make serve` で cirrus-sim + controller + worker(10台) 連携起動
- [x] **Task S001-5-3**: healthz が 200 を返す

---

## Sprint S002: Identity（認証・認可・テナント管理） [DONE]

組織・テナントを API で作成でき、静的トークンで認証、RBAC で認可判定が動く。

### Story S002-1: テナントモデルの DB [x]

- [x] **Task S002-1-1**: マイグレーション: organizations, tenants, users, role_assignments テーブル
- [x] **Task S002-1-2**: `internal/identity/models.go` 構造体定義

### Story S002-2: Identity Service [x]

- [x] **Task S002-2-1**: `internal/identity/service.go` Service インターフェース定義
- [x] **Task S002-2-2**: `internal/identity/store.go` CreateOrganization, CreateTenant, AssignRole 実装
- [x] **Task S002-2-3**: RBAC 認可のユニットテスト

### Story S002-3: 認証 [x]

- [x] **Task S002-3-1**: `internal/identity/authenticator.go` Authenticator インターフェース定義
- [x] **Task S002-3-2**: 静的トークン認証実装（開発用）
- [x] **Task S002-3-3**: `internal/api/auth_middleware.go` 認証ミドルウェア

### Story S002-4: 認可 [x]

- [x] **Task S002-4-1**: `internal/identity/authorizer.go` RBAC 実装（infra_admin, org_admin, tenant_admin, tenant_member）
- [x] **Task S002-4-2**: X-Tenant-ID ヘッダからテナントスコープ解決

### Story S002-5: API エンドポイント [x]

- [x] **Task S002-5-1**: POST/GET /api/v1/organizations
- [x] **Task S002-5-2**: POST/GET /api/v1/organizations/{org_id}/tenants
- [x] **Task S002-5-3**: POST/GET/DELETE /api/v1/tenants/{id}/role-assignments
- [x] **Task S002-5-4**: 結合テスト: 認証→認可→テナント操作フロー

---

## Sprint S003: CLIクライアント（cirrusctl）基盤 [DONE]

CLI クライアントで Identity の全機能を操作できる。以降のスプリントで拡充する基盤を構築。

### Story S003-1: CLI 基盤 [x]

- [x] **Task S003-1-1**: `cmd/cirrusctl/main.go` ルートコマンド + グローバルフラグ（--endpoint, --token, --output）
- [x] **Task S003-1-2**: `internal/client/client.go` HTTP API クライアント
- [x] **Task S003-1-3**: Makefile に cirrusctl ビルド追加

### Story S003-2: Identity 操作コマンド [x]

- [x] **Task S003-2-1**: `org create/list/show`
- [x] **Task S003-2-2**: `tenant create/list/show`
- [x] **Task S003-2-3**: `role assign/list/delete`

### Story S003-3: 出力フォーマット [x]

- [x] **Task S003-3-1**: デフォルト: テーブル形式（text/tabwriter）
- [x] **Task S003-3-2**: `--output json` JSON 出力

---

## Sprint S004: Host 管理 + Worker Agent [DONE]

ホストを登録し、worker が heartbeat を送り、capability とリソースが管理される。

### Story S004-1: Host モデルの DB [x]

- [x] **Task S004-1-1**: マイグレーション: hosts, host_storage_domains テーブル
- [x] **Task S004-1-2**: capability JSONB、resource_physical JSONB、overcommit_ratios JSONB

### Story S004-2: Host Service [x]

- [x] **Task S004-2-1**: `internal/host/service.go` Service インターフェース
- [x] **Task S004-2-2**: Register, UpdateCapability, SetOperationalState, Heartbeat 実装
- [x] **Task S004-2-3**: GetAllocatable（スケジューラ向け）

### Story S004-3: Worker Agent [x]

- [x] **Task S004-3-1**: proto/agent.proto Heartbeat RPC に ResourceReport 追加
- [x] **Task S004-3-2**: `internal/agent/agent.go` 定期 heartbeat 送信（リソース情報含む）
- [x] **Task S004-3-3**: controller 側: heartbeat 受信 → hosts テーブル更新

### Story S004-4: Hypervisor 接続 [x]

- [x] **Task S004-4-1**: `internal/hypervisor/driver.go` Driver インターフェース
- [x] **Task S004-4-2**: `internal/hypervisor/libvirt.go` cirrus-sim HTTP API 経由で ListVMs, GetHostInfo

### Story S004-5: API + CLI [x]

- [x] **Task S004-5-1**: POST/GET /api/v1/hosts（管理者）
- [x] **Task S004-5-2**: POST /api/v1/hosts/{id}/actions（maintenance, activate, drain, retire）
- [x] **Task S004-5-3**: GET /api/v1/hosts/{host_id}
- [x] **Task S004-5-4**: `cirrusctl host list/show/maintenance/activate` コマンド

---

## Sprint S005: Worker 自動登録（登録トークン方式） [DONE]

worker が登録トークンで安全に controller に自己登録し、管理者の承認を経て VM 配置対象になる。

### Story S005-1: 登録トークン [x]

- [x] **Task S005-1-1**: controller に `--registration-token` フラグ追加
- [x] **Task S005-1-2**: Makefile に `REGISTRATION_TOKEN` 変数追加

### Story S005-2: Worker 自己登録 gRPC [x]

- [x] **Task S005-2-1**: proto/agent.proto RegisterHost RPC 追加
- [x] **Task S005-2-2**: worker 起動時: ホスト情報収集 → RegisterHost 呼び出し
- [x] **Task S005-2-3**: controller 側: トークン検証 → hosts テーブルに registering で INSERT
- [x] **Task S005-2-4**: RegisterHost レスポンスで host UUID 返却

### Story S005-3: Heartbeat を UUID ベースに移行 [x]

- [x] **Task S005-3-1**: worker: RegisterHost で取得した UUID を heartbeat に使用
- [x] **Task S005-3-2**: controller: heartbeat は UUID のみでマッチ（名前マッチ廃止）

### Story S005-4: 管理者承認フロー [x]

- [x] **Task S005-4-1**: `cirrusctl admin host list --pending` で registering ホスト一覧
- [x] **Task S005-4-2**: `cirrusctl admin host activate` で registering → active
- [x] **Task S005-4-3**: 承認前のホストはスケジューラ対象外

### Story S005-5: テスト [x]

- [x] **Task S005-5-1**: 無効トークンでの登録拒否テスト
- [x] **Task S005-5-2**: worker 起動→自動登録→activate→heartbeat 正常フロー
- [x] **Task S005-5-3**: 同一ホスト名の重複登録が冪等なことの確認

---

## Sprint S006: Topology（到達性ドメイン・ロケーション） [DONE]

ストレージ/ネットワークドメイン、ロケーションツリーが登録でき、コンピュートプールが導出される。

### Story S006-1: ドメインモデルの DB [x]

- [x] **Task S006-1-1**: マイグレーション: storage_domains, network_domains, locations テーブル
- [x] **Task S006-1-2**: locations: parent_id 自己参照、type (site/floor/row/rack/unit)、fault_attributes JSONB

### Story S006-2: Topology Service [x]

- [x] **Task S006-2-1**: `internal/topology/service.go` Service インターフェース
- [x] **Task S006-2-2**: CreateStorageDomain, CreateNetworkDomain
- [x] **Task S006-2-3**: ホスト⇔ストレージドメイン関連付け（host_storage_domains）

### Story S006-3: コンピュートプール導出 [x]

- [x] **Task S006-3-1**: GetComputePool: ストレージドメイン ∩ ネットワークドメインのホスト集合
- [x] **Task S006-3-2**: ListReachableHosts(backendID)
- [x] **Task S006-3-3**: ListReachableBackends(hostID)

### Story S006-4: ロケーション管理 [x]

- [x] **Task S006-4-1**: ロケーションツリー CRUD
- [x] **Task S006-4-2**: WITH RECURSIVE によるパス取得、サブツリー検索
- [x] **Task S006-4-3**: フォルトドメイン導出（指定階層でのグルーピング）

### Story S006-5: API + CLI [x]

- [x] **Task S006-5-1**: POST/GET /api/v1/storage-domains, /network-domains, /locations
- [x] **Task S006-5-2**: GET /api/v1/compute-pools
- [x] **Task S006-5-3**: cirrusctl storage-domain/network-domain/location/compute-pool コマンド

---

## Sprint S007: Network 基盤（OVN） [DONE]

OVN ベースのネットワーク実装。Sprint S010-S012 で全面的に VPC モデルへ置換済み。

### Story S007-1: OVN クライアント + ネットワークモデル [x]

- [x] **Task S007-1-1**: OVNClient インターフェース + OVSDB 実装
- [x] **Task S007-1-2**: マイグレーション: networks, subnets, ports テーブル
- [x] **Task S007-1-3**: IPAM: AllocateIP, ReleaseIP, MAC生成

### Story S007-2: Network Service + Reconciler [x]

- [x] **Task S007-2-1**: Network/Subnet/Port CRUD（DB + OVN）
- [x] **Task S007-2-2**: OVNReconciler 基礎実装（ログのみ）
- [x] **Task S007-2-3**: API + CLI（network/subnet/port コマンド）

---

## Sprint S008: テナント向けリソース抽象化（AZ 導入） [DONE]

Availability Zone を導入し、テナント API からインフラ詳細を隠蔽する。テナントは AZ 名とリソース名だけで操作できる。設計: docs/tenant-model.md 参照。

### Story S008-1: Availability Zone モデル [x]

- [x] **Task S008-1-1**: マイグレーション: availability_zones, az_storage_domains テーブル
- [x] **Task S008-1-2**: `internal/az/models.go, service.go, store.go` CRUD 実装

### Story S008-2: AZ 管理者 API + CLI [x]

- [x] **Task S008-2-1**: POST/GET/PUT/DELETE /api/v1/availability-zones（管理者）
- [x] **Task S008-2-2**: POST/DELETE /api/v1/availability-zones/{id}/storage-domains
- [x] **Task S008-2-3**: `cirrusctl admin az create/list/show/delete`

### Story S008-3: AZ テナント API [x]

- [x] **Task S008-3-1**: GET /api/v1/availability-zones（テナント向け）
- [x] **Task S008-3-2**: RBAC: 全テナントロールで参照可能

### Story S008-4: Network API からインフラ詳細を隠蔽 [x]

- [x] **Task S008-4-1**: POST /api/v1/networks からテナント API の network_domain_id を除去
- [x] **Task S008-4-2**: `make serve` での AZ 自動シード（デフォルト AZ 作成）
- [x] **Task S008-4-3**: テスト: AZ CRUD + network_domain 指定なしのネットワーク作成

---

## Sprint S009: cirrus-sim 統合 + テスト基盤構築 [DONE]

cirrus-sim リポジトリを cirrus に統合し、3レイヤーテスト体制を構築する。Sprint S010 の前提となるテスト基盤。

### Story S009-1: シミュレータコード移行 [x]

- [x] **Task S009-1-1**: storage-sim → `test/sim/storage/`（API 互換維持）
- [x] **Task S009-1-2**: awx-sim → `test/sim/awx/`（API 互換維持）
- [x] **Task S009-1-3**: common（イベントログ、障害注入、データジェネレータ）→ `test/sim/common/`
- [x] **Task S009-1-4**: embedded PostgreSQL → `test/sim/postgres/`
- [x] **Task S009-1-5**: OVN-sim 廃止（OVS は結合テストで実物使用）

### Story S009-2: libvirtd-sim のホスト単位分割 + VM シミュレーション [x]

- [x] **Task S009-2-1**: libvirtd-sim をホスト単位に分割（各 worker コンテナ内で独立インスタンス）
- [x] **Task S009-2-2**: `/sim/` 管理 API 維持（hosts, stats, reset, domains）
- [x] **Task S009-2-3**: DomainCreateWithFlags → network namespace + veth ペア作成 + OVS ポート接続
- [x] **Task S009-2-4**: ドメイン XML から interfaceid / ディスク情報をパースし OVS external_ids に設定
- [x] **Task S009-2-5**: ライブマイグレーションシミュレーション（namespace + veth の移動）

### Story S009-3: シミュレータ集約 API + ダッシュボード [x]

- [x] **Task S009-3-1**: `test/sim/aggregator/` 分散シミュレータ状態集約
- [x] **Task S009-3-2**: GET /sim/overview, /sim/hosts, /sim/events, /sim/faults
- [x] **Task S009-3-3**: ダッシュボード WebUI（全ホスト一覧、イベントログ、3秒自動更新）

### Story S009-4: docker-compose 結合テスト基盤 [x]

- [x] **Task S009-4-1**: `test/integration/Dockerfile.worker` cirrus-sim-worker イメージ（OVS + libvirtd-sim）
- [x] **Task S009-4-2**: `docker-compose.dev.yml` worker×3(privileged) + fabric ネットワーク
- [x] **Task S009-4-3**: worker 間の Geneve トンネル通信確認

### Story S009-5: OVS モッククライアント + Makefile [x]

- [x] **Task S009-5-1**: `test/mock/ovs/` MockOVSClient interface
- [x] **Task S009-5-2**: Makefile: test-unit / test-mock / test-integration ターゲット
- [x] **Task S009-5-3**: `make serve` 更新: sim/controller ホスト + worker コンテナ（portman）

### Story S009-6: 障害注入統合 + 既存テスト確認 [x]

- [x] **Task S009-6-1**: libvirtd-sim / storage-sim / awx-sim に fault.Check() 統合
- [x] **Task S009-6-2**: `cmd/cirrus-sim-ctl/` CLI ツール（status, fault inject/list/clear, snapshot）
- [x] **Task S009-6-3**: Sprint 1-5.5 の既存テストが統合後のシミュレータで動作確認
- [x] **Task S009-6-4**: cirrus-sim リポジトリのアーカイブ

---

## Sprint S010: ネットワーク再設計 — データモデル + Service + API/CLI [DONE]

Network/Group/Policy を API・CLI 経由で CRUD できる状態にする。VPC モデルへの全面移行。

### Story S010-1: ネットワークデータモデル移行 [x]

- [x] **Task S010-1-1**: networks テーブル改修（cidr CIDR追加、vni INTEGER UNIQUE追加）
- [x] **Task S010-1-2**: groups テーブル新設（network_id FK、name）
- [x] **Task S010-1-3**: policies テーブル新設（src_group_id、dst_group_id、protocol、dst_port、priority、action）
- [x] **Task S010-1-4**: ports テーブル改修（group_id FK、host_id、role追加）
- [x] **Task S010-1-5**: egresses, ingresses, gateway_nodes テーブル新設

### Story S010-2: IPAM（/30 ブロック採番） [x]

- [x] **Task S010-2-1**: Network の CIDR から /30 ブロックを順番に払い出し
- [x] **Task S010-2-2**: CIDR プール管理（デフォルト 100.64.0.0/10）
- [x] **Task S010-2-3**: VNI 自動採番、MAC アドレス生成
- [x] **Task S010-2-4**: テスト: /30 採番ロジック、CIDR 枯渇、VNI ユニーク性

### Story S010-3: Network/Group/Policy Service [x]

- [x] **Task S010-3-1**: `internal/network/service.go` Service インターフェース定義
- [x] **Task S010-3-2**: Network CRUD: DB + CIDR/VNI 割当
- [x] **Task S010-3-3**: Group CRUD: DB（フロー変更なし、同期レスポンス）
- [x] **Task S010-3-4**: Policy CRUD: DB + HostNetworkState 再計算

### Story S010-4: API + CLI [x]

- [x] **Task S010-4-1**: POST/GET/DELETE /api/v1/networks（name, cidr 指定）
- [x] **Task S010-4-2**: POST/GET/DELETE /api/v1/networks/{nid}/groups
- [x] **Task S010-4-3**: POST/GET/DELETE /api/v1/networks/{nid}/policies
- [x] **Task S010-4-4**: cirrusctl network/group/policy コマンド追加

---

## Sprint S011: ネットワーク再設計 — HostNetworkState + エージェント [DONE]

VM がネットワーク接続し、OVS フロー・DHCP・DNS・メタデータが動作する。

### Story S011-1: HostNetworkState 計算・配信 [x]

- [x] **Task S011-1-1**: `internal/network/controller.go` HostNetworkState 計算ロジック
- [x] **Task S011-1-2**: `proto/network.proto` NetworkStateService（gRPC server streaming）
- [x] **Task S011-1-3**: WatchHostNetworkState: 初回全状態 + 差分ストリーミング
- [x] **Task S011-1-4**: テスト: 状態計算・差分計算のユニットテスト

### Story S011-2: OVS エージェント [x]

- [x] **Task S011-2-1**: `internal/network/agent/flow.go` HostNetworkState → OpenFlow フロー変換（純粋関数）
- [x] **Task S011-2-2**: OpenFlow パイプライン（Table 0-7: 入力分類→conntrack→GROUP_ID→Policy→Geneve→ローカル出力）
- [x] **Task S011-2-3**: `internal/network/agent/ovsclient.go` OVSClient interface
- [x] **Task S011-2-4**: Port Security（MAC スプーフィング防止）、conntrack ステートフル制御、inline ARP
- [x] **Task S011-2-5**: テスト: レイヤー2（MockOVSClient）でフロー変換検証

### Story S011-3: DHCP / DNS / メタデータサービス [x]

- [x] **Task S011-3-1**: `internal/network/agent/dhcp.go` エージェント内 DHCP サーバ（insomniacslk/dhcp）
- [x] **Task S011-3-2**: `internal/network/agent/dns.go` エージェント内 DNS サーバ（miekg/dns）
  Network 間隔離・外部 DNS フォワード・PTR レコード対応
- [x] **Task S011-3-3**: `internal/network/agent/metadata.go` エージェント内メタデータ HTTP（169.254.169.254）

### Story S011-4: 統合 [x]

- [x] **Task S011-4-1**: `internal/network/agent/agent.go` NetworkAgent（gRPC→StateCache→Pipeline）
- [x] **Task S011-4-2**: controller/worker 両方の起動統合
- [x] **Task S011-4-3**: マイグレーション 000009: ports.vm_name カラム追加

---

## Sprint S012: ネットワーク再設計 — 実 OVS クライアント + Reconciler + 結合テスト [DONE]

ネットワーク状態の整合性チェックと全機能の結合テストが通る。

### Story S012-1: 実 OVS クライアント + ポート作成 [x]

- [x] **Task S012-1-1**: `internal/network/agent/ovs_openflow.go` ExecOVSClient 実装（os/exec CLIラッパー）
- [x] **Task S012-1-2**: ovs-ofctl dump-flows 出力パーサー + ユニットテスト
- [x] **Task S012-1-3**: IsOVSAvailable(): ovs-vsctl で判定、フォールバックで state-only モード
- [x] **Task S012-1-4**: `internal/network/store.go` CreatePort 追加

### Story S012-2: Agent 統合 + Reconciler [x]

- [x] **Task S012-2-1**: agent.go: IsOVSAvailable() → ExecOVSClient 接続
- [x] **Task S012-2-2**: `internal/controller/reconcile/network.go` NetworkReconciler（ログのみ）
- [x] **Task S012-2-3**: cmd/cirrus/main.go に NetworkReconciler errgroup 起動追加
- [x] **Task S012-2-4**: テスト: Reconciler 4ケース（NoWarnings, PortMissingGroup 等）

### Story S012-3: conntrack フロー修正 + fabric_ip [x]

- [x] **Task S012-3-1**: conntrack フローに ip protocol match 追加（OFPBAC_MATCH_INCONSISTENT 修正）
- [x] **Task S012-3-2**: マイグレーション 000010: hosts.fabric_ip カラム追加
- [x] **Task S012-3-3**: RegisterHost に fabric_ip 追加（全クエリ・gRPC 含む）
- [x] **Task S012-3-4**: entrypoint.sh: コンテナ IP 自動検出 → --fabric-ip フラグ

### Story S012-4: 結合テスト基盤 [x]

- [x] **Task S012-4-1**: `test/integration/testutil.go` TestEnv（ネットワーク/グループ/ポリシー/ポート作成ヘルパー）
- [x] **Task S012-4-2**: `test/integration/network_test.go` テストケース5件（OVSフロー適用、クロスホストトンネル、差分フロー、Network隔離、Reconciler整合性）

---

## Sprint S013: Storage 基盤 [DONE]

ストレージバックエンドを登録し、ボリュームを API で作成でき、cirrus-sim storage-sim に反映される。

### Story S013-1: バックエンドドライバ [x]

- [x] **Task S013-1-1**: `internal/storage/driver.go` BackendDriver インターフェース（CreateVolume, DeleteVolume, ExportVolume, UnexportVolume）
- [x] **Task S013-1-2**: `internal/storage/driver/sim/sim.go` storage-sim 用ドライバ（REST API 呼び出し）
- [x] **Task S013-1-3**: Capabilities() 返却

### Story S013-2: ストレージモデルの DB [x]

- [x] **Task S013-2-1**: マイグレーション 000011: storage_backends, volume_types, volumes テーブル
- [x] **Task S013-2-2**: volume_types: required_capabilities JSONB、qos_policy JSONB
- [x] **Task S013-2-3**: volumes.exported_host_id + export_info JSONB（アタッチ状態管理）

### Story S013-3: Storage Service [x]

- [x] **Task S013-3-1**: `internal/storage/service.go` Service インターフェース定義
- [x] **Task S013-3-2**: RegisterBackend, CreateVolume, DeleteVolume, ResizeVolume 実装
- [x] **Task S013-3-3**: ボリューム作成時: capability 要件でバックエンド選定 + AZ フィルタ
- [x] **Task S013-3-4**: ExportVolume/UnexportVolume（AttachVolume の設計変更版: Storage 側 + BlockDev.Attach に分離）

### Story S013-4: API エンドポイント [x]

- [x] **Task S013-4-1**: POST/GET /api/v1/admin/storage-backends, POST /drain
- [x] **Task S013-4-2**: POST /api/v1/admin/volume-types
- [x] **Task S013-4-3**: GET /api/v1/volume-types, GET /api/v1/volume-types/{id}
- [x] **Task S013-4-4**: POST/GET/DELETE /api/v1/volumes（volume_type_id + az 指定）
  注: POST /api/v1/volumes/{id}/attach, /detach は S015 (Compute Orchestrator) で実装

### Story S013-5: Storage Reconciler [x]

- [x] **Task S013-5-1**: `internal/controller/reconcile/storage.go` StorageReconciler（5分間隔）
- [x] **Task S013-5-2**: 各バックエンドに ListVolumes 問い合わせ → DB と照合（ログのみ）
- [x] **Task S013-5-3**: 遷移中（creating, deleting）ボリュームは除外

### Story S013-6: テスト [x]

- [x] **Task S013-6-1**: `internal/storage/service_impl_test.go` capability マッチング、バックエンド選択、AZ フィルタ
- [x] **Task S013-6-2**: ResizeVolume: サイズ縮小・InUse 拒否、テナント分離
- [x] **Task S013-6-3**: 結合テスト: バックエンド登録 → ボリューム作成 → storage-sim 確認

### Story S013-7: CLI クライアント [x]

- [x] **Task S013-7-1**: `cirrusctl admin storage-backend create/list/show/drain`
- [x] **Task S013-7-2**: `cirrusctl admin volume-type create`
- [x] **Task S013-7-3**: `cirrusctl volume-type list/show`
- [x] **Task S013-7-4**: `cirrusctl volume create/list/show/delete/resize`

---

## Sprint S014: Storage プロトコル Layer 3 テスト（iSCSI / RBD） [DONE]

実 iSCSI target・実 Ceph（RBD）を docker-compose に追加し、Driver の ExportVolume → Worker の BlockDev アタッチまでのフルスタックを通す。S013 の sim ベーステストでは検証できないプロトコルレベルの接続性を担保する。

### Story S014-1: iSCSI Layer 3 テスト [x]

- [x] **Task S014-1-1**: docker-compose.storage.yml に iSCSI target コンテナ追加（tgt ベース、ローカル開発専用）
- [x] **Task S014-1-2**: iSCSI 用 BackendDriver 実装（cirrus-iscsi-server HTTP wrapper 経由、CLI 使用）
- [x] **Task S014-1-3**: Worker コンテナに Open-iSCSI イニシエータ（iscsiadm）インストール
- [x] **Task S014-1-4**: 結合テスト: ExportVolume → iscsiadm discovery → UnexportVolume フロー

### Story S014-2: RBD（Ceph）Layer 3 テスト [x]

- [x] **Task S014-2-1**: docker-compose.storage.yml に Ceph シングルノードコンテナ追加（quay.io/ceph/demo、ローカル開発専用）
- [x] **Task S014-2-2**: RBD 用 BackendDriver 実装（cirrus-rbd-server HTTP wrapper 経由、CLI 使用）
- [x] **Task S014-2-3**: Worker コンテナに Ceph クライアント（rbd コマンド）インストール
- [x] **Task S014-2-4**: 結合テスト: ExportVolume → rbd info → UnexportVolume フロー

---

## Sprint S015: Scheduler + VM 作成 [DONE]

VM 作成 API が動作し、スケジューラがホストとバックエンドを選定し、cirrus-sim 上で VM が起動する。

### Story S015-0: S014 動作確認（make serve + make serve-storage） [x]

S014 で実装した iSCSI / RBD ドライバーを実際のコンテナ環境で動作確認する。ユニットテスト・ビルドは済んでいるが、実行時の結合テストは未実施。

- [x] **Task S015-0-1**: `make serve` でコントローラー + ワーカーが正常起動すること（ログ確認・`/healthz` 疎通確認）
- [x] **Task S015-0-2**: `make serve-storage` で iSCSI target コンテナ（10.100.0.100:8080）が起動し `/healthz` が 200 を返すこと
- [x] **Task S015-0-3**: `make serve-storage` で Ceph コンテナ（10.100.0.101:8090）が起動し `/healthz` が 200 を返すこと
- [x] **Task S015-0-4**: `go test -tags integration -run TestISCSIDriver ./test/integration/...` が PASS すること
- [x] **Task S015-0-5**: `go test -tags integration -run TestRBDDriver ./test/integration/...` が PASS すること

### Story S015-1: Flavor エンティティ [x]

- [x] **Task S015-1-1**: マイグレーション: flavors テーブル
- [x] **Task S015-1-2**: `internal/flavor/models.go, service.go, store.go` Flavor CRUD
- [x] **Task S015-1-3**: 管理者 API: POST/DELETE /api/v1/admin/flavors
- [x] **Task S015-1-4**: テナント API: GET /api/v1/flavors（利用可能な Flavor 一覧）
- [x] **Task S015-1-5**: CLI: `cirrusctl admin flavor create/delete` + `cirrusctl flavor list/show`
- [x] **Task S015-1-6**: `make serve` でデフォルト Flavor シード（m1.small/medium/large）

### Story S015-2: Scheduler [x]

- [x] **Task S015-2-1**: `internal/scheduler/scheduler.go` Scheduler インターフェース
- [x] **Task S015-2-2**: フィルタリング: AZ フィルタ、Flavor→Capability マッチング、稼働状態フィルタ
- [x] **Task S015-2-3**: スコアリング: ホストのリソース空き率、バックエンドの容量空き率
- [x] **Task S015-2-4**: Schedule() → (host_id, backend_id) ペア返却

### Story S015-3: BlockDev（Worker 側） [x]

- [x] **Task S015-3-1**: `internal/blockdev/manager.go` Manager インターフェース
- [x] **Task S015-3-2**: Attach/Detach 実装（ExportInfo の protocol に応じた処理）
- [x] **Task S015-3-3**: cirrus-sim では protocol="sim" のスタブ接続（no-op）

### Story S015-4: Hypervisor VM 操作 [x]

- [x] **Task S015-4-1**: `internal/hypervisor/libvirt/libvirt.go` DefineVM, StartVM, StopVM, DestroyVM, UndefineVM
- [x] **Task S015-4-2**: domain XML 生成（テンプレートベース）: ディスク、ポート（interfaceid）、cloud-init
- [x] **Task S015-4-3**: cloud-init ISO 生成（network-config, meta-data, user-data）

### Story S015-5: gRPC CreateVM [x]

- [x] **Task S015-5-1**: proto/agent.proto WorkerService + CreateVM/DeleteVM RPC 追加（DiskSpec、PortSpec）
- [x] **Task S015-5-2**: worker 側: CreateVM → BlockDev.Attach → Hypervisor.DefineVM → StartVM; controller 側 WorkerClientPool

### Story S015-6: Compute Orchestrator [x]

- [x] **Task S015-6-1**: `internal/compute/service.go` Service インターフェース
- [x] **Task S015-6-2**: `internal/compute/orchestrator.go` CreateVM（Network.CreatePort → Storage.CreateVolume → Scheduler.Schedule → Storage.ExportVolume → Agent.CreateVM）
- [x] **Task S015-6-3**: 非同期ジョブ実行（goroutine + detached context）
- [x] **Task S015-6-4**: マイグレーション: vms, vm_volumes テーブル（migration 000014）; hosts.worker_grpc_addr 追加（migration 000013）

### Story S015-7: API + テスト + CLI [x]

- [x] **Task S015-7-1**: POST /api/v1/vms（202 Accepted）: flavor_id, az, network_id, volume_type_id 指定
- [x] **Task S015-7-2**: GET /api/v1/vms, GET /api/v1/vms/{id}, DELETE /api/v1/vms/{id}
- [ ] **Task S015-7-3**: POST /api/v1/volumes/{id}/attach, /detach（S013-4-4 からの移行） ← S016 へ持越し
- [x] **Task S015-7-4**: ユニットテスト全パス（go test ./...）
- [x] **Task S015-7-5**: `cirrusctl vm create/list/show/delete`

---

## Sprint S016: VM ライフサイクル [DONE]

VM の起動・停止・再起動・削除が動作し、ステータス遷移が正しく管理される。

### Story S016-1: VM 操作 gRPC [x]

- [x] **Task S016-1-1**: proto/agent.proto DeleteVM, StartVM, StopVM, ForceStopVM, RebootVM, GetVMState RPC 追加
- [x] **Task S016-1-2**: worker 側: 各操作の Hypervisor 委譲実装（StopVM=ACPI shutdown, ForceStopVM=destroy）

### Story S016-2: Compute Orchestrator 操作 [x]

- [x] **Task S016-2-1**: DeleteVM: DestroyVM → BlockDev.Detach → Network.DeletePort → UndefineVM → Storage.UnexportVolume → DeleteVolume → DB 更新
- [x] **Task S016-2-2**: StartVM / StopVM / ForceStopVM / RebootVM: gRPC 経由で worker に指示
- [x] **Task S016-2-3**: 削除時のリソース解放順序の保証（UndefineVM 前にアタッチメントを全解除）

### Story S016-3: ステータス管理 [x]

- [x] **Task S016-3-1**: VM ステータス遷移の状態機械実装（許可遷移: stopped→start→running, running→stop/force-stop→stopped, running→reboot→running, stopped/error→delete）
- [x] **Task S016-3-2**: 操作ガード: running 中の delete は 409, building/deleting 中の全操作は 409
- [x] **Task S016-3-3**: エラー時のステータス遷移（building→error 等）
- [x] **Task S016-3-4**: 非同期ジョブ失敗時のクリーンアップ（作成途中リソース削除）

### Story S016-4: 管理者 VM 修復 API [x]

- [x] **Task S016-4-1**: POST /api/v1/admin/vms/{id}/repair — error → stopped に強制遷移（管理者専用）
- [x] **Task S016-4-2**: `cirrusctl admin vm repair` コマンド

### Story S016-5: make serve シード拡充 [x]

`make serve` 後に追加手作業なしで VM 作成が通るよう、`_seed-topology` に以下を追加する。

- [x] **Task S016-5-1**: `_seed-topology` に sim StorageBackend 作成を追加（driver=sim、storage-domain=default-sd）
- [x] **Task S016-5-2**: `_seed-topology` に default VolumeType 作成を追加
- [x] **Task S016-5-3**: `_activate-hosts` に ホスト→default-sd 関連付けを追加（登録済み全ホスト対象）

### Story S016-6: テスト + CLI [x]

- [x] **Task S016-6-1**: 結合テスト: 作成→停止→起動→再起動→削除の全ライフサイクル
- [x] **Task S016-6-2**: 結合テスト: force-stop からの削除フロー
- [x] **Task S016-6-3**: 異常系: 作成途中の worker 障害で error ステータス遷移
- [x] **Task S016-6-4**: cirrus-sim 障害注入: libvirt-sim の DomainCreate 失敗
- [x] **Task S016-6-5**: `cirrusctl vm start/stop/force-stop/reboot/delete`

---

## Sprint S017: ホスト状態遷移制約 + Heartbeat 監視 [DONE]

ホストの operational_state 遷移に制約を適用し、heartbeat 途絶で faulty 自動遷移、draining 完了で maintenance 自動遷移が動作する。設計: docs/host.md 参照。

### Story S017-1: 状態遷移制約 [x]

- [x] **Task S017-1-1**: SetOperationalState に遷移ルール適用（host.md の遷移表に準拠）
- [x] **Task S017-1-2**: retiring は終端状態（activate 不可）
- [x] **Task S017-1-3**: active→maintenance は稼働 VM 数=0 の場合のみ許可
- [x] **Task S017-1-4**: 不正な遷移は 409 Conflict で拒否

### Story S017-2: Heartbeat 監視 + faulty 自動遷移 [x]

- [x] **Task S017-2-0**: マイグレーション: hosts テーブルに `missed_heartbeat_count` カラム追加（DB 永続カウンタ）
- [x] **Task S017-2-1**: `internal/controller/heartbeat_monitor.go` 定期的に last_heartbeat を監視
- [x] **Task S017-2-2**: 30 秒ごとにチェック、応答なしホストのみ missed_heartbeat_count +1。count >= 3 で active/draining → faulty、遷移時カウンタリセット

### Story S017-3: HostFaultyHandler（カスケード障害処理） [x]

- [x] **Task S017-3-1**: `internal/controller/host_faulty_handler.go` faulty 遷移時のカスケード更新
- [x] **Task S017-3-2**: faulty 遷移直後: ホスト上の全 VM を error に、関連ポートを down に更新

### Story S017-4: Draining 完了の自動遷移 + テスト [x]

- [x] **Task S017-4-1**: draining 状態のホストで稼働 VM 数が 0 になったら maintenance に自動遷移
- [x] **Task S017-4-2**: テスト: 不正遷移拒否、heartbeat 途絶→faulty 自動遷移
- [x] **Task S017-4-3**: テスト: faulty→HostFaultyHandler→VM/ポートカスケード更新
- [x] **Task S017-4-4**: テスト: draining→VM 退避完了→maintenance 自動遷移

---

## Sprint S018: DriftEvent 基盤 + Heartbeat Reconciler [DONE]

統一的な DriftEvent 基盤を構築し、heartbeat 内の VM 情報で Compute 層のドリフトをパッシブ検出する。S012/S013 の Network/Storage Reconciler を DriftEvent 発火に移行する。設計: docs/reconciliation.md 参照。

### Story S018-1: DriftEvent 基盤 [x]

- [x] **Task S018-1-1**: `internal/controller/reconcile/drift.go` DriftEvent 型定義
- [x] **Task S018-1-2**: DriftHandler: Deduplicator（インメモリ TTL キャッシュ）+ Logger/AlertSink + AutoHealer
- [x] **Task S018-1-3**: マイグレーション: drift_events テーブル
- [x] **Task S018-1-4**: reconcile 設定パラメータ（--reconcile-interval, --auto-heal-enabled 等）

### Story S018-2: Heartbeat Reconciler [x]

- [x] **Task S018-2-1**: ResourceReport に RunningVMs フィールド追加（proto 実装済み: VMInfo{vm_id, status, vcpus, ram_mb}）
- [x] **Task S018-2-2**: `internal/agent/agent.go` collectResources を running + shutoff + crashed に拡張（paused/shutdown は除外）
- [x] **Task S018-2-3**: `internal/controller/reconcile/compute.go` HeartbeatReconciler
- [x] **Task S018-2-4**: DB有・heartbeat無 → Auto-heal: DB→error（楽観的ロック）
- [x] **Task S018-2-5**: DB無・heartbeat有 → Alert
- [x] **Task S018-2-6**: VM ステータス不一致: ユーザ操作記録の有無で分岐

### Story S018-3: 既存 Reconciler 移行 + テスト [x]

- [x] **Task S018-3-1**: Network Reconciler を DriftEvent 発火に移行 + AutoHealer: state_mismatch → HostNetworkState 再配信
- [x] **Task S018-3-2**: Storage Reconciler を DriftEvent 発火に移行（Alert のみ、修復ロジックなし）
- [x] **Task S018-3-3**: テスト: DriftEvent 永続化、HeartbeatReconciler 各ケース、重複抑制

---

## Sprint S019: Quota [DONE]

階層化クォータ（組織→テナント）が機能し、超過時にリソース作成が拒否される。

### Story S019-1: Quota Service [x]

- [x] **Task S019-1-1**: `internal/quota/service.go` Service インターフェース
- [x] **Task S019-1-2**: Check, Reserve, Commit, Release, Decommit 実装（予約パターン）
- [x] **Task S019-1-3**: 組織クォータとテナントクォータの両方を検査

### Story S019-2: クォータ対象リソース + 統合 [x]

- [x] **Task S019-2-1**: vCPU, メモリ, VM 数, ボリューム数/容量, スナップショット数、ネットワーク数（egress/ingress は S020-4-6 で追加）
- [x] **Task S019-2-2**: Compute.CreateVM に Quota.Reserve/Commit/Release 組み込み + DeleteVM に Decommit 組み込み
- [x] **Task S019-2-3**: Storage.CreateVolume / Network.CreateNetwork にクォータチェック組み込み + 削除時 Decommit
- [x] **Task S019-2-4**: PUT/GET /api/v1/tenants/{id}/quota, PUT/GET /api/v1/organizations/{id}/quota
- [x] **Task S019-2-5**: 結合テスト: クォータ上限設定→超過で 403 拒否（test/integration/quota_test.go）
- [x] **Task S019-2-6**: `cirrusctl quota show/set`

---

## Sprint S020: NAT Egress + Direct IP Ingress + ゲートウェイ [DONE]

テナント Network からの外部接続（NAT Gateway Egress）と外部からの着信（Direct IP Ingress）がゲートウェイノード経由で動作する。VPN/Direct Connect Egress は S042、L4 LB Ingress は S043、内部 LB は S044 で対応。Active-Standby HA は S033 で対応。設計: docs/network.md 参照。

### Story S020-1: ゲートウェイノード管理 [x]

- [x] **Task S020-1-1**: マイグレーション: `hosts.node_roles TEXT[] NOT NULL DEFAULT '{vm}'` カラム追加（`vm` / `gateway` / `controller` のケーパビリティを表現、co-location 可能）
- [x] **Task S020-1-2**: 管理者 API: POST/GET/DELETE /api/v1/admin/gateway-nodes（`node_roles` に `gateway` を含むホストの登録・一覧・削除）
- [x] **Task S020-1-3**: Network 単位で GW ノードを1台割り当て（Active-Standby HA は S033 で対応）

### Story S020-2: NAT Gateway Egress [x]

- [x] **Task S020-2-1**: テナント API: POST/GET/DELETE /tenants/{tid}/networks/{nid}/egresses（type=nat_gateway のみ。VPN/Direct Connect は S042 で対応）
- [x] **Task S020-2-2**: HostNetworkState に Egress ルールを含めて配信
- [x] **Task S020-2-3**: エージェント側: Egress 宛フローを GW ノードへ転送（SNAT ルール適用）

### Story S020-3: Direct IP Ingress + IP プール [x]

- [x] **Task S020-3-1**: 管理者 API: POST/GET/DELETE /api/v1/admin/ip-pools（パブリック IP プール管理）
- [x] **Task S020-3-2**: テナント API: POST/GET/DELETE /tenants/{tid}/networks/{nid}/ingresses（type=direct_ip のみ。L4 LB は S043 で対応）
- [x] **Task S020-3-3**: GW ノードで DNAT ルール適用 + HostNetworkState に Ingress ルールを含めて配信

### Story S020-4: Reconciler + テスト + CLI + Quota [x]

- [x] **Task S020-4-1**: Reconciler 拡張: Egress/Ingress のデータプレーン照合 → DriftEvent 発火
- [x] **Task S020-4-2**: テスト: NAT GW Egress フロー、Direct IP Ingress フロー
- [x] **Task S020-4-3**: `cirrusctl egress/ingress/admin gateway-node/admin ip-pool`
- [x] **Task S020-4-4**: Egress/Ingress 数クォータチェックを Quota.Check/Reserve/Commit/Release に組み込み（S019 で対象外とした分）

---

## Sprint S021: Phase 1 安定化 [DONE]

Phase 1 全機能の結合テストが通り、安定してデプロイできる状態。

### Story S021-1: E2E テストスイート [x]

- [x] **Task S021-1-1**: テナント作成→VM 削除のフルフロー E2E テスト
- [x] **Task S021-1-2**: マルチテナントシナリオ（テナント A/B の隔離確認）
- [x] **Task S021-1-3**: Policy/Group によるアクセス制御確認
- [x] **Task S021-1-4**: cirrus-sim medium 環境でのテスト

### Story S021-2: エラーハンドリング + API 仕上げ [x]

- [x] **Task S021-2-1**: 全 API エンドポイントのバリデーション強化
- [x] **Task S021-2-2**: 非同期ジョブの失敗時クリーンアップ（defer cleanup パターンによる P0/P1 宙吊りリソース解消: VM作成パイプライン途中失敗時のポート/ボリューム自動削除、Quota Commit 失敗時の補正、teardownVM 段階的失敗の安全な継続処理）。ジョブキュー基盤は S045 で対応
- [x] **Task S021-2-3**: ページネーション・フィルタリング・ソート（全リスト系 API）。ページネーションはカーソルベース（`?after=<cursor>`）

### Story S021-3: Reconcile 結合テスト [x]

- [x] **Task S021-3-1**: cirrus-sim で VM 状態不整合を注入 → HeartbeatReconciler が DriftEvent 発火 → Auto-heal 動作確認
- [x] **Task S021-3-2**: OVS フロー手動削除 → Network Reconciler 検出 → Alert 確認（Network Reconciler に OVS フローレベル検証を追加実装する）
- [x] **Task S021-3-3**: heartbeat 停止 → faulty → HostFaultyHandler → VM/ポートカスケード更新

---

## Sprint S022: WebUI 基盤 + Phase 1 管理画面 [DONE]

Phase 1 の全機能（Identity・Host・Network・Storage・Compute・Quota・Egress/Ingress）を Web ブラウザで操作できる。**WebUI でできることはすべて REST API でも実行可能**（API ファースト原則）。デザインシステムに準拠した UI を controller が静的ファイルとして配信する。

### Story S022-1: フロントエンド基盤 [x]

- [x] **Task S022-1-1**: `web/` ディレクトリにフロントエンドプロジェクト初期化（Vite + React + Tailwind CSS + shadcn/ui、design-system のデザイントークンを Tailwind theme に適用）
- [x] **Task S022-1-2**: 開発時は Vite dev server (:5173) が `/api/*` を Go controller にプロキシ、本番は `web/dist/` を chi FileServer で配信
- [x] **Task S022-1-3**: 認証フロー: トークン入力 → localStorage 保存 → 全 API リクエストに付与
- [x] **Task S022-1-4**: テナントコンテキスト切り替え（X-Tenant-ID ヘッダ管理）
- [x] **Task S022-1-5**: `make build` に web ビルドを統合

### Story S022-2: 管理者 UI [x]

- [x] **Task S022-2-1**: 組織・テナント管理画面（CRUD + ロール割り当て）
- [x] **Task S022-2-2**: ホスト管理画面（一覧・状態遷移ボタン: activate/drain/maintenance/retire）
- [x] **Task S022-2-3**: Storage Backend・Volume Type・Flavor 管理画面
- [x] **Task S022-2-4**: Quota 設定画面（テナント別 vCPU/メモリ/VM 数/ボリューム容量）
- [x] **Task S022-2-5**: Drift Event ビューア（一覧・フィルタ・ステータス確認）

### Story S022-3: テナント UI [x]

- [x] **Task S022-3-1**: ダッシュボード（リソース使用量サマリ・クォータ残量）
- [x] **Task S022-3-2**: VM 管理画面（作成フォーム・一覧・詳細・start/stop/reboot/delete）
- [x] **Task S022-3-3**: ネットワーク管理画面（Network/Group/Policy CRUD）
- [x] **Task S022-3-4**: ボリューム管理画面（作成・一覧・削除・リサイズ）
- [x] **Task S022-3-5**: Egress / Ingress 管理画面

### Story S022-4: テスト [x]

- [x] **Task S022-4-1**: 主要フローの E2E テスト（Playwright 等）: ログイン→VM 作成→削除
- [x] **Task S022-4-2**: `make serve` で WebUI が http://localhost:{port} で起動すること確認

---

## Sprint S046: 管理者 UI [DONE]

インフラ管理者・組織管理者が Phase 1 の全管理リソース（組織・テナント・ホスト・ストレージ・Quota・Drift Event・GW ノード・IP プール）を WebUI から操作できる。

### Story S046-1: 管理者として、組織・テナント・ロールを WebUI から管理したい。なぜなら、CLI を使わずにテナント払い出しができるようにしたいから。 [x]

- [x] **Task S046-1-1**: 組織一覧・作成・削除画面（POST/GET/DELETE /api/v1/organizations）
- [x] **Task S046-1-2**: テナント一覧・作成・削除画面（POST/GET/DELETE /api/v1/organizations/{id}/tenants）
- [x] **Task S046-1-3**: ロール割り当て画面（POST/GET/DELETE /api/v1/tenants/{id}/role-assignments）
- [x] **Task S046-1-4**: Playwright テスト: 組織作成 → テナント作成 → ロール割り当てフロー

**Acceptance Criteria (GUI):**
- [x] State diagram confirmed with user (see sprint-logs/S046/gui-spec-S046-1.md)
- [x] Playwright tests pass: `npx playwright test admin-s046-1`
- [x] All interactive elements have `data-testid` attributes
- [x] API calls are mocked in tests (no real backend dependency)

### Story S046-2: 管理者として、ホスト・ストレージ・Flavor を WebUI から管理したい。なぜなら、コンピュートリソースの状態を一覧で把握・操作したいから。 [x]

- [x] **Task S046-2-1**: ホスト一覧・状態遷移ボタン（activate / drain / maintenance / retire）
- [x] **Task S046-2-2**: Storage Backend・Volume Type・Flavor 一覧・作成・削除画面
- [x] **Task S046-2-3**: ゲートウェイノード登録・一覧・削除画面（/api/v1/admin/gateway-nodes）
- [x] **Task S046-2-4**: IP プール管理画面（/api/v1/admin/ip-pools）
- [x] **Task S046-2-5**: Playwright テスト: ホスト状態遷移・Flavor 作成フロー

**Acceptance Criteria (GUI):**
- [x] State diagram confirmed with user (see sprint-logs/S046/gui-spec-S046-2.md)
- [x] Playwright tests pass: `npx playwright test admin-s046-2`
- [x] All interactive elements have `data-testid` attributes
- [x] API calls are mocked in tests (no real backend dependency)

### Story S046-3: 管理者として、Quota 設定と Drift Event を WebUI から確認したい。なぜなら、テナント別のリソース上限管理と異常検知を一箇所で行いたいから。 [x]

- [x] **Task S046-3-1**: テナント別 Quota 設定画面（vCPU / メモリ / VM 数 / ボリューム容量 / ネットワーク数 / Egress・Ingress 数）
- [x] **Task S046-3-2**: Drift Event ビューア（一覧・リソース種別フィルタ・ステータス確認）+ バックエンド API 実装（GET/PATCH /admin/drift-events）
- [x] **Task S046-3-3**: Playwright テスト: Quota 設定 → 超過エラー確認フロー

**Acceptance Criteria (GUI):**
- [x] State diagram confirmed with user (see sprint-logs/S046/gui-spec-S046-3.md)
- [x] Playwright tests pass: `npx playwright test admin-s046-3`
- [x] All interactive elements have `data-testid` attributes
- [x] API calls are mocked in tests (no real backend dependency)

---

## Sprint S047: テナント UI — VM 管理 [DONE]

テナントメンバーが WebUI から VM のライフサイクル全体（作成・一覧・詳細・電源操作・削除）を操作できる。

### Story S047-1: テナントメンバーとして、VM を WebUI から作成・管理したい。なぜなら、GUI で直感的に仮想マシンを払い出したいから。 [x]

- [x] **Task S047-1-1**: VM 一覧画面（ステータスバッジ・Flavor・AZ 表示）
- [x] **Task S047-1-2**: VM 作成フォーム（名前・Flavor・AZ・ブートボリュームサイズ・ネットワーク選択）
- [x] **Task S047-1-3**: VM 詳細画面（スペック・状態・接続ポート・ボリューム一覧）
- [x] **Task S047-1-4**: VM 電源操作ボタン（start / stop / reboot）と削除
- [x] **Task S047-1-5**: Playwright テスト: VM 作成 → 起動 → 停止 → 削除フロー

---

## Sprint S048: テナント UI — ネットワーク・ボリューム [DONE]

テナントメンバーが WebUI からネットワーク・セキュリティ設定・ボリュームを管理できる。

### Story S048-1: テナントメンバーとして、ネットワークとセキュリティポリシーを WebUI から管理したい。なぜなら、VM のネットワーク構成を GUI で完結させたいから。 [x]

- [x] **Task S048-1-1**: Network 一覧・作成（CIDR 省略時は自動割り当て）・削除画面（CIDR・status 表示）
- [x] **Task S048-1-2**: グループ（Group）一覧・作成・削除画面（ネットワーク展開パネル内）
- [x] **Task S048-1-3**: ポリシー（Policy）一覧・作成（src_group/dst_group 選択、protocol/dst_port/priority/action）・削除画面
- [x] **Task S048-1-4**: Playwright テスト: `web/e2e/s048-network.spec.ts`

**Acceptance Criteria (GUI):**
- [x] 状態遷移図確認済み（docs/sprint-logs/S048/gui-spec-S048-1.md 参照）
- [x] Playwright テスト通過: `npx playwright test s048-network` (7/7 passed)
- [x] すべてのインタラクティブ要素に `data-testid` 属性あり
- [x] API コールはテスト内でモック（実バックエンド不要）

### Story S048-2: テナントメンバーとして、ボリュームを WebUI から管理したい。なぜなら、ストレージのプロビジョニングを GUI で行いたいから。 [x]

- [x] **Task S048-2-1**: Volume 一覧・作成・削除画面（サイズ・Volume Type・state バッジ表示）
- [x] **Task S048-2-2**: Volume リサイズ操作（new_size_gb、現在値より大きい値のみ）
- [x] **Task S048-2-3**: Playwright テスト: `web/e2e/s048-volume.spec.ts`

**Acceptance Criteria (GUI):**
- [x] 状態遷移図確認済み（docs/sprint-logs/S048/gui-spec-S048-2.md 参照）
- [x] Playwright テスト通過: `npx playwright test s048-volume` (9/9 passed)
- [x] すべてのインタラクティブ要素に `data-testid` 属性あり
- [x] API コールはテスト内でモック（実バックエンド不要）

---

## Sprint S049: テナント UI — Egress/Ingress + ダッシュボード [DONE]

テナントメンバーが WebUI から外部接続（Egress/Ingress）を管理でき、リソース使用状況をダッシュボードで把握できる。

### Story S049-1: テナントメンバーとして、Egress と Ingress を WebUI から管理したい。なぜなら、NAT ゲートウェイや外部 IP の設定を GUI で完結させたいから。 [x]

- [x] **Task S049-1-0**: `web/src/api/egress.ts` / `ingress.ts` の型をバックエンド実装に合わせて修正（`name`/`status` 削除、`config` 構造体対応、作成リクエスト形式修正）
- [x] **Task S049-1-1**: Egress 一覧・作成（type=nat_gateway）・削除画面を正しい API 型で実装。全インタラクティブ要素に `data-testid` 付与
- [x] **Task S049-1-2**: Ingress 一覧・作成（type=direct_ip、IP プール選択・ターゲット VM 指定）・削除画面を正しい API 型で実装。全インタラクティブ要素に `data-testid` 付与
- [x] **Task S049-1-3**: Playwright テスト pass: `npx playwright test web/e2e/s049-egress-ingress.spec.ts`

**Acceptance Criteria (GUI):**
- [x] 状態遷移図確認済み（docs/sprint-logs/S049/gui-spec-S049-1.md）
- [x] Playwright テスト pass: `npx playwright test web/e2e/s049-egress-ingress.spec.ts` (15/15)
- [x] 全インタラクティブ要素に `data-testid` 属性あり
- [x] API モック使用（実バックエンド不要）

### Story S049-2: テナントメンバーとして、リソース使用状況をダッシュボードで把握したい。なぜなら、クォータ残量と全体状況を一目で確認したいから。 [x]

- [x] **Task S049-2-1**: ダッシュボード画面（VM 数・ネットワーク数・ボリューム容量・vCPU/メモリ使用量のサマリカード）。データソースは `GET /api/v1/tenants/{id}/quota` の `usage` フィールドのみ
- [x] **Task S049-2-2**: Quota 残量バー（vCPU / メモリ / VM 数 / ボリューム容量 / ネットワーク数 / ボリューム数 / Egress 数 / Ingress 数）。上限到達時に `data-full="true"` を付与
- [x] **Task S049-2-3**: Playwright テスト pass: `npx playwright test web/e2e/s049-dashboard.spec.ts`

**Acceptance Criteria (GUI):**
- [x] 状態遷移図確認済み（docs/sprint-logs/S049/gui-spec-S049-2.md）
- [x] Playwright テスト pass: `npx playwright test web/e2e/s049-dashboard.spec.ts` (5/5)
- [x] 全インタラクティブ要素に `data-testid` 属性あり
- [x] API モック使用（実バックエンド不要）

---

## Sprint S050: WebUI E2E テスト拡充 [DONE]

Phase 1 WebUI 全体の結合 E2E テストが通り、`make serve` 環境で安定してデモできる状態にする。

### Story S050-1: QA エンジニアとして、Phase 1 の全テナントワークフローを E2E テストで自動検証したい。なぜなら、デグレを CI で検出できるようにしたいから。 [x]

- [x] **Task S050-1-1**: globalSetup / globalTeardown: テスト用組織・テナント・Quota・Flavor・AZ・IP プール・GW ノードのシード
- [x] **Task S050-1-2**: ライフサイクル spec: 組織/テナント/ロール → ネットワーク/ボリューム/VM → start/stop/reboot → egress/ingress → 全リソース削除
- [x] **Task S050-1-3**: 既存 spec（login / tenant-switch / admin / vms）の通過確認・修正

### Story S050-2: 開発者として、`make serve` 直後にデフォルトテナントが利用可能な状態にしたい。なぜなら、開発・デモ環境を素早く立ち上げられるようにしたいから。 [x]

- [x] **Task S050-2-1**: `make serve` の `_seed-tenant` ステップ: default-org / default-tenant / dev-admin ロール / Quota / IP プール / GW ノード（冪等）
- [x] **Task S050-2-2**: `make serve` 後に Playwright 全 spec が通ることを確認

---

## Sprint S051: エラー UX 改善 — GUI・CLI 全体 [DONE]

すべての GUI 操作および CLI コマンドでエラーが発生した際に、原因と対処方法がユーザーに伝わるメッセージが表示される。

### Story S051-1: 開発者として、API エラーレスポンスに機械可読なエラーコードと詳細情報を含めたい。なぜなら、GUI・CLI がエラー種別を判別してユーザーフレンドリーなメッセージを組み立てられるようにしたいから。 [x]

- [x] **Task S051-1-1**: エラーレスポンス構造体を定義（`{"code": "ERR_NO_HOST", "message": "...", "detail": {...}}`）
- [x] **Task S051-1-2**: エラーコード一覧を定義（`internal/apierror/codes.go`）: スケジューラ系（`ERR_NO_HOST`, `ERR_INSUFFICIENT_RESOURCES`）、クォータ系（`ERR_QUOTA_VCPU`, `ERR_QUOTA_MEMORY`, `ERR_QUOTA_VM_COUNT`, etc.）、認証認可系（`ERR_UNAUTHORIZED`, `ERR_FORBIDDEN`）、リソース系（`ERR_NOT_FOUND`, `ERR_CONFLICT`）
- [x] **Task S051-1-3**: 全 API ハンドラのエラー返却をエラーコード付き構造体に移行
- [x] **Task S051-1-4**: `detail` フィールドにコンテキスト情報を付与（例: クォータ超過時は `{"resource": "vcpu", "limit": 8, "requested": 2, "current": 7}`）

### Story S051-2: テナントメンバー・管理者として、GUI 操作でエラーが発生した際に原因と対処方法がわかるメッセージを見たい。なぜなら、技術的なエラー文字列では何が問題でどうすればいいのかが判断できないから。 [x]

- [x] **Task S051-2-1**: フロントエンドのエラーコード→日本語メッセージ変換マップを実装（`web/src/lib/errorMessages.ts`）
  - `ERR_NO_HOST` → 「利用可能なホストがありません。しばらく待ってから再試行するか、AZ を変更してください。」
  - `ERR_QUOTA_*` → 「クォータ上限に達しています（{resource}: {current}/{limit}）。不要なリソースを削除するか、管理者にクォータ増加を依頼してください。」
  - `ERR_CONFLICT` → 「同じ名前のリソースが既に存在します。別の名前を指定してください。」
  - その他共通エラーコード対応
- [x] **Task S051-2-2**: エラー表示コンポーネント改善（`web/src/components/ErrorMessage.tsx`）: エラーコード対応メッセージ + detail からの動的補完（クォータ数値の埋め込み等）
- [x] **Task S051-2-3**: リソースエラー状態（VM `error` ステータス等）の一覧・詳細画面への表示: エラーメッセージをツールチップまたはバナーで表示
- [x] **Task S051-2-4**: Playwright テスト: 各エラーコードのメッセージ表示確認（モック使用）

**Acceptance Criteria (GUI):**
- [x] State diagram confirmed with user (see sprint-logs/S051/gui-spec-S051-2.md)
- [x] Playwright tests pass: `npx playwright test web/e2e/s051-error-ux.spec.ts`
- [x] All interactive elements have `data-testid` attributes
- [x] API calls are mocked in tests (no real backend dependency)

### Story S051-3: 運用者・開発者として、cirrusctl でエラーが発生した際に原因と対処方法がわかるメッセージを見たい。なぜなら、Go の内部エラー文字列ではユーザーが原因を特定できず、スクリプトによるエラー種別の判別もできないから。 [x]

- [x] **Task S051-3-1**: CLI エラーフォーマッタを実装（`internal/client/errors.go`）: API エラーコードを日本語メッセージ + 対処ヒントに変換
- [x] **Task S051-3-2**: 全 cirrusctl コマンドのエラー出力をフォーマッタ経由に統一
- [x] **Task S051-3-3**: `--output json` 指定時はエラーも JSON 形式で出力（スクリプト連携対応）
- [x] **Task S051-3-4**: ユニットテスト: エラーコード別メッセージ変換の確認

---

## Sprint S023: ライブマイグレーション [DONE]

同一コンピュートプール内で VM をライブマイグレーションできる。Fallback パターンによるゼロパケットロス移行。

### Story S023-0: インフラ管理者として、cirrus-sim 上でホスト間の VM ライブマイグレーションを動作させたい。なぜなら、Cirrus の実装を実機なしで開発・テストできるようにしたいから。 [x]

- [x] **Task S023-0-1**: `test/sim/libvirt/internal/handler/management.go` に `POST /sim/hosts/{src_host_id}/domains/{uuid}/migrate` endpoint 追加（body で `dest_host_id` を受け取り store の MigratePrepare → MigratePerform → MigrateFinish → MigrateConfirm を順に呼ぶ）
- [x] **Task S023-0-2**: `internal/hypervisor/driver.go` の `Driver` インターフェースに `MigrateVM(ctx, vmName, destHostID string) error` を追加し、`LibvirtDriver` で上記 HTTP API を呼ぶ実装
- [x] **Task S023-0-3**: `cmd/cirrus-sim/main.go` で共有 `fault.Engine` を作成し `libvirtSim` に渡す（`libvirtsim.Server.SetFaultEngine` メソッドも追加）

### Story S023-1: VM オペレーターとして、VM をライブマイグレーションしたい。なぜなら、サービス停止なしにホストの負荷を分散したいから。 [x]

- [x] **Task S023-1-1**: `internal/hypervisor/libvirt.go` に `MigrateVM(ctx, vmName, destHostID string) error` 実装（sim 管理 HTTP API `POST /sim/hosts/{src}/domains/{uuid}/migrate` を呼ぶ）
- [x] **Task S023-1-2**: `proto/agent.proto` に `PrepareMigration` / `StartMigration` RPC + メッセージ定義を追加、コード生成
- [x] **Task S023-1-3**: worker 側: `PrepareMigration`（宛先ホスト確認）→ `StartMigration`（`hypervisor.MigrateVM` 呼び出し）の2フェーズ実装
- [x] **Task S023-1-4**: `internal/scheduler/scheduler.go` に `Reschedule(ctx, RescheduleSpec) (*ScheduleResult, error)` を追加（移行元ホストを除外して再配置先選定）
- [x] **Task S023-1-5**: `internal/compute/service.go` に `MigrateVM(ctx, tenantID, vmID uuid.UUID, targetHostID *uuid.UUID) error` を追加 → Reschedule → PrepareMigration → StartMigration → DB 更新（status: migrating → active, host_id 更新）のオーケストレーション実装

### Story S023-2: VM オペレーターとして、マイグレーション中にネットワーク断が起きないようにしたい。なぜなら、ゼロパケットロス移行が要件だから。 [x]

- [x] **Task S023-2-1**: 移行先ホストにフロー + ポート準備（`HostNetworkState` ストリーミング経由でポート追加）
- [x] **Task S023-2-2**: 移行元ホストに Fallback 転送設定（移行先への Geneve 転送を `HostNetworkState` 拡張で配信）
- [x] **Task S023-2-3**: 他ホストのトンネル宛先更新 + ACK 管理（タイムアウト 30 秒）※本スプリントでは 3 秒 sleep で簡略化、本格 ACK 待ちは別スプリント
- [x] **Task S023-2-4**: ポート状態遷移: `active→migrating→switching→draining→active`

### Story S023-3: VM オペレーターとして、CLI から VM マイグレーションを指示したい。なぜなら、スクリプトや日常運用で使いたいから。 [x]

- [x] **Task S023-3-1**: `POST /api/v1/vms/{id}/actions` に `action=migrate` を追加（任意で `target_host_id` 指定可能）
- [x] **Task S023-3-2**: 結合テスト: VM 作成 → マイグレーション → 移行先ホストで稼働確認
- [x] **Task S023-3-3**: cirrus-sim 障害注入: `MigratePerform` 失敗 → Fallback で元ホスト継続確認
- [x] **Task S023-3-4**: `cirrusctl vm migrate <vm> [--target-host <host>]`

### Story S023-4: インフラ管理者として、HostInstance モード（docker-compose）でもコンテナ間 VM マイグレーションが E2E で動作してほしい。なぜなら、S024 以降の HA Failover テストなど複数ホスト間シナリオを実機なしで検証できるようにしたいから。 [x]

- [x] **Task S023-4-1**: `proto/agent.proto` に `AcceptMigratedVM` RPC 追加（dest worker が受け取る VM 情報）
- [x] **Task S023-4-2**: `internal/hypervisor/driver.go` に `AcceptMigratedVM(ctx, spec AcceptMigratedVMSpec) error` 追加。`LibvirtDriver` は sim mgmt API `POST /sim/hosts/{id}/domains/accept` を呼ぶ
- [x] **Task S023-4-3**: `internal/agent/worker_server.go` に `AcceptMigratedVM` gRPC handler 実装
- [x] **Task S023-4-4**: `internal/compute/orchestrator.go` の `MigrateVM` で `StartMigration` 完了後に dest worker の `AcceptMigratedVM` を呼ぶ
- [x] **Task S023-4-5**: demo: `cirrusctl vm migrate` で VM が dest ホストで `running` のまま安定すること（error に落ちない）

---

## Sprint S024: HA Failover [DONE]

ホスト障害検出時にフェンシングを行い、影響 VM を別ホストで自動再起動する。設計: docs/reconciliation.md 参照。

### Story S024-1: インフラ管理者として、障害ホストを自動的に電源断（IPMI 経由）したい。なぜなら、障害ホスト上の VM を安全に別ホストへ退避するには、まずそのホストが完全に停止していることを保証する必要があるから。 [x]

- [x] **Task S024-1-1**: `internal/controller/fencing/agent.go` FencingAgent インターフェース
- [x] **Task S024-1-2**: cirrus-sim に `/hosts/{id}/power-off` IPMI スタブ追加、FencingAgent 実装（power-off 実行 + 電源 OFF 確認ポーリング）
- [x] **Task S024-1-3**: フェンシングタイムアウト + 失敗時 Alert(critical) → failover 中止（VM は error 状態で手動対応待ち）

### Story S024-2: インフラ管理者として、障害ホスト上の VM が自動的に別ホストで再起動されてほしい。なぜなら、ホスト障害時の影響を最小化し、テナントの VM サービスを無人で自動復旧させたいから。 [x]

- [x] **Task S024-2-1**: `internal/controller/reconcile/failover.go` FailoverTrigger 実装
- [x] **Task S024-2-2**: faulty 遷移 → FencingAgent → 成功 → Reschedule → VM 再起動
- [x] **Task S024-2-3**: error 状態 VM の Reschedule → Storage.ReexportVolume → Agent.CreateVM → Network.RebindPort

### Story S024-3: インフラ管理者として、HA Failover の全シナリオ（正常・失敗・複数 VM 同時）をシミュレーション環境で検証したい。なぜなら、本番投入前に障害シナリオを安全に確認できる必要があるから。 [x]

- [x] **Task S024-3-1**: worker 停止 → faulty → フェンシング → failover → 別ホストで VM 再起動
- [x] **Task S024-3-2**: フェンシング失敗シナリオ: failover 中止 + Alert 発火
- [x] **Task S024-3-3**: 複数 VM 同時 failover: 全 VM が順次別ホストに再配置

---

## Sprint S025: DRS [ ]

コンピュートプール内のリソース偏りを検出し、自動で再配分する。S031 で leader-only 実行に wrap 予定。

### Story S025-1: インフラ管理者として、コンピュートプール内のホスト間でリソース利用率の偏りを自動的に解消したい。なぜなら、特定ホストへの集中によるパフォーマンスばらつきと障害時の影響拡大を防ぎたいから。 [x]

- [x] **Task S025-1-1**: `internal/config` に DRS ポリシー追加（`enabled`, `stddev_threshold`（既定 0.15）, `interval`（既定 5min）, `max_concurrent_migrations`（既定 2））
- [x] **Task S025-1-2**: `internal/scheduler/drs.go` Engine — AZ 単位で空きリソース割合（`free_vCPU/total`, `free_RAM/total`）の標準偏差を計算し、過負荷→低負荷へ greedy にマイグレーション計画生成（既存 `Reschedule` を再利用、計画件数は `max_concurrent_migrations` まで）
- [x] **Task S025-1-3**: `internal/controller` に DRS 周期実行ループ追加（ticker、同時実行ガード、`compute.MigrateVM` 呼び出し）。マイグレーション失敗は warn ログのみで次サイクル再試行。S031 leader gating 導入箇所を TODO コメントで明示
- [x] **Task S025-1-4**: 単体テスト（偏り計算、計画生成、greedy 選択ロジック）+ 結合テスト（ホスト間偏り → DRS 1 サイクル実行 → σ が閾値以下に改善）

### Story S025-2: インフラ管理者として、CLI から DRS を手動実行・状態確認したい。なぜなら、運用上の偏り解消対応や障害調査時に即時に DRS を起動・観察したいから。 [x]

- [x] **Task S025-2-1**: `POST /api/v1/admin/drs/run` 即時実行エンドポイント（実行中なら 409 Conflict）
- [x] **Task S025-2-2**: `GET /api/v1/admin/drs/status` 最終実行結果（開始/終了時刻、検出した σ、計画した移行件数、成功/失敗内訳）
- [x] **Task S025-2-3**: `cirrusctl admin drs run` / `cirrusctl admin drs status` コマンド

---

## Sprint S026: ホストプロファイル + Hook [ ]

ホストプロファイルを定義し、AWX hook でホストに適用できる。ロールアウトが動作する。

### Story S026-1: プロファイルモデル + Hook Executor [ ]

- [ ] **Task S026-1-1**: マイグレーション: host_profiles テーブル、hosts.profile_id, hosts.profile_status
- [ ] **Task S026-1-2**: `internal/hook/awx/awx.go` AWX REST API 実装（ジョブ実行 + ポーリング + パラメータマッピング）
- [ ] **Task S026-1-3**: cirrus-sim awx-sim への接続テスト

### Story S026-2: Profile Service + Rollout [ ]

- [ ] **Task S026-2-1**: `internal/host/profile.go` CreateProfile, ApplyProfile（Hook.Execute → profile_status 更新）
- [ ] **Task S026-2-2**: `internal/host/rollout.go` StartRollout（フォルトドメイン単位のカナリアデプロイ）
- [ ] **Task S026-2-3**: batch_size, pause_between_batches、ロールバック条件（ヘルスチェック失敗率）
- [ ] **Task S026-2-4**: POST/GET /api/v1/host-profiles, POST /rollout
- [ ] **Task S026-2-5**: 結合テスト: プロファイル作成→ロールアウト→awx-sim でジョブ実行確認
- [ ] **Task S026-2-6**: `cirrusctl host-profile/rollout`

---

## Sprint S027: スナップショット + クローン [ ]

ボリュームのスナップショット取得、スナップショットからのクローン作成、依存関係管理が動作する。

### Story S027-1: スナップショットモデル + Service 拡張 [ ]

- [ ] **Task S027-1-1**: マイグレーション: snapshots テーブル、volumes.parent_snapshot_id
- [ ] **Task S027-1-2**: CreateSnapshot → BackendDriver.CreateSnapshot + DB
- [ ] **Task S027-1-3**: DeleteSnapshot → 依存関係チェック（子クローンあれば拒否）→ BackendDriver.DeleteSnapshot
- [ ] **Task S027-1-4**: CloneFromSnapshot → BackendDriver.CloneSnapshot + DB

### Story S027-2: 依存関係グラフ + API + CLI [ ]

- [ ] **Task S027-2-1**: ボリューム→スナップショット→クローンの親子関係管理
- [ ] **Task S027-2-2**: フラット化操作（非同期）: 子をフルコピーに変換して依存を切る
- [ ] **Task S027-2-3**: POST/GET/DELETE /api/v1/volumes/{id}/snapshots, POST /api/v1/snapshots/{id}/clone
- [ ] **Task S027-2-4**: 結合テスト: ボリューム→スナップショット→クローン→削除拒否→フラット化→削除成功
- [ ] **Task S027-2-5**: `cirrusctl snapshot/clone`

---

## Sprint S028: ストレージドレイン + マイグレーション [ ]

ストレージバックエンドのライフサイクル管理とボリュームのライブマイグレーションが動作する。

### Story S028-1: バックエンドライフサイクル + ボリューム移行 [ ]

- [ ] **Task S028-1-1**: ステータス遷移: active → degraded → draining → readonly → retired
- [ ] **Task S028-1-2**: BackendDriver.MigrateVolume（同種バックエンド間）
- [ ] **Task S028-1-3**: 汎用ブロックコピー（異種バックエンド間、ホスト経由）
- [ ] **Task S028-1-4**: ドレインオーケストレーション: 依存関係考慮の移行順序算出、帯域制限、進捗追跡
- [ ] **Task S028-1-5**: 結合テスト: バックエンドドレイン→ボリューム順次移行→退役

---

## Sprint S029: テンプレートサービス [ ]

テンプレートの登録・公開・キャッシュコピーが動作し、VM 作成時のテンプレート選択が機能する。

### Story S029-1: テンプレートモデル + Service [ ]

- [ ] **Task S029-1-1**: マイグレーション: templates, template_caches テーブル
- [ ] **Task S029-1-2**: `internal/template/service.go` Create, EnsureCached
- [ ] **Task S029-1-3**: 公開範囲管理: public, organization, tenant

### Story S029-2: キャッシュ LRU 管理 + VM 統合 [ ]

- [ ] **Task S029-2-1**: last_used_at 更新 + LRU eviction（容量閾値ベース）
- [ ] **Task S029-2-2**: Compute.CreateVM: テンプレート指定時に EnsureCached → キャッシュ完了待ち → クローン
- [ ] **Task S029-2-3**: POST/GET/DELETE/PUT /api/v1/templates
- [ ] **Task S029-2-4**: 結合テスト: テンプレート作成→別バックエンドで VM 作成→キャッシュコピー→VM 起動
- [ ] **Task S029-2-5**: `cirrusctl template`

---

## Sprint S030: 監視・メトリクス + Phase 2 安定化 [ ]

Prometheus メトリクス、ヘルスチェック、宣言的トポロジの乖離検出が動作する。Phase 2 全機能が安定。

### Story S030-1: メトリクス + 乖離検出 [ ]

- [ ] **Task S030-1-1**: Prometheus exporter（/metrics エンドポイント）
- [ ] **Task S030-1-2**: ホスト/ストレージ/ネットワーク/スケジューラのメトリクス
- [ ] **Task S030-1-3**: 宣言トポロジと実態の乖離検出（ストレージ到達性、ネットワーク到達性、Capability 差異）

### Story S030-2: Phase 2 E2E テスト [ ]

- [ ] **Task S030-2-1**: ライブマイグレーション + DRS + ストレージドレインの複合シナリオ
- [ ] **Task S030-2-2**: ホストプロファイルロールアウト中の VM 可用性
- [ ] **Task S030-2-3**: cirrus-sim medium 環境での全機能テスト + 障害注入強化

---

## Sprint S031: Controller HA [ ]

Controller を複数インスタンス Active/Active 構成で運用でき、1台停止しても自動でリーダー切り替えが行われる。設計: docs/controller-ha.md 参照。

### Story S031-1: リーダー選出 [ ]

- [ ] **Task S031-1-1**: `internal/controller/leader.go` PostgreSQL アドバイザリーロックによるリーダー選出
- [ ] **Task S031-1-2**: /healthz にリーダーフラグ追加

### Story S031-2: シングルトンジョブのリーダー限定実行 [ ]

- [ ] **Task S031-2-1**: HeartbeatMonitor / ReconcileLoop / HostFaultyHandler / DRS: リーダーのみ起動/停止
- [ ] **Task S031-2-2**: `SELECT FOR UPDATE` による Scheduler 分散ロック

### Story S031-3: HA 対応 + テスト [ ]

- [ ] **Task S031-3-1**: Worker gRPC 接続のリトライロジック（agent 側）
- [ ] **Task S031-3-2**: PostgreSQL フェイルオーバー時の 503 レスポンス + pgxpool 自動リコネクト
- [ ] **Task S031-3-3**: 2台構成でリーダー選出 + 停止 → 切り替え + 同時スケジューリングで競合しないこと

---

## Sprint S032: Service Insertion（トラフィック経路挿入） [ ]

テナント Network のトラフィック経路にサービス VM（FW、IDS 等）を挿入できる。

### Story S032-1: Service Insertion 実装 [ ]

- [ ] **Task S032-1-1**: マイグレーション: service_insertions テーブル
- [ ] **Task S032-1-2**: テナント API: POST/GET/DELETE /tenants/{tid}/networks/{nid}/service-insertions
- [ ] **Task S032-1-3**: service_in / service_out ポートの自動作成
- [ ] **Task S032-1-4**: OpenFlow パイプラインに Service Insertion 分岐を追加（GW ノードへのステアリング）
- [ ] **Task S032-1-5**: ヘルスチェック: サービス VM 死活監視 + 障害時バイパス
- [ ] **Task S032-1-6**: テスト: 対象トラフィックがサービス VM 経由で転送されること

---

## Sprint S033: ゲートウェイ HA + スケールアウト [ ]

ゲートウェイノードの高可用性とスケールアウトが動作する。Active-Standby + BFD モデル。

### Story S033-1: GW HA + 無停止移動 + スケールアウト [ ]

- [ ] **Task S033-1-1**: BFD による死活監視 + Active → Standby 自動フェイルオーバー
- [ ] **Task S033-1-2**: GW ノードの無停止移動（Fallback パターン: drain フロー → 既存セッション自然タイムアウト待ち → 旧 GW から削除）
- [ ] **Task S033-1-3**: GW ペア追加による Network 割り当て再配分
- [ ] **Task S033-1-4**: テスト: 障害注入→BFD フェイルオーバー + 無停止移動 + スケールアウト

---

## Sprint S034: 複数ストレージドメイン + レプリケーション [ ]

複数ストレージドメインとリージョン間レプリケーションが動作する。

### Story S034-1: マルチストレージドメイン + レプリケーション [ ]

- [ ] **Task S034-1-1**: スケジューラ: ドメインを考慮したバックエンド選定
- [ ] **Task S034-1-2**: マイグレーション: replication_policies テーブル
- [ ] **Task S034-1-3**: レプリケーションポリシー定義（対象、宛先、頻度、保持世代数）
- [ ] **Task S034-1-4**: 定期レプリケーション実行 + 差分転送 capability 判定
- [ ] **Task S034-1-5**: テスト: cirrus-sim medium（2バックエンド: SSD/HDD）でレプリケーション確認

---

## Sprint S035: Service Endpoint（テナント間サービス公開） [ ]

テナントが自身のサービスを他テナントに公開し、消費側テナントが DNS 名で安全に接続できる。双方向 NAT によるIP アドレス空間の完全隔離。

### Story S035-1: Service Endpoint 実装 [ ]

- [ ] **Task S035-1-1**: マイグレーション: service_endpoints, endpoint_connections テーブル
- [ ] **Task S035-1-2**: テナント API: POST/GET/DELETE /tenants/{tid}/networks/{nid}/service-endpoints
- [ ] **Task S035-1-3**: CreateEndpointConnection: 消費側テナントが接続リクエスト + 承認フロー
- [ ] **Task S035-1-4**: 双方向 NAT（DNAT + SNAT）でテナント間 IP アドレス空間を完全隔離
- [ ] **Task S035-1-5**: DNS 統合: my-api.tenant-b.service.internal → VIP（100.127.0.0/16）
- [ ] **Task S035-1-6**: テスト: テナント A 公開 → テナント B 接続 → DNS 名で通信 + 双方向 NAT

---

## Sprint S036: Phase 3 安定化 [ ]

Phase 3 全機能の結合テストが通り、安定してデプロイできる状態。

### Story S036-1: 外部 IPAM 連携 + NetBox 同期 [ ]

- [ ] **Task S036-1-1**: IPAM インターフェースの外部実装（NetBox, Infoblox）
- [ ] **Task S036-1-2**: `internal/hook/netbox/` NetBox REST API 同期アダプタ（サイト/ラック/デバイス → Cirrus ロケーションツリー）

### Story S036-2: Phase 3 E2E テスト [ ]

- [ ] **Task S036-2-1**: Service Insertion + GW HA + レプリケーション + Service Endpoint の複合シナリオ
- [ ] **Task S036-2-2**: cirrus-sim medium/large 環境でのテスト

---

## Sprint S037: ファイルストレージ [ ]

NFS/CIFS 共有ボリュームが API で管理できる。

- [ ] **Task S037-1**: ファイルストレージバックエンドドライバ
- [ ] **Task S037-2**: 共有ボリュームの ACL 管理
- [ ] **Task S037-3**: VM からのマウント
- [ ] **Task S037-4**: テスト

---

## Sprint S038: オブジェクトストレージ連携 [ ]

S3 互換 API でオブジェクトストレージが使用でき、テンプレートのバックストアとして機能する。

- [ ] **Task S038-1**: MinIO 等の外部サービス連携
- [ ] **Task S038-2**: Cirrus 認証基盤との統合
- [ ] **Task S038-3**: テンプレートのオブジェクトストレージ保存
- [ ] **Task S038-4**: テスト

---

## Sprint S039: QoS + 帯域管理 [ ]

ボリューム QoS とネットワーク帯域管理が動作する。

- [ ] **Task S039-1**: ボリュームタイプ QoS ポリシーの実効化（バックエンドドライバ経由）
- [ ] **Task S039-2**: ネットワーク帯域制限（データプレーン QoS 機能）
- [ ] **Task S039-3**: noisy neighbor 制御のテスト

---

## Sprint S040: ポリシーエンジン連携 [ ]

OPA 等の外部ポリシーエンジンで認可判定が行える。

- [ ] **Task S040-1**: Authorizer インターフェースの OPA 実装
- [ ] **Task S040-2**: RBAC → OPA への段階的移行パス
- [ ] **Task S040-3**: リソース属性に基づく判定（ABAC サポート）
- [ ] **Task S040-4**: テスト

---

## Sprint S041: CMDB 同期 + Phase 4 安定化 [ ]

外部 CMDB との双方向同期が動作する。全 Phase 完了。

- [ ] **Task S041-1**: NetBox 双方向同期（Cirrus → NetBox のステータス反映）
- [ ] **Task S041-2**: 他 CMDB 対応（同期アダプタインターフェースの汎用化）
- [ ] **Task S041-3**: 全 Phase E2E テスト（cirrus-sim large 環境）
- [ ] **Task S041-4**: 負荷テスト: 2,500+ ホスト環境でのスケジューラ性能
- [ ] **Task S041-5**: ドキュメント最終更新

---

## Sprint S042: VPN Egress + Direct Connect [DONE]

NAT Gateway に加えて VPN（IPsec/WireGuard）と Direct Connect（VLAN trunk）の Egress タイプをサポートする。S020 の GW 基盤に依存。

### Story S042-1: VPN Egress [x]

- [x] **Task S042-1-1**: Egress type=vpn_ipsec: IKEv2 による IPsec トンネル設定（事前共有鍵 or 証明書）
- [x] **Task S042-1-2**: Egress type=vpn_wireguard: WireGuard トンネル設定（鍵ペア管理）
- [x] **Task S042-1-3**: GW ノードへの VPN 設定配信 + HostNetworkState 拡張

### Story S042-2: Direct Connect Egress + テスト [x]

- [x] **Task S042-2-1**: Egress type=direct_connect: VLAN trunk 設定（VLAN ID、物理ポート指定）
- [x] **Task S042-2-2**: GW ノードでの VLAN trunk 設定配信
- [x] **Task S042-2-3**: テスト: VPN トンネル確立（cirrus-sim）、Direct Connect フロー
- [x] **Task S042-2-4**: `cirrusctl egress` に vpn-ipsec/vpn-wireguard/direct-connect サブタイプ追加

### Design Notes

- **IPsec 実装**: strongSwan + govici ライブラリ（CLI ラッパー禁止ポリシーに準拠）
- **WireGuard 鍵管理**: Controller 側で鍵ペアを生成・DB に AES-GCM 暗号化保存。テナントは API 経由で公開鍵を取得
- **Direct Connect uplink ポート**: cirrus.yaml に `worker.gw.uplink_port` として記述 → worker 起動時に gRPC で Controller へ通知 → `gateway_nodes` テーブルに保存

---

## Sprint S043: L4 LB Ingress [DONE]

外部からの着信を複数 VM に分散する L4 Load Balancer Ingress を実装する。S020 の Direct IP Ingress 基盤に依存。

### Story S043-1: L4 LB Ingress 実装 [x]

- [x] **Task S043-1-1**: Ingress type=l4_lb: conntrack + DNAT による L4 分散（ラウンドロビン）
- [x] **Task S043-1-2**: conntrack セッションアフィニティ（送信元 IP ベース）
- [x] **Task S043-1-3**: コントローラ主導ヘルスチェック（TCP/HTTP probe）+ 不健全バックエンドの除外
- [x] **Task S043-1-4**: テスト: L4 LB 分散、セッションアフィニティ、バックエンド障害時の除外
- [x] **Task S043-1-5**: `cirrusctl ingress` に l4-lb サブタイプ追加

---

## Sprint S044: 内部 LB [DONE]

テナント Network 内部で VIP による L4 負荷分散ができる。各ホストの OVS で分散実行するため GW ノード不要。S020 の Network 基盤に依存。

### Story S044-1: 内部 LB 実装 [x]

- [x] **Task S044-1-1**: テナント API: POST/GET/DELETE /tenants/{tid}/networks/{nid}/load-balancers
- [x] **Task S044-1-2**: Group に VIP 割り当て、各ホストの OVS で分散実行
- [x] **Task S044-1-3**: テスト: 内部 LB 分散、VIP への疎通確認
- [x] **Task S044-1-4**: `cirrusctl load-balancer`

---

## Sprint S045: 非同期ジョブキュー基盤 [DONE]

Controller 再起動後も非同期ジョブが安全にリカバリできる。`jobs` テーブルをジョブキューとして使い、pending/running 状態のジョブを起動時に自動再実行する。

**設計方針（S045）**:
- 各 API は `202 Accepted` + `job_id` を返す（完全非同期化）
- 1 API 操作 = 1 ジョブ。VM 作成ジョブはハンドラ内部でボリューム作成を同期実行する
- ジョブ認可: tenant_member は自分が作成したジョブのみ参照可、tenant_admin はテナント内全ジョブ参照可、infra_admin は全ジョブ参照可
- 将来拡張: `parent_job_id` / `depends_on` を追加してサブジョブ依存グラフに移行する（docs/todo.md 参照）

### Story S045-1: ジョブキュー DB 基盤 [x]

- [x] **Task S045-1-1**: マイグレーション: `jobs` テーブル（id, type, status, payload JSONB, tenant_id, created_by, created_at, updated_at, started_at, completed_at, error）
- [x] **Task S045-1-2**: `internal/jobqueue/` パッケージ: JobQueue インターフェース、Enqueue/Dequeue/Complete/Fail/ListStuck
- [x] **Task S045-1-3**: Controller 起動時に status=running のジョブを pending に戻してリカバリ

### Story S045-2: 既存パイプラインの移行 [x]

- [x] **Task S045-2-1**: VM 作成/削除パイプライン（`orchestrator.go`）を JobQueue 経由に移行。API は `202 Accepted` + `job_id` を返す
- [x] **Task S045-2-2**: ボリューム作成/削除パイプライン（`storage/service_impl.go`）を JobQueue 経由に移行。API は `202 Accepted` + `job_id` を返す
- [x] **Task S045-2-3**: `GET /api/v1/jobs/{id}` エンドポイント。認可: tenant_member は自分のジョブのみ、tenant_admin はテナント内全ジョブ、infra_admin は全ジョブ参照可

### Story S045-3: テスト [x]

- [x] **Task S045-3-1**: Controller 再起動シミュレーション: 実行中ジョブが再起動後にリカバリされること確認
- [x] **Task S045-3-2**: Quota/リソース整合性: ジョブ失敗・再試行後に使用量が正しいこと確認

---

## Dependencies

フェーズ間の主要依存関係。詳細な依存ツリーは下記。

- S002 depends on S001（DB + gRPC 骨格）
- S003 depends on S002（Identity API が必要）
- S004 depends on S002（認証認可が必要）
- S005 depends on S004（Host Service が必要）
- S006 depends on S004（Host モデルが必要）
- S007 depends on S006（StorageDomain, NetworkDomain が必要）
- S008 depends on S007（ネットワーク + トポロジが必要）
- S009 depends on S008（テスト対象の機能が揃っている必要）
- S010 depends on S009（テスト基盤が前提）, S008（AZ が必要）
- S011 depends on S010
- S012 depends on S011
- S013 depends on S008（AZ フィルタが必要）
- S014 depends on S013（Storage Service が必要）
- S015 depends on S012（Network Agent 完成）, S013（Storage Service 完成）, S008（AZ 必要）
- S016 depends on S015（VM 作成フローが必要）
- S017 depends on S016（VM ステータス管理が必要）
- S018 depends on S016（heartbeat × VM 状態の照合が必要）
- S019 depends on S015（Compute/Storage/Network サービス完成後にクォータチェックを組み込む）
- S020 depends on S015（VM + ネットワーク全機能）
- S021 depends on S020（Phase 1 全機能完了が前提）
- S045 depends on S021（Phase 1 安定化後にジョブキュー基盤を導入）
- S042 depends on S020（GW 基盤・Egress 基盤が必要）
- S043 depends on S020（Direct IP Ingress 基盤・IP プールが必要）
- S044 depends on S020（Network 基盤が必要）
- S022 depends on S044（S042〜S044 で Phase 1 ネットワーク全機能完了が前提）
- S023 depends on S016（VM ライフサイクル完成）
- S024 depends on S023（マイグレーションインフラが必要）, S017（faulty 遷移トリガー）
- S025 depends on S023（MigrateVM が必要）
- S026 depends on S021（Phase 1 安定後に運用機能追加）
- S031 depends on S030（Phase 2 安定後に HA 追加）
- S032-S036 depends on S021（Phase 1 完了が前提）

---

## Backlog

- [ ] **WebUI ロール別 Admin ナビ分離**: `GET /api/v1/me`（ログインユーザー情報・ロール取得）API を追加し、フロントエンドの Admin ナビを infra_admin 向け（ホスト・ストレージ・Flavor・Quota・DriftEvent）と org_admin 向け（組織・テナント・ロール割り当て）に分離。現状は UI レベルのアクセス制御なし（API の RBAC に委ねている）。
- [ ] **S008 フォローアップ**: `make serve` での storage-domain → AZ の自動シード改善
- [ ] **S024 フォローアップ: MigrateVM/FailoverVM 共通ロジック抽出**: `internal/compute/orchestrator.go` の `MigrateVM` と `FailoverVM` が共通する「ボリューム再エクスポート → CreateVMRequest 構築 → worker.CreateVM」処理を `launchVMOnHost(ctx, vm, hostID)` ヘルパーに切り出してデュプリケーション削減。
- [ ] **S024 フォローアップ: FailoverVM の冪等性ドキュメント化**: 失敗後の再試行時に同一ボリュームへの重複 `ExportVolume` 呼び出しがストレージドライバーの冪等性に依存している点をコメントまたは docs/storage.md に明記。
- [ ] **S024 フォローアップ: FailoverTrigger 並行性ユニットテスト**: 同一ホストへの二重 `Handle()` 呼び出しが `inFlight` map で正しく防がれることを `internal/controller/reconcile/failover_test.go` に追加。
- [ ] **cirrus-sim: `handleUpdateHostConfig` データレース修正**: `test/sim/libvirt/internal/handler/management.go` の `handleUpdateHostConfig` が `host.CPUOvercommitRatio`/`MemOvercommitRatio` を無ロックで直接書き換えている。`UpdateHostConfig(hostID, cpu, mem)` を `state.Store` に追加してアトミックな更新に統一する（`handleUpdateHostState` と同じパターン）。
- [ ] **OVS クライアント移行**: S012 で実装した ExecOVSClient（os/exec）を antrea-io/ofnet に移行（OVSClient interface は不変）
- [ ] **Port API 公開**: POST /api/v1/ports（現在は Compute 統合まで内部ヘルパーのみ）→ S015 で実装
- [ ] **DriftEvent 対応判定テーブル**: reconciliation.md に基づく Alert/Auto-heal 振り分けルールの実装（S018 で対応）
- [ ] **Orchestrator goroutine ライフサイクル管理**: `compute.Orchestrator.CreateVM` が起動する `buildVM` goroutine は detached context（5分タイムアウト）で動作するため、controller シャットダウン時に最大5分孤立する可能性がある。`sync.WaitGroup` + `Orchestrator.Shutdown()` を追加して graceful shutdown に対応する。Reconciler による自動修復が前提なので本番運用フェーズ移行前に対応する。
- [ ] **Egress ポリシールーティング（宛先ベース出口選択）**: 現状は1ネットワーク1 NAT Gateway のみでネットワーク内の全 VM が同一出口を使う。VPN/Direct Connect が複数 Egress として共存する構成に向けて、`egress_routes (egress_id, dest_cidr)` テーブルを追加し「宛先 CIDR → どの Egress を使うか」をテナントが設定できるようにする。OVS フロー側では `nw_dst` マッチ＋Priority の組み合わせで実現（例: 0.0.0.0/0 → NAT、192.168.1.0/24 → VPN、172.16.0.0/12 → DX）。nat_gateway の1ネットワーク1制約は「デフォルトゲートウェイは1つ」の意味として維持し、VPN/DX は複数共存可とする。API: `POST /tenants/{tid}/networks/{nid}/egresses/{eid}/routes`、UI: Egress 詳細画面にルートテーブル管理を追加。S042（VPN/DX 実装）に依存。
- [ ] **ホスト物理リソース（vCPU/メモリ総数）未報告**: `proto/agent.proto` の `ResourceReport` に `total_vcpus`/`total_ram_mb` フィールドがなく、worker がハートビートで物理キャパシティを報告できない。`resource_physical` が常に空（`{}`）のため、ホスト管理 UI の「VCPU/メモリ 総数」列が「—」表示になる。修正箇所: (1) proto に `total_vcpus`/`total_ram_mb` 追加・再生成、(2) `internal/agent/agent.go` の `collectResources()` でハイパーバイザーの `GetNodeInfo()` 相当から総数を取得、(3) `internal/controller/grpc.go` の `Heartbeat()` ハンドラで `UpdateResourcePhysical()` を呼ぶ。

### CLI クライアント

- [ ] **名前解決のサーバーサイドフィルタ対応**: 現在 `Resolve*` は全件取得してクライアント側で名前フィルタしている。サーバー側に `?name=` クエリパラメータが入ったら切り替える。該当: `internal/client/identity.go` の `ResolveOrganization`, `ResolveTenant`

### データベース

- [ ] **UUID v7 移行**: 設計（database.md）は「UUID v7（時系列ソート可能）」だが実装は `gen_random_uuid()`（v4）。新規マイグレーションで `gen_random_uuid()` のデフォルトを UUID v7 生成関数に差し替える
- [ ] **resource_used JSONB 列の設計整合**: database.md では「vms テーブルから集計」と記載しているが、実装は heartbeat で直接上書き。heartbeat によるリアルタイム更新が正しい方式なので database.md の記載を修正する

### ホスト管理

- [ ] **Service/Store 分離**: 現在 `host.Service` インターフェースを `host.Store` が直接実装している。ビジネスロジック層（状態遷移ルール、active→maintenance 時の VM 数チェック等）を `host.Manager` に分離し、Store はデータアクセスのみに限定する
- [ ] **Heartbeat Service インターフェースの型統一**: `Heartbeat(ctx, hostID string, ...)` の `hostID` が `string` で、他メソッドの `uuid.UUID` と不整合。gRPC 境界で UUID 変換し、Service 層は `uuid.UUID` を受け取るように統一する

### API

- [ ] **PUT /api/v1/hosts/{id} 未実装**: api.md に定義があるがエンドポイント未実装。ホスト属性（address 等）の更新用

### ライブマイグレーション

- [ ] **Migration ACK ハンドシェイク**: `MigrateVM` の 3 秒 sleep（`migrationNetworkSettleTime`）を、src worker の OVS フロー適用確認を取る gRPC バージョン確認ベースのハンドシェイクに置き換える。GRPCStateServer のポーリング完了を worker が通知するか、ネットワークエージェントが state バージョンを確認する仕組みを実装する（S023 で TODO として残した）
- [ ] **マイグレーション先 AZ 検証**: REST API で `target_host_id` を直接指定する場合、VM が所属する AZ と指定ホストの AZ が一致するかを `vm_handler.go` で検証する（現状は scheduler/orchestrator 任せで AZ 制約が効かない）
- [ ] **孤立 FallbackRoute レコードの定期クリーンアップ**: `migration_fallback_routes` テーブルを定期スキャンし、`created_at` が N 分以上前で対応 VM が `migrating` でないレコードをガベージコレクトする Reconciler を追加する（異常終了時の OVS フロー残留対策）
- [ ] **MigrateVM エラーパスのテスト拡充**: `orchestrator_test.go` に以下のケースを追加: FallbackRoute insert 失敗時の VM ステータスロールバック、`ErrConflict` 返却（transitional/stopped VM）、`setVMHost` 失敗後の fallback route defer 動作確認

### ネットワーク

- [ ] **ネットワークモジュールの OVN→VPC モデル移行（Sprint 5N）**: Sprint 5 の既存 OVN 実装を新しい VPC モデル（Network/Group/Policy + OVS データプレーン）に全面書き換え。前提として Sprint 5S（cirrus-sim 統合）を先に完了させる
- [ ] **NetworkDomain.OVNNBConnection フィールド削除**: `internal/topology/models.go` の `OVNNBConnection` フィールド、`internal/state/migrations/000004_topology.up.sql` の `ovn_nb_connection` カラムを削除。Sprint 5S でバリデーションを任意に緩和済み（`internal/api/topology_handler.go`）。Sprint 5N（OVN→VPC 移行）と合わせて対応
- [ ] **controller の --ovn-nb フラグ削除**: `internal/config/config.go` の `OVNNBConnection` 設定項目を削除。Sprint 5N と合わせて対応
- [ ] **client の OVNNBConnection 参照削除**: `internal/client/topology.go` の関連コード。Sprint 5N と合わせて対応
- [ ] **cirrus-sim リポジトリのアーカイブ**: OVN→VPC 移行完了・動作確認後にアーカイブ化

### 非同期ジョブキュー

- [ ] **ジョブ依存グラフ（入れ子ジョブ）**: S045 では「1 API 操作 = 1 ジョブ」として実装。将来的には `jobs` テーブルに `parent_job_id`/`depends_on` カラムを追加し、VM 作成ジョブがボリューム作成サブジョブを生成できる依存グラフ構造（A2 パターン）に拡張する。各サブ操作が独立してリカバリ・再試行できるようになる
