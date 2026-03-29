# 実装ロードマップ

各スプリントは独立してデプロイ・動作確認可能な単位で設計している。
テストは3レイヤー（Goユニットテスト、OVSモッククライアント、実OVS + docker-compose結合テスト）で構成する。

---

## Phase 1: 最小構成（Sprint 1〜12 + CLI）

ゴール: Single管理ドメインで、VM作成→ネットワーク接続→ボリュームアタッチ→削除の基本フローが動く。

---

### Sprint 1: プロジェクト骨格 ✅

**ゴール**: バイナリがビルドでき、controller/workerとして起動し、cirrus-simに接続できる。

#### S1-1: プロジェクト初期化
- [x] Go module初期化、ディレクトリ構成作成（architecture.mdのディレクトリ構成に準拠）
- [x] cmd/cirrus/main.go: controller/workerサブコマンド（cobra）
- [x] internal/config: 設定構造体（CLIフラグで注入）
- [x] Makefile: build, test, lint, proto, serve, stop, logs ターゲット
- [x] .gitignore, cirrus.yaml.example

#### S1-2: State モジュール基盤
- [x] internal/state/db.go: PostgreSQL接続（pgx）、コネクションプール
- [x] internal/state/migrations/: golang-migrate導入、初回マイグレーション（hostsテーブル）
- PostgreSQLは外部で起動済みの前提（DB_DSN Makefile変数で接続先指定）

#### S1-3: API骨格
- [x] internal/api/router.go: chi HTTPルーター
- [x] internal/api/middleware.go: RequestID、Logger、Recovery
- [x] GET /healthz エンドポイント（DB接続チェック）

#### S1-4: gRPC骨格
- [x] proto/agent.proto: ControllerService（Heartbeat RPC）
- [x] protoc生成（protoc-gen-go / protoc-gen-go-grpc）
- [x] internal/controller/grpc.go: controller側gRPCサーバ（heartbeat受信）
- [x] internal/agent/agent.go: worker側gRPCクライアント（10秒間隔heartbeat送信）

#### S1-5: cirrus-sim接続確認
- [x] worker起動時にcirrus-sim（libvirt-sim）へTCP接続確認
- [x] make serve でcirrus-sim + controller + worker(10台)が連携起動
- [x] healthzが200を返す

**技術選定**: cobra (CLI), chi (HTTP), pgx (PostgreSQL), golang-migrate, gRPC, slog (logging)

**デプロイ確認**: `make serve` → controller + worker×10起動 → healthz OK → 全ホストからheartbeat受信 → libvirt-sim接続OK

---

### Sprint 2: Identity（認証・認可・テナント管理） ✅

**ゴール**: 組織・テナントをAPIで作成でき、静的トークンで認証、RBACで認可判定が動く。

#### S2-1: テナントモデルのDB
- [x] マイグレーション: organizations, tenants, users, role_assignments テーブル
- [x] internal/identity/models.go: Organization, Tenant, User, RoleAssignment 構造体

#### S2-2: Identity Service
- [x] internal/identity/service.go: Service インターフェース定義
- [x] internal/identity/store.go: CreateOrganization, CreateTenant, AssignRole 実装
- [x] テスト: RBAC認可のユニットテスト

#### S2-3: 認証
- [x] internal/identity/authenticator.go: Authenticator インターフェース定義
- [x] 静的トークン認証実装（開発用）: CLIフラグでtoken→external_idマッピング
- [x] internal/api/auth_middleware.go: 認証ミドルウェア（Authorizationヘッダ解析）

#### S2-4: 認可
- [x] internal/identity/authorizer.go: Authorizer インターフェース定義
- [x] RBAC実装: infra_admin, org_admin, tenant_admin, tenant_member
- [x] internal/api/auth_middleware.go: X-Tenant-IDヘッダからテナントスコープ解決

#### S2-5: APIエンドポイント
- [x] POST/GET /api/v1/organizations
- [x] POST/GET /api/v1/organizations/{org_id}/tenants
- [x] POST/GET/DELETE /api/v1/tenants/{id}/role-assignments
- [x] 結合テスト: 認証→認可→テナント操作の一連フロー

**デプロイ確認**: APIでorg作成→テナント作成→ロール付与→権限のないユーザが403

---

### Sprint 2.5: CLIクライアント（cirrusctl） ✅

**ゴール**: CLIクライアントでSprint 2（Identity）の全機能を操作できる。以降のSprintで機能追加に合わせてCLIも拡充する。

#### S2.5-1: CLIクライアント基盤
- [x] cmd/cirrusctl/main.go: ルートコマンド + グローバルフラグ（--endpoint, --token, --output）
- [x] internal/client/client.go: HTTP APIクライアント（認証ヘッダ付与、エラーハンドリング）
- [x] Makefile: build に cirrusctl 追加

#### S2.5-2: Identity操作コマンド
- [x] org create/list/show: 組織のCRUD
- [x] tenant create/list/show: テナントのCRUD
- [x] role assign/list/delete: ロール割り当て管理

#### S2.5-3: 出力フォーマット
- [x] デフォルト: テーブル形式（text/tabwriter）
- [x] --output json: JSON出力

**技術選定**: cobra (CLI), text/tabwriter (テーブル出力)

**デプロイ確認**: `cirrusctl org create "ACME"` → `cirrusctl org list` → `cirrusctl tenant create <org-id> dev` → `cirrusctl role list <tenant-id>`

---

### Sprint 3: Host管理 + Worker Agent ✅

**ゴール**: ホストを登録し、workerがheartbeatを送り、capabilityとリソースが管理される。

#### S3-1: HostモデルのDB
- [x] マイグレーション: hosts, host_storage_domains テーブル
- [x] capability JSONB、resource_physical JSONB、overcommit_ratios JSONB

#### S3-2: Host Service
- [x] internal/host/service.go: Service インターフェース定義
- [x] internal/host/store.go: Register, UpdateCapability, SetOperationalState, Heartbeat
- [x] internal/host/store.go: GetAllocatable（スケジューラ向け）

#### S3-3: Worker Agent
- [x] proto/agent.proto: Heartbeat RPC にResourceReport追加（VMInfo含む）
- [x] internal/agent/agent.go: 定期heartbeat送信（リソース情報含む）
- [x] controller側: heartbeat受信→hosts テーブル更新

#### S3-4: Hypervisor接続
- [x] internal/hypervisor/driver.go: Driver インターフェース定義
- [x] internal/hypervisor/libvirt.go: cirrus-sim HTTP API経由でListVMs, GetHostInfo
- [x] worker起動時にcirrus-sim（libvirt-sim）に接続し、ホスト情報取得

#### S3-5: APIエンドポイント + テスト
- [x] POST/GET /api/v1/hosts（インフラ管理者）
- [x] POST /api/v1/hosts/{id}/actions（maintenance, activate, drain, retire）
- [x] GET /api/v1/hosts/{host_id}

#### S3-6: CLIクライアント
- [x] cirrusctl host list/show コマンド追加
- [x] cirrusctl host maintenance/activate コマンド追加

**デプロイ確認**: worker起動→cirrus-simに接続→heartbeat→GET /api/v1/hostsでリソース確認

---

### Sprint 3.5: Worker自動登録（登録トークン方式） ✅

**ゴール**: workerが登録トークンで安全にcontrollerに自己登録し、管理者の承認（activate）を経てVM配置対象になる。事前のAPI手動登録を不要にする。

#### S3.5-1: 登録トークン
- [x] internal/config: controller に `--registration-token` フラグ追加
- [x] 登録トークンは共有シークレット（worker設定ファイルに記載）
- [x] Makefile: AUTH_TOKENS と同様に `REGISTRATION_TOKEN` 変数で注入

#### S3.5-2: Worker自己登録 gRPC
- [x] proto/agent.proto: RegisterHost RPC 追加（registration_token, hostname, address, capability）
- [x] worker起動時: ホスト情報（hostname, address, libvirt経由のcapability）を収集
- [x] worker起動時: RegisterHost を呼び出し、自ホストをcontrollerに登録
- [x] controller側: トークン検証 → hosts テーブルに registering 状態で INSERT（既存なら無視）
- [x] RegisterHost レスポンスで割り当てられた host UUID を返却
- [x] worker は以降の heartbeat で UUID を使用（名前マッチ廃止）

#### S3.5-3: Heartbeat を UUID ベースに移行
- [x] worker: RegisterHost で取得した UUID を heartbeat の host_id に使用
- [x] controller: heartbeat は UUID のみでマッチ（名前マッチを廃止）
- [x] 未登録ホストからの heartbeat は拒否（accepted=false）

#### S3.5-4: 管理者承認フロー
- [x] registering 状態のホスト一覧表示（`cirrusctl admin host list --pending`）
- [x] `cirrusctl admin host activate` で registering → active に承認
- [x] 承認前のホストはスケジューラの配置対象外

#### S3.5-5: Makefile 対応
- [x] `_register-hosts` ターゲットを削除（worker自己登録に移行）
- [x] worker 起動時に `--registration-token` を渡す
- [x] worker 起動後に自動で全ホストを activate する開発用ステップ追加

#### S3.5-6: テスト
- [x] 無効なトークンでの登録拒否テスト
- [x] worker起動→自動登録→管理者activate→heartbeat正常の一連フロー
- [x] 同一ホスト名での重複登録が冪等であることの確認

**技術選定**: gRPC Registration RPC, 共有シークレットトークン（将来的にmTLS移行可能な設計）

**デプロイ確認**: worker起動→自動登録（registering）→`cirrusctl admin host list --pending`→activate→heartbeat正常→リソース表示

---

### Sprint 4: Topology（到達性ドメイン・ロケーション） ✅

**ゴール**: ストレージ/ネットワークドメイン、ロケーションツリーが登録でき、コンピュートプールが導出される。

#### S4-1: ドメインモデルのDB
- [x] マイグレーション: storage_domains, network_domains, locations テーブル
- [x] locations: parent_id自己参照、type (site/floor/row/rack/unit)、fault_attributes JSONB

#### S4-2: Topology Service
- [x] internal/topology/service.go: Service インターフェース定義
- [x] CreateStorageDomain, CreateNetworkDomain
- [x] ホスト⇔ストレージドメイン関連付け（host_storage_domains）
- [x] ホスト⇔ネットワークドメイン関連付け（hosts.network_domain_id）

#### S4-3: コンピュートプール導出
- [x] GetComputePool: ストレージドメイン ∩ ネットワークドメインのホスト集合
- [x] ListReachableHosts(backendID): バックエンドの所属ドメインから到達可能ホスト
- [x] ListReachableBackends(hostID): ホストの所属ドメインから到達可能バックエンド

#### S4-4: ロケーション管理
- [x] ロケーションツリーCRUD
- [x] WITH RECURSIVE によるパス取得、サブツリー検索
- [x] フォルトドメイン導出（指定階層でのグルーピング）

#### S4-5: APIエンドポイント + テスト
- [x] POST/GET /api/v1/storage-domains
- [x] POST/GET /api/v1/network-domains
- [x] POST/GET /api/v1/locations
- [x] GET /api/v1/compute-pools（導出結果）
- [x] 結合テスト: ドメイン作成→ホスト関連付け→コンピュートプール導出確認

#### S4-6: CLIクライアント
- [x] cirrusctl storage-domain/network-domain/location コマンド追加
- [x] cirrusctl compute-pool list コマンド追加

**デプロイ確認**: ドメイン・ロケーション登録→コンピュートプールAPIで導出結果確認

---

### Sprint 5: Network基盤（OVN） ✅ [Sprint 5Nで置換]

> **注意**: このSprintはOVNベースのネットワーク実装として完了済み。Sprint 5N（VPCモデル）で全面置換される。

**ゴール**: テナントネットワーク/サブネット/ポートをAPIで作成でき、OVN（cirrus-sim ovn-sim）に反映される。

#### S5-1: OVNクライアント
- [x] internal/network/ovn/client.go: OVNClient インターフェース定義
- [x] internal/network/ovn/ovsdb.go: OVSDBプロトコル実装（JSON-RPC over TCP）
- [x] CreateLogicalSwitch, CreateLogicalSwitchPort, DeleteLogicalSwitch, DeleteLogicalSwitchPort
- [x] cirrus-sim ovn-simへの接続テスト

#### S5-2: ネットワークモデルのDB
- [x] マイグレーション: networks, subnets, ports テーブル
- [x] networks.network_domain_id 外部キー

#### S5-3: IPAM
- [x] internal/network/ipam/ipam.go: IPAM インターフェース定義
- [x] internal/network/ipam/builtin.go: DB上のCIDR演算による内蔵IPAM
- [x] AllocateIP, ReleaseIP
- [x] MACアドレス生成（02:xx:xx:xx:xx:xx、UNIQUE制約で衝突防止）

#### S5-4: Network Service
- [x] internal/network/service.go: Service インターフェース定義
- [x] CreateNetwork → DB + OVN Logical Switch作成
- [x] CreateSubnet → DB + OVN DHCP Options作成
- [x] CreatePort → DB + IPAM IP払い出し + OVN LSP作成
- [x] DeleteNetwork/Subnet/Port（逆順の削除）

#### S5-5: OVN Reconciler 基礎（docs/reconciliation.md 参照）
- [x] internal/controller/reconcile/network.go: OVNReconciler実装
- [x] reconcile loop（デフォルト5分間隔）でOVN NBをスキャン（Logical Switch, LSP）
- [x] DB上のnetworks/portsと照合
- [x] 初期実装はログ出力のみ（DriftEvent基盤はSprint 8.5で完成後に移行）
- [x] 遷移中ステータスのリソースはreconcile対象外とする

#### S5-6: APIエンドポイント + テスト
- [x] POST/GET/DELETE /api/v1/networks
- [x] POST/GET/DELETE /api/v1/networks/{id}/subnets
- [x] POST/GET/DELETE /api/v1/ports
- [x] 結合テスト: ネットワーク作成→ovn-simにLogical Switchが作成される

#### S5-7: CLIクライアント
- [x] cirrusctl network/subnet/port コマンド追加

**デプロイ確認**: APIでネットワーク/サブネット/ポート作成→ovn-simの管理APIで確認

---

### Sprint 5.5: テナント向けリソース抽象化（AZ導入） ✅

**ゴール**: Availability Zone（AZ）を導入し、テナント API からインフラ詳細を隠蔽する。テナントは AZ 名とリソース名だけで操作できる。AZ はネットワークドメインとは独立した概念であり、ロケーション階層とストレージドメインを束ねる運用単位。設計詳細は [docs/tenant-model.md](tenant-model.md) を参照。

#### S5.5-1: Availability Zone モデル
- [x] マイグレーション: availability_zones, az_storage_domains テーブル
- [x] internal/az/models.go: AvailabilityZone 構造体
- [x] internal/az/service.go: Service インターフェース定義
- [x] internal/az/store.go: CRUD実装

#### S5.5-2: AZ 管理者 API + CLI
- [x] POST/GET/PUT/DELETE /api/v1/availability-zones（管理者）
- [x] POST/DELETE /api/v1/availability-zones/{id}/storage-domains（SD紐付け）
- [x] cirrusctl admin az create/list/show/delete コマンド

#### S5.5-3: AZ テナント API
- [x] GET /api/v1/availability-zones（テナント向け: 利用可能AZ一覧）
- [x] GET /api/v1/availability-zones/{id}（AZ詳細: 名前、説明）
- [x] RBAC: 全テナントロールで AZ 一覧参照可能

#### S5.5-4: Network API からインフラ詳細を隠蔽
- [x] POST /api/v1/networks: テナント API から network_domain_id を除去
- [x] CLI: cirrusctl network create から --network-domain を除去

#### S5.5-5: make serve での AZ 自動シード
- [x] Makefile: トポロジシード時にデフォルト AZ を自動作成（Location + SD 紐付け）
- [x] 既存の default-sd, default-site を AZ に集約

#### S5.5-6: テスト
- [x] AZ CRUD テスト
- [x] ネットワーク作成がインフラ詳細なしで動作すること
- [x] cirrus-sim 結合テスト

**デプロイ確認**: `cirrusctl network create app-net --tenant t1 --org o1` で network_domain 指定なしにネットワーク作成できる

---

### Sprint 5S: cirrus-sim統合 + テスト基盤構築

**ゴール**: cirrus-simリポジトリをcirrusに統合し、3レイヤーテスト体制を構築する。Sprint 5N（ネットワーク再設計）の前提となるテスト基盤を整備する。

#### S5S-1: シミュレータコード移行
- [ ] cirrus-simリポジトリからstorage-sim をcirrus/test/sim/storage/ に移行（API互換維持）
- [ ] cirrus-simリポジトリからawx-sim をcirrus/test/sim/awx/ に移行（API互換維持）
- [ ] cirrus-simリポジトリからcommon（イベントログ、障害注入、データジェネレータ）をcirrus/test/sim/common/ に移行
- [ ] cirrus-simリポジトリからembedded PostgreSQL をcirrus/test/sim/postgres/ に移行
- [ ] OVN-simは廃止（OVSは結合テストで実物を使用）
- [ ] NetBox-simは廃止（NetBox連携はPhase 3で外部IPAM/CMDBとして実装）
- [ ] 環境定義YAML（small/medium/large）をcirrus/test/sim/environments/ に移行

#### S5S-2: libvirtd-simのホスト単位分割 + VMシミュレーション
- [ ] 現在の libvirtd-sim（1プロセスで全ホストをシミュレート）をホスト単位に分割
  - 各workerコンテナ内で独立した libvirtd-sim インスタンスが1ホスト分を担当
  - libvirt RPCプロトコル互換は維持（go-libvirtから接続可能）
- [ ] VM作成時のnetwork namespace + veth実装（QEMUの代替）
  - DomainDefineXMLFlags → ドメイン定義を保持（メモリ管理）
  - DomainCreateWithFlags → network namespace作成 + vethペア作成 + OVSポート接続
  - DomainDestroyFlags → namespace削除 + vethペア削除 + OVSポート削除
  ```bash
  # VM作成時の処理
  ip netns add vm-${uuid}
  ip link add vm-${uuid}-tap type veth peer name eth0 netns vm-${uuid}
  ip link set vm-${uuid}-tap up
  ovs-vsctl add-port br-int vm-${uuid}-tap -- set Interface vm-${uuid}-tap external_ids:iface-id=${port_id}
  ip netns exec vm-${uuid} dhclient eth0
  ```
- [ ] namespace内でping/curl/dig実行可能なことの確認（エンドツーエンドテストの前提）
- [ ] ドメインXMLからinterfaceid（PortID）とディスク情報をパースし、OVSポートのexternal_idsに設定
- [ ] ライブマイグレーションシミュレーション: namespace + vethの移動（DomainMigratePerform3Params等）

#### S5S-3: docker-compose結合テスト基盤
- [ ] test/integration/Dockerfile.worker: cirrus-sim-workerイメージ
  ```dockerfile
  FROM ubuntu:24.04
  RUN apt-get install -y openvswitch-switch iproute2 iputils-ping dnsutils curl
  COPY cirrus /usr/local/bin/cirrus           # cirrus-agent（実バイナリ）
  COPY libvirtd-sim /usr/local/bin/libvirtd-sim  # シミュレータ
  COPY entrypoint.sh /entrypoint.sh
  ```
- [ ] test/integration/entrypoint.sh: コンテナ起動スクリプト
  ```bash
  #!/bin/bash
  ovsdb-server --remote=punix:/var/run/openvswitch/db.sock --detach
  ovs-vswitchd --detach
  ovs-vsctl add-br br-int
  libvirtd-sim &       # 1ホスト分のlibvirt RPCシミュレータ
  cirrus --role worker &  # cirrus-agent含む
  wait
  ```
- [ ] test/integration/docker-compose.yml: controller + postgres + worker×3(privileged) + storage-sim + awx-sim + fabricネットワーク
- [ ] workerコンテナはprivileged（network namespace操作のため）
- [ ] fabricネットワーク: workerコンテナ間のGeneveトンネル通信用

#### S5S-4: OVSモッククライアント（レイヤー2テスト用）
- [ ] test/mock/ovs/: MockOVSClient interface
  ```go
  type MockOVSClient interface {
      AddFlow(table int, priority int, match string, actions string) error
      DeleteFlow(table int, match string) error
      AddPort(bridge string, port string) error
      DeletePort(bridge string, port string) error
      GetRecordedCommands() []OVSCommand
  }
  ```
- [ ] フロー変換ロジックのテストに使用（実OVS不要、レイヤー2テスト）

#### S5S-5: Makefile + CI
- [ ] Makefile: test-unit（レイヤー1: Goユニットテスト）ターゲット
- [ ] Makefile: test-mock（レイヤー2: OVSモッククライアント）ターゲット
- [ ] Makefile: test-integration（レイヤー3: docker-compose + 実OVS）ターゲット
- [ ] make serve 更新: 統合されたシミュレータを使用するよう変更
- [ ] make stop / make logs 更新

#### S5S-6: cirrus-sim CLIツール
- [ ] cmd/cirrus-sim-ctl/: 状態確認コマンド（status, hosts list, vms list, backends list, volumes list）
- [ ] 障害注入コマンド（fault inject/list/clear）
- [ ] 状態管理コマンド（snapshot save/restore/list, reset）
- [ ] ポート自動検出（portman envファイル）

#### S5S-7: 障害注入の各シミュレータ統合
- [ ] libvirtd-sim: RPCハンドラでfault.Check()を呼び、マッチ時にエラー/遅延/タイムアウトを発生
- [ ] storage-sim: APIハンドラでfault.Check()を呼ぶ
- [ ] awx-sim: ジョブ実行でfault.Check()を呼ぶ

#### S5S-8: 既存テストの移行確認 + 結合テスト基盤動作確認
- [ ] Sprint 1-5.5の既存テストが統合後のシミュレータで動作することを確認
- [ ] make serve → make test が通ること
- [ ] docker-compose up → workerコンテナ内でOVS + libvirtd-sim + namespace作成が動作すること
- [ ] workerコンテナ間でGeneveトンネルが疎通すること（Sprint 5Nの前提確認）
- [ ] cirrus-simリポジトリをアーカイブ

**デプロイ確認**: make serve で統合シミュレータが起動 → 既存テスト全パス → make test-integration でdocker-compose環境が動作

---

### Sprint 5N: ネットワーク再設計（VPCモデル）

**ゴール**: OVNを廃止し、OVSデータプレーン + cirrus-agentによるVPCモデルネットワークに全面移行する。

#### S5N-1: ネットワークデータモデル移行
- [ ] マイグレーション: networks テーブル改修（cidr CIDR追加、vni INTEGER UNIQUE追加、network_domain_id削除）
- [ ] マイグレーション: subnets テーブル廃止
- [ ] マイグレーション: groups テーブル新設（network_id FK、name、UNIQUE(network_id, name)）
- [ ] マイグレーション: policies テーブル新設（network_id FK、src_group_id、dst_group_id、protocol、dst_port、priority、action）
- [ ] マイグレーション: ports テーブル改修（subnet_id削除、group_id FK追加、host_id追加、role追加、UNIQUE(vm_id, role)）
- [ ] マイグレーション: egresses テーブル新設
- [ ] マイグレーション: ingresses テーブル新設
- [ ] マイグレーション: gateway_nodes テーブル新設
- [ ] マイグレーション: service_insertions テーブル新設
- [ ] マイグレーション: load_balancers テーブル新設
- [ ] マイグレーション: network_domains テーブル廃止、hosts.network_domain_id削除
- [ ] マイグレーション: routers, router_interfaces, security_groups, security_group_rules, port_security_groups, floating_ips テーブル廃止

#### S5N-2: IPAM（/30ブロック採番）
- [ ] internal/network/ipam.go: NetworkのCIDRから/30ブロックを順番に払い出し
- [ ] CIDRプール管理（デフォルト100.64.0.0/10、VPN/専用線用はユーザ指定）
- [ ] 削除されたVMのIPは再利用しない（conntrackステート残存リスク回避）
- [ ] MACアドレス生成（既存ロジック流用）
- [ ] VNI自動採番（Network作成時にユニークVNI割当）
- [ ] テスト: /30採番ロジック、CIDR枯渇、VNIユニーク性

#### S5N-3: Network/Group/Policy Service
- [ ] internal/network/model.go: Network, Group, Policy, Port 構造体
- [ ] internal/network/service.go: Service インターフェース定義（CreateNetwork, CreateGroup, CreatePolicy, CreatePort等）
- [ ] Network CRUD: DB + CIDR/VNI割当
- [ ] Group CRUD: DB（フロー変更なし、同期レスポンス）
- [ ] Policy CRUD: DB + HostNetworkState再計算
- [ ] Port CRUD（内部API）: IP/MAC払い出し + Group割り当て
- [ ] テスト: Network/Group/Policy のCRUDユニットテスト

#### S5N-4: HostNetworkState計算・配信
- [ ] internal/network/controller.go: HostNetworkState計算ロジック
- [ ] ホストごとに「自ホスト上のVM」＋「関連するリモートVM」＋「Policyルール」＋「DNSレコード」を集約
- [ ] proto/network.proto: NetworkStateService（gRPC server streaming）
- [ ] StreamHostNetworkState: 初回全状態送信 + 以降差分ストリーミング
- [ ] HostNetworkState proto message（PortState, PolicyRule, RemotePort, DnsRecord）
- [ ] テスト: 状態計算のユニットテスト、差分計算テスト

#### S5N-5: OVSエージェント
- [ ] internal/network/agent.go: OVS OpenFlowフロー管理
- [ ] HostNetworkState → OpenFlowフロー変換ロジック
- [ ] OpenFlowパイプライン実装（Table 0-7: 入力分類→conntrack→宛先GROUP_ID解決→Policy評価→宛先ホスト解決→Geneveカプセル化→ローカル出力→Egress処理）
- [ ] OVSクライアント: AddFlow, DeleteFlow, AddPort, DeletePort
- [ ] Port Security（MACスプーフィング防止）
- [ ] conntrackベースのステートフル制御
- [ ] テスト: レイヤー2（MockOVSClient）でフロー変換を検証

#### S5N-6: DHCP応答
- [ ] エージェント内DHCPサーバ
- [ ] /30サブネット情報配布（IP、Mask、Gateway、DNS）
- [ ] OVSフローでDHCP要求をエージェントに転送
- [ ] テスト: DHCPリクエスト→レスポンス検証

#### S5N-7: DNS
- [ ] エージェント内DNSサーバ（組み込み、CoreDNS等の外部依存なし）
- [ ] レコード種類: VM個別（vm-1.api.my-app.internal）、Group全体（Aレコード複数返し）、逆引き
- [ ] Network間隔離: 送信元IPからNetwork IDを解決し、そのNetworkのレコードだけを返す
- [ ] 外部DNSフォワード: 内部レコード非該当の問い合わせをフォワード
- [ ] DNSレコードはHostNetworkStateに含めて配信
- [ ] テスト: レコード生成ロジック、Network隔離

#### S5N-8: メタデータサービス
- [ ] エージェント内HTTPサーバ（169.254.169.254）
- [ ] OVSフローで169.254.169.254宛をエージェントに転送
- [ ] 送信元IPからVM識別→メタデータ返却（vm_id, network, group, interfaces, hostname等）
- [ ] cloud-init統合: DHCP→ネットワーク確立→メタデータ取得→cloud-init完了
- [ ] テスト: メタデータレスポンス生成

#### S5N-9: API + CLI
- [ ] POST/GET/DELETE /api/v1/networks（テナント）: name, cidr(optional) 指定
- [ ] POST/GET/DELETE /api/v1/networks/{nid}/groups
- [ ] POST/GET/DELETE /api/v1/networks/{nid}/policies
- [ ] GET /api/v1/networks/{nid}/ports（テナント: 読み取りのみ）
- [ ] cirrusctl network/group/policy コマンド追加
- [ ] 旧 subnet/router/security-group/floating-ip コマンド廃止

#### S5N-10: Network Reconciler
- [ ] internal/controller/reconcile/network.go: NetworkReconciler実装
- [ ] 各ホストのOVSフロー状態 vs 期待されるHostNetworkStateを比較
- [ ] 初期実装はログ出力のみ（DriftEvent基盤はSprint 8.5で移行）
- [ ] 遷移中ステータスのリソースは除外

#### S5N-11: 結合テスト（Sprint 5Sのテスト基盤を使用）
- [ ] テストケース: VM(namespace)間のGeneveトンネル通信
- [ ] テストケース: DHCP応答でIP/GW/DNS取得
- [ ] テストケース: DNS応答とNetwork隔離
- [ ] テストケース: Policy（conntrack）による通信許可/拒否
- [ ] テストケース: メタデータサービスへのアクセス
- [ ] テストケース: HostNetworkState差分配信でフロー更新

**技術選定**: OVS (OpenFlow 1.3), Geneve, gRPC server streaming, network namespaces (テスト用)

**デプロイ確認**: Network作成→Group作成→Policy定義→VM作成→namespace間通信→DNS/DHCP/メタデータ全動作

---

### Sprint 6: Storage基盤

**ゴール**: ストレージバックエンドを登録し、ボリュームをAPIで作成でき、cirrus-sim storage-simに反映される。

#### S6-1: バックエンドドライバ
- [ ] internal/storage/driver/driver.go: BackendDriver インターフェース定義
- [ ] cirrus-sim storage-sim用ドライバ実装（REST API呼び出し）
- [ ] CreateVolume, DeleteVolume, ExportVolume, UnexportVolume
- [ ] Capabilities() 返却

#### S6-2: ストレージモデルのDB
- [ ] マイグレーション: storage_backends, volume_types, volumes テーブル
- [ ] storage_backends.storage_domain_id 外部キー
- [ ] volume_types: required_capabilities JSONB、qos_policy JSONB

#### S6-3: Storage Service
- [ ] internal/storage/service.go: Service インターフェース定義
- [ ] RegisterBackend, CreateVolume, DeleteVolume, ResizeVolume
- [ ] ボリューム作成時: ボリュームタイプのcapability要件でバックエンド選定
- [ ] AttachVolume/DetachVolume（メタデータ管理、実際のエクスポートはドライバ経由）

#### S6-4: APIエンドポイント
- [ ] POST/GET /api/v1/storage-backends（インフラ管理者）
- [ ] POST/GET /api/v1/volume-types（インフラ管理者作成 + テナント向け一覧）
- [ ] GET /api/v1/volume-types（テナント: 利用可能な Volume Type 一覧）
- [ ] POST/GET/DELETE /api/v1/volumes（テナント操作、volume_type_id + az(optional) 指定）
- [ ] POST /api/v1/volumes/{id}/attach, /detach

#### S6-5: Storage Reconciler 基礎（docs/reconciliation.md 参照）
- [ ] internal/controller/reconcile/storage.go: StorageReconciler実装
- [ ] reconcile loop（デフォルト5分間隔）で各バックエンドにListVolumes問い合わせ
- [ ] DB上のvolumesと照合
- [ ] 初期実装はログ出力のみ（DriftEvent基盤はSprint 8.5で完成後に移行）
- [ ] 遷移中ステータス（creating, deleting, migrating）のボリュームは除外

#### S6-6: テスト
- [ ] 結合テスト: バックエンド登録→ボリューム作成→storage-simに作成される
- [ ] ボリュームタイプのcapabilityマッチングテスト

#### S6-7: CLIクライアント
- [ ] cirrusctl storage-backend/volume-type/volume コマンド追加

**デプロイ確認**: バックエンド登録→ボリューム作成→storage-simの管理APIで確認

---

### Sprint 7: Scheduler + VM作成

**ゴール**: VM作成APIが動作し、スケジューラがホストとバックエンドを選定し、cirrus-sim上でVMが起動する。

#### S7-0: Flavor エンティティ（docs/tenant-model.md 参照）
- [ ] マイグレーション: flavors, flavor_access テーブル
- [ ] internal/flavor/models.go, service.go, store.go: Flavor CRUD
- [ ] 管理者 API: POST/GET/PUT/DELETE /api/v1/flavors
- [ ] テナント API: GET /api/v1/flavors（利用可能な Flavor 一覧）
- [ ] Flavor アクセス制御: public フラグ + flavor_access テーブル
- [ ] CLI: cirrusctl admin flavor create/list/show + cirrusctl flavor list
- [ ] make serve: デフォルト Flavor シード（m1.small: 1vCPU/1GB, m1.medium: 2/4, m1.large: 4/8）

#### S7-1: Scheduler
- [ ] internal/scheduler/scheduler.go: Scheduler インターフェース定義
- [ ] internal/scheduler/filter.go: フィルタリング
  - AZ フィルタ（AZ → Compute Pool への解決）
  - Flavor → Capability マッチング
  - 稼働状態フィルタ（active のみ）
- [ ] internal/scheduler/scorer.go: スコアリング
  - ホスト: リソース空き率
  - バックエンド: 容量空き率
- [ ] Schedule() → (host_id, backend_id) ペア返却

#### S7-2: BlockDev（Worker側）
- [ ] internal/blockdev/manager.go: Manager インターフェース定義
- [ ] Attach/Detach 実装（ExportInfoのprotocolに応じた処理）
- [ ] cirrus-simではprotocol="rbd"のスタブ接続

#### S7-3: Hypervisor VM操作
- [ ] internal/hypervisor/libvirt/libvirt.go: DefineVM, StartVM, StopVM, DestroyVM, UndefineVM
- [ ] domain XML生成（テンプレートベース）: ディスク、ポート（interfaceid）、cloud-init
- [ ] cloud-init ISO生成（network-config, meta-data, user-data）

#### S7-4: gRPC: CreateVM
- [ ] proto/agent.proto: CreateVM RPC追加（CreateVMRequest/Response）
- [ ] DiskSpec（ExportInfo含む）、PortSpec メッセージ定義
- [ ] worker側: CreateVM → BlockDev.Attach → Hypervisor.DefineVM → Hypervisor.StartVM

#### S7-5: Compute Orchestrator
- [ ] internal/compute/service.go: Service インターフェース定義
- [ ] internal/compute/orchestrator.go: CreateVM実装
  - Quota.Check（Sprint 10で実装、ここではスタブ）
  - Network.CreatePort（IP/MAC払い出し + Group割り当て）
  - Storage.CreateVolume（バックエンドドライバ経由）
  - Scheduler.Schedule（ホスト+バックエンド選定）
  - Storage.ExportVolume（ホストへのエクスポート）
  - Agent.CreateVM（gRPC → worker）
  - Network.BindPort（ホストにバインド）
- [ ] 非同期ジョブ実行（goroutine + channel）
- [ ] DB: vms テーブルのステータス遷移管理

#### S7-6: APIエンドポイント + テスト
- [ ] POST /api/v1/vms（202 Accepted）: flavor_id, az(optional), network_id, volume_type_id 指定
- [ ] GET /api/v1/vms, GET /api/v1/vms/{id}
- [ ] マイグレーション: vms, vm_volumes テーブル
- [ ] 結合テスト: VM作成→スケジューラ→worker→cirrus-simでドメイン起動確認

#### S7-7: CLIクライアント
- [ ] cirrusctl vm create/list/show コマンド追加

**デプロイ確認**: VM作成API → スケジューリング → storage-simにボリューム + libvirt-simにドメイン

---

### Sprint 8: VMライフサイクル

**ゴール**: VMの起動・停止・再起動・削除が動作し、ステータス遷移が正しく管理される。

#### S8-1: VM操作 gRPC
- [ ] proto/agent.proto: DeleteVM, StartVM, StopVM, RebootVM, GetVMState RPC追加
- [ ] worker側: 各操作のHypervisor委譲実装

#### S8-2: Compute Orchestrator 操作
- [ ] DeleteVM: Hypervisor.DestroyVM → Hypervisor.UndefineVM → BlockDev.Detach → Storage.UnexportVolume → Storage.DeleteVolume → Network.DeletePort → DB更新
- [ ] StartVM / StopVM / RebootVM: gRPC経由でworkerに指示
- [ ] 削除時のリソース解放順序の保証

#### S8-3: ステータス管理
- [ ] VMステータス遷移の厳密な状態機械実装
- [ ] エラー時のステータス遷移（building→error等）
- [ ] 非同期ジョブ失敗時のクリーンアップ（作成途中のリソース削除）

#### S8-4: テスト
- [ ] 結合テスト: 作成→停止→起動→再起動→削除の全ライフサイクル
- [ ] 異常系: 作成途中のworker障害でエラーステータスに遷移
- [ ] cirrus-sim障害注入: libvirt-simのDomainCreate失敗

#### S8-5: CLIクライアント
- [ ] cirrusctl vm start/stop/reboot/delete コマンド追加

**デプロイ確認**: VM作成→停止→起動→削除の一連操作が正常に完了

---

### Sprint 8.5a: ホスト状態遷移制約 + Heartbeat監視

**ゴール**: ホストの operational_state 遷移に制約を適用し、heartbeat途絶でfaulty自動遷移、draining完了で自動maintenance遷移、faulty時のカスケード状態更新が動作する。

#### S8.5a-1: 状態遷移制約
- [ ] internal/host/store.go: SetOperationalState に遷移ルール適用（docs/host.md の遷移表に準拠）
- [ ] retiring は終端状態 — activate 不可
- [ ] active→maintenance は稼働VM数=0の場合のみ許可（VMがある場合は先にdrain）
- [ ] maintenance→retiring のみ retiring への遷移を許可
- [ ] 不正な遷移は 409 Conflict で拒否

#### S8.5a-2: Heartbeat監視 + faulty自動遷移
- [ ] internal/controller/heartbeat_monitor.go: 定期的にlast_heartbeatを監視
- [ ] 3回連続タイムアウト（デフォルト30秒無応答）でactive/draining→faulty に自動遷移
- [ ] heartbeat-fail-countはインメモリ。controller再起動時はリセットされ、最初のheartbeat受信後にカウント開始

#### S8.5a-3: HostFaultyHandler（カスケード障害処理）
- [ ] internal/controller/host_faulty_handler.go: faulty遷移時のカスケード状態更新
- [ ] faulty遷移直後に実行: ホスト上の全VMをerrorに、関連ポートをdownに更新
- [ ] ログ出力（将来HA failoverトリガーの接続点）

#### S8.5a-4: Draining完了の自動遷移
- [ ] draining状態のホストで稼働VM数が0になったらmaintenanceに自動遷移
- [ ] heartbeat受信時にVM数チェック → 条件成立で遷移

#### S8.5a-5: テスト
- [ ] 不正遷移の拒否テスト（retiring→activate、VM有りでactive→maintenance等）
- [ ] heartbeat途絶→faulty自動遷移テスト
- [ ] faulty遷移→HostFaultyHandler→VM/ポート状態カスケード更新テスト
- [ ] draining→VM退避完了→maintenance自動遷移テスト

**デプロイ確認**: 不正遷移が拒否される + heartbeat停止→faulty自動遷移→カスケード更新 + drain完了→maintenance自動遷移

---

### Sprint 8.5b: DriftEvent基盤 + Heartbeat Reconciler

**ゴール**: 統一的なDriftEvent基盤を構築し、heartbeat内のVM情報でCompute層のドリフトをパッシブ検出する。Sprint 5N/6で実装済みのNetwork/Storage ReconcilerをDriftEvent発火に移行する。

#### S8.5b-1: DriftEvent基盤
- [ ] internal/controller/reconcile/drift.go: DriftEvent型定義（docs/reconciliation.md 参照）
- [ ] internal/controller/reconcile/handler.go: DriftHandler実装
  - Deduplicator: resource_id + typeベースの重複排除（インメモリTTLキャッシュ）
  - Logger/AlertSink: ログ出力 + drift_eventsテーブル永続化
  - AutoHealer: auto-healアクション実行（楽観的ロック付き）
- [ ] マイグレーション: drift_events テーブル
- [ ] reconcile設定パラメータの追加（--reconcile-interval, --auto-heal-enabled 等）

#### S8.5b-2: Heartbeat Reconciler
- [ ] internal/host/models.go: ResourceReport を拡張（RunningVMs フィールド追加）
- [ ] internal/controller/grpc.go: heartbeatハンドラでRunningVMsをreconcilerに渡す
- [ ] internal/controller/reconcile/compute.go: HeartbeatReconciler実装
- [ ] heartbeat受信時: running_vms vs DB上の安定状態VMリストを比較
- [ ] 遷移中ステータス（scheduling, building, deleting, migrating等）は除外
- [ ] DB有・heartbeat無（expected_missing）→ Auto-heal: DB→error（楽観的ロック）
- [ ] DB無・heartbeat有（unexpected_present）→ Alert
- [ ] VMステータス不一致（state_mismatch）→ ユーザ操作記録の有無で分岐:
  - ユーザ操作無し + libvirt=shutoff → Auto-heal: DB→error（異常停止）
  - ユーザ操作有り → Auto-heal: DB同期
- [ ] auto-heal実行時はリソース単位のアドバイザリーロックで排他制御

#### S8.5b-3: 既存Reconciler移行
- [ ] Sprint 5NのNetwork ReconcilerをDriftEvent発火に移行
- [ ] Sprint 6のStorage ReconcilerをDriftEvent発火に移行
- [ ] DriftEvent対応判定テーブル（reconciliation.md）に基づきAlert/Auto-heal振り分け

#### S8.5b-4: テスト
- [ ] DriftEvent永続化テスト（drift_eventsテーブルに記録されること）
- [ ] HeartbeatReconciler: VM消失時にDB→errorに更新（楽観的ロック動作含む）
- [ ] HeartbeatReconciler: 遷移中VMは無視されること
- [ ] HeartbeatReconciler: 未知VM検出時にAlertが発火すること
- [ ] HeartbeatReconciler: ユーザ操作中のVMへのauto-healが排他制御で抑制されること
- [ ] 重複DriftEventが抑制されること

**デプロイ確認**: VM状態不整合でDriftEvent発火 + drift_eventsテーブルに記録 + 既存reconcilerがDriftEvent経由で動作

---

### Sprint 10: Quota

**ゴール**: 階層化クォータ（組織→テナント）が機能し、超過時にリソース作成が拒否される。

#### S10-1: Quota Service
- [ ] internal/quota/service.go: Service インターフェース定義
- [ ] internal/quota/manager.go: Check, Reserve, Commit, Release
- [ ] 組織クォータとテナントクォータの両方を検査
- [ ] 予約パターン: 作成開始時にReserve → 成功時Commit / 失敗時Release

#### S10-2: クォータ対象リソース
- [ ] vCPU, メモリ, VM数, ボリューム数, ボリューム容量, スナップショット数
- [ ] ネットワーク数, egress数, ingress数
- [ ] リソースごとの使用量集計クエリ

#### S10-3: Compute/Storage/Network統合
- [ ] Compute.CreateVM にQuota.Check + Reserve/Commit/Release 組み込み
- [ ] Storage.CreateVolume にクォータチェック組み込み
- [ ] Network.CreateNetwork にクォータチェック組み込み
- [ ] Sprint 7で入れたスタブを本実装に差し替え

#### S10-4: APIエンドポイント + テスト
- [ ] PUT /api/v1/tenants/{id}/quota（クォータ設定）
- [ ] GET /api/v1/tenants/{id}/quota（クォータ使用状況）
- [ ] 結合テスト: クォータ上限設定→超過するVM作成→403拒否

#### S10-5: CLIクライアント
- [ ] cirrusctl quota show/set コマンド追加

**デプロイ確認**: クォータ設定→リソース作成→上限到達で拒否される

---

### Sprint 11: Egress + Ingress + ゲートウェイ

**ゴール**: テナントNetworkからの外部接続（Egress）と外部からの着信（Ingress）がゲートウェイノード経由で動作する。re-arch.mdのVPCモデルに準拠。

#### S11-1: ゲートウェイノード管理
- [ ] マイグレーション: gateway_nodes テーブル（Sprint 5N-2で作成済みなら流用）
- [ ] 管理者API: POST/GET/DELETE /gateway-nodes（GWノード登録: host_id, external_ip, internal_ip）
- [ ] GWノードのステータス管理（active, draining, retired）
- [ ] Network単位でGWノードペア（Active-Standby）を割り当て

#### S11-2: Egress（外部向け通信）
- [ ] テナントAPI: POST/GET/DELETE /tenants/{tid}/networks/{nid}/egresses
- [ ] Egressタイプ:
  - NAT Gateway: GWノードでSNAT（パブリックIP共有、ポート範囲分割）
  - VPN: GWノードにIPsec/WireGuardトンネル終端（CIDR指定必須）
  - Direct Connect: Border Router経由の専用線接続（VLAN trunk）
- [ ] HostNetworkStateにEgressルールを含めて配信
- [ ] エージェント側: Egress宛フローをGWノードへ転送

#### S11-3: Ingress（外部からの着信）
- [ ] テナントAPI: POST/GET/DELETE /tenants/{tid}/networks/{nid}/ingresses
- [ ] Ingressタイプ:
  - Direct IP: パブリックIPを特定VMに1対1 DNAT
  - L4 Load Balancer: パブリックIPでトラフィック受信→Group内VM群にconntrack + DNATで分散
- [ ] 管理者API: /ip-pools（パブリックIPプール管理）
- [ ] GWノード上でDNAT実行
- [ ] L4 LB: conntrackセッションアフィニティ、コントローラ主導ヘルスチェック

#### S11-4: 内部LB
- [ ] テナントAPI: POST/GET/DELETE /tenants/{tid}/networks/{nid}/load-balancers
- [ ] GroupにVIP割り当て、各ホストのOVSで分散実行（GWノード不経由）
- [ ] DNS統合: VIP名（api-lb.my-app.internal）→OVSがGroup内VMにDNAT分散

#### S11-5: データプレーン統合
- [ ] GWノードのエージェント: Egress SNAT / Ingress DNATフロー管理
- [ ] NAT Gateway: ホストごとのポート範囲分割によるパブリックIP共有
- [ ] VPN: GWノードでのトンネル終端フロー
- [ ] Direct Connect: VLAN trunkフロー
- [ ] L4 LB: GWノード上のラウンドロビンDNATフロー + ヘルスチェック連動

#### S11-6: Reconciler拡張（docs/reconciliation.md 参照）
- [ ] Gateway/Egress/Ingressのデータプレーン照合 → DriftEvent発火
- [ ] Egress断（SNAT消失）→ Alert（critical）
- [ ] Ingress断（DNAT消失）→ Alert（critical）
- [ ] 未知ルール → Alert（high）

#### S11-7: テスト
- [ ] Egress: NAT Gateway経由のインターネット接続テスト
- [ ] Egress: VPN接続テスト（CIDR指定）
- [ ] Ingress: Direct IP → VM到達テスト
- [ ] Ingress: L4 LB → Group内VM分散テスト
- [ ] 内部LB: VIP経由のVM間分散テスト
- [ ] Reconciler: DNAT/SNAT消失検出テスト

#### S11-8: CLIクライアント
- [ ] cirrusctl egress/ingress/load-balancer コマンド追加
- [ ] cirrusctl admin gateway-node コマンド追加

**デプロイ確認**: GWノード登録→Egress(NAT GW)有効化→Ingress(Direct IP + L4 LB)作成→内部LB作成→全データプレーンで動作確認

---

### Sprint 12: Phase 1 安定化

**ゴール**: Phase 1全機能の結合テストが通り、安定してデプロイできる状態。

#### S12-1: E2Eテストスイート
- [ ] テナント作成からVM削除までのフルフローE2Eテスト
- [ ] マルチテナントシナリオ（テナントA/Bの隔離確認）
- [ ] Policy/Groupによるアクセス制御の確認
- [ ] Gateway/Egress/Ingressの外部接続確認
- [ ] cirrus-sim medium環境でのテスト

#### S12-2: エラーハンドリング強化
- [ ] 全APIエンドポイントのバリデーション強化
- [ ] 非同期ジョブの失敗時クリーンアップ網羅
- [ ] 部分的に作成されたリソースの整合性修復

#### S12-3: API仕上げ
- [ ] ページネーション（全リスト系API）
- [ ] フィルタリング（status, name等）
- [ ] ソート

#### S12-4: Reconcile結合テスト
- [ ] Reconcile E2Eテスト: cirrus-sim上でVM状態不整合を注入 → HeartbeatReconcilerがDriftEvent発火 → Auto-heal動作確認
- [ ] Reconcile E2Eテスト: OVSフロー手動削除 → Network Reconciler検出 → Alert確認
- [ ] Reconcile E2Eテスト: Storage上のボリュームを手動削除 → StorageReconcilerがDriftEvent発火 → Alert確認
- [ ] Reconcile E2Eテスト: heartbeat停止 → faulty遷移 → HostFaultyHandler → VM/ポートカスケード更新
- [ ] 各Reconciler（S5N-11, S6-5, S8.5b, S11-6）の統合動作確認
- [ ] drift_eventsテーブルに全イベントが記録されていること

**デプロイ確認**: cirrus-sim medium環境でE2Eテストスイートが全件パス + Reconcile異常注入テストがパス

---

## Phase 2: 運用機能（Sprint 13〜19 + CLI）

ゴール: ライブマイグレーション、DRS、ホストプロファイル管理、スナップショット、テンプレートが動作する。

---

### Sprint 13: ライブマイグレーション

**ゴール**: 同一コンピュートプール内でVMをライブマイグレーションできる。Fallbackパターンによるゼロパケットロス移行。

#### S13-1: Hypervisor Migration
- [ ] internal/hypervisor/libvirt/libvirt.go: MigrateVM実装
- [ ] DomainMigratePerform3Params / Prepare3Params / Finish3Params / Confirm3Params
- [ ] マイグレーション速度設定（MigrateGetMaxSpeed / MigrateSetMaxSpeed）

#### S13-2: gRPC Migration
- [ ] proto/agent.proto: PrepareMigration, StartMigration RPC
- [ ] worker側: migration準備→実行→完了の3フェーズ

#### S13-3: Scheduler Reschedule
- [ ] internal/scheduler/scheduler.go: Reschedule実装
- [ ] 同一コンピュートプール内での移行先候補選定
- [ ] 移行元ホストの除外

#### S13-4: Compute Orchestrator Migration
- [ ] MigrateVM: Reschedule → PrepareMigration → StartMigration → Network.BindPort更新 → DB更新
- [ ] VMステータス: active → migrating → active
- [ ] 失敗時のFallback: 元ホストで継続稼働を確認し、ステータスをactiveに戻す

#### S13-5: Fallbackパターン（re-arch.md準拠）
- [ ] 移動先ホストにフロー+ポート準備
- [ ] 移動元ホストにFallback転送設定（移動先へのGeneve転送）
- [ ] 他ホストのトンネル宛先更新 + ACK管理
- [ ] 全ACK受信後にFallback削除
- [ ] タイムアウト（30秒）後の未応答ホストはログ記録してFallback削除
- [ ] portsテーブル状態遷移: active → migrating → switching → draining → active
  - migrating: Phase 1（メモリコピー）開始
  - switching: フロー切替中
  - draining: Fallback残存、ACK待ち
  - active: 全ACK受信、Fallback削除完了

#### S13-6: APIエンドポイント + テスト
- [ ] POST /api/v1/vms/{id}/actions (action=migrate)
- [ ] 結合テスト: VM作成→マイグレーション→libvirt-simで移行先に存在確認
- [ ] cirrus-sim障害注入: マイグレーション中の失敗→Fallbackで元ホスト継続
- [ ] Fallbackパターンのパケットロスゼロ検証

#### S13-7: CLIクライアント
- [ ] cirrusctl vm migrate コマンド追加

**デプロイ確認**: VM作成→別ホストへマイグレーション→Fallbackパターンでゼロパケットロス移行→移行先で稼働継続

---

### Sprint 13.5: HA Failover（docs/reconciliation.md 参照）

**ゴール**: ホスト障害検出時にフェンシングを行い、影響VMを別ホストで自動再起動する。

#### S13.5-1: FencingAgent
- [ ] internal/controller/fencing/agent.go: FencingAgent インターフェース定義
- [ ] hook経由（AWX）でIPMI power-off実行
- [ ] 電源OFF確認のポーリング
- [ ] フェンシングタイムアウト + 失敗時のAlert(critical)

#### S13.5-2: FailoverTrigger
- [ ] internal/controller/reconcile/failover.go: FailoverTrigger実装
- [ ] DriftHandler にFailoverTriggerを接続
- [ ] faulty遷移 → FencingAgent → フェンシング成功 → Scheduler.Reschedule → VM再起動
- [ ] フェンシング失敗時: failover中止、管理者通知（VMはerror状態のまま）

#### S13.5-3: VM再起動フロー
- [ ] error状態のVMをReschedule（Sprint 13のインフラ使用）
- [ ] 新ホスト選定 → Storage.ReexportVolume → Agent.CreateVM → Network.RebindPort
- [ ] VMステータス: error → scheduling → building → active

#### S13.5-4: テスト
- [ ] cirrus-sim障害注入: worker停止 → faulty → フェンシング → failover → 別ホストでVM再起動
- [ ] フェンシング失敗シナリオ: failoverが中止されAlertが発火すること
- [ ] 複数VM同時failover: ホスト上の全VMが順次別ホストに再配置されること

**デプロイ確認**: worker停止→faulty検出→フェンシング→別ホストでVM復旧

---

### Sprint 14: DRS

**ゴール**: コンピュートプール内のリソース偏りを検出し、自動で再配分する。

#### S14-1: DRSポリシー
- [ ] DRSポリシー定義: 閾値（CPU/メモリ使用率の標準偏差）、実行間隔、最大同時マイグレーション数
- [ ] コンピュートプールへのDRSポリシー関連付け

#### S14-2: DRSエンジン
- [ ] internal/scheduler/drs.go: リソース偏りの検出
- [ ] マイグレーション計画の生成: どのVMをどこに動かすか
- [ ] 計画の評価: マイグレーション後のリソース分布シミュレーション
- [ ] 安定条件: 改善幅が閾値以下なら実行しない

#### S14-3: DRS実行
- [ ] 定期実行（configurable interval）
- [ ] マイグレーション計画を順次実行（Sprint 13のMigrateVM使用）
- [ ] 同時マイグレーション数の制限
- [ ] 実行ログと結果記録

#### S14-4: テスト
- [ ] cirrus-sim: ホスト間でリソース偏りを作為的に発生させる
- [ ] DRS実行後にリソース分布が改善されることを確認
- [ ] DRS中のVM可用性確認

**デプロイ確認**: リソース偏りのあるクラスタ→DRS発動→リソース分布改善

---

### Sprint 15: ホストプロファイル + Hook

**ゴール**: ホストプロファイルを定義し、AWX hookでホストに適用できる。ロールアウトが動作する。

#### S15-1: プロファイルモデルのDB
- [ ] マイグレーション: host_profiles テーブル
- [ ] hosts.profile_id, hosts.profile_status カラム

#### S15-2: Hook Executor
- [ ] internal/hook/executor.go: Executor インターフェース定義
- [ ] internal/hook/awx/awx.go: AWX REST API実装
  - ジョブテンプレート実行
  - ポーリングによる完了待ち
  - パラメータマッピング
- [ ] cirrus-sim awx-simへの接続テスト

#### S15-3: Profile Service
- [ ] internal/host/profile.go: CreateProfile, ApplyProfile
- [ ] ApplyProfile → Hook.Execute → プロファイル適用ジョブ実行
- [ ] 適用結果に応じてprofile_status更新（in_sync / drifted）

#### S15-4: Rollout
- [ ] internal/host/rollout.go: StartRollout実装
- [ ] フォルトドメイン単位のカナリアデプロイ
- [ ] batch_size, pause_between_batches
- [ ] ロールバック条件（ヘルスチェック失敗率）

#### S15-5: APIエンドポイント + テスト
- [ ] POST/GET /api/v1/host-profiles
- [ ] POST /api/v1/host-profiles/{id}/rollout
- [ ] PUT /api/v1/hosts/{id}（プロファイル適用）
- [ ] 結合テスト: プロファイル作成→ロールアウト→awx-simでジョブ実行確認

#### S15-6: CLIクライアント
- [ ] cirrusctl host-profile/rollout コマンド追加

**デプロイ確認**: プロファイル定義→ロールアウト開始→awx-simでジョブ順次実行→全ホストin_sync

---

### Sprint 16: スナップショット + クローン

**ゴール**: ボリュームのスナップショット取得、スナップショットからのクローン作成、依存関係管理が動作する。

#### S16-1: スナップショットモデルのDB
- [ ] マイグレーション: snapshots テーブル
- [ ] volumes.parent_snapshot_id（クローン元参照）

#### S16-2: Storage Service拡張
- [ ] CreateSnapshot → BackendDriver.CreateSnapshot + DB
- [ ] DeleteSnapshot → 依存関係チェック（子クローンあれば拒否）→ BackendDriver.DeleteSnapshot
- [ ] CloneFromSnapshot → BackendDriver.CloneSnapshot + DB（新ボリューム作成）

#### S16-3: 依存関係グラフ
- [ ] ボリューム→スナップショット→クローンの親子関係管理
- [ ] 削除制約の実装: 子が存在するスナップショットは削除不可
- [ ] フラット化操作（非同期）: 子をフルコピーに変換して依存を切る

#### S16-4: APIエンドポイント + テスト
- [ ] POST/GET/DELETE /api/v1/volumes/{id}/snapshots
- [ ] POST /api/v1/snapshots/{id}/clone
- [ ] 結合テスト: ボリューム→スナップショット→クローン→スナップショット削除拒否→フラット化→削除成功

#### S16-5: CLIクライアント
- [ ] cirrusctl snapshot/clone コマンド追加

**デプロイ確認**: スナップショット→クローン→依存関係による削除制約が正しく動作

---

### Sprint 17: ストレージドレイン + マイグレーション

**ゴール**: ストレージバックエンドのライフサイクル管理とボリュームのライブマイグレーションが動作する。

#### S17-1: バックエンドライフサイクル
- [ ] ステータス遷移実装: active → degraded → draining → readonly → retired
- [ ] 縮退: 新規ボリューム配置の優先度低下（スケジューラ統合）
- [ ] ドレイン: 新規ボリューム作成の完全停止

#### S17-2: ストレージライブマイグレーション
- [ ] BackendDriver.MigrateVolume（同種バックエンド間）
- [ ] 汎用ブロックコピー（異種バックエンド間、ホスト経由）
- [ ] マイグレーション中のVM継続稼働

#### S17-3: ドレインオーケストレーション
- [ ] 依存関係を考慮した移行順序算出
- [ ] 帯域制限
- [ ] 進捗追跡（残りボリューム数、データ量、推定完了時間）

#### S17-4: テスト
- [ ] 結合テスト: バックエンドドレイン開始→ボリューム順次移行→完了→退役
- [ ] スナップショット/クローンを持つボリュームの移行順序
- [ ] 移行中のVM I/O継続確認

**デプロイ確認**: バックエンドドレイン→ボリューム移行→全ボリューム移行完了→退役

---

### Sprint 18: テンプレートサービス

**ゴール**: テンプレートの登録・公開・キャッシュコピーが動作し、VM作成時のテンプレート選択が機能する。

#### S18-1: テンプレートモデルのDB
- [ ] マイグレーション: templates, template_caches テーブル

#### S18-2: Template Service
- [ ] internal/template/service.go: Service インターフェース定義
- [ ] Create: ボリュームからテンプレート作成
- [ ] EnsureCached: 指定バックエンドにキャッシュがなければバックグラウンドコピー
- [ ] 公開範囲管理: public, organization, tenant

#### S18-3: キャッシュLRU管理
- [ ] last_used_at更新（VM作成時）
- [ ] LRU eviction: 使われていないキャッシュの自動削除
- [ ] 容量閾値ベースのeviction

#### S18-4: VM作成フロー統合
- [ ] Compute.CreateVM: テンプレート指定時にTemplate.EnsureCached呼び出し
- [ ] キャッシュ完了待ち→ボリュームクローン

#### S18-5: APIエンドポイント + テスト
- [ ] POST/GET/DELETE/PUT /api/v1/templates
- [ ] 結合テスト: テンプレート作成→別バックエンドでVM作成→キャッシュコピー発生→VM起動

#### S18-6: CLIクライアント
- [ ] cirrusctl template コマンド追加

**デプロイ確認**: テンプレート登録→キャッシュのないバックエンドでVM作成→透過的にキャッシュ→VM起動

---

### Sprint 19: 監視・メトリクス + Phase 2 安定化

**ゴール**: Prometheusメトリクス、ヘルスチェック、宣言的トポロジの乖離検出が動作する。Phase 2全機能が安定。

#### S19-1: メトリクス
- [ ] Prometheus exporter（/metrics エンドポイント）
- [ ] ホスト: リソース使用率、VM数、稼働状態
- [ ] ストレージ: バックエンド容量使用率、IOPS
- [ ] ネットワーク: ポート数、Policy/Group数
- [ ] スケジューラ: 配置レイテンシ、スケジューリング成功/失敗率

#### S19-2: ヘルスチェック・乖離検出
- [ ] 宣言されたトポロジと実態の乖離検出
  - ストレージ到達性: バックエンドに到達できるか
  - ネットワーク到達性: データプレーンに接続できるか
  - ホストCapability: 宣言と実態の差異
- [ ] 乖離アラート（障害アラート + セキュリティ・整合性アラート）

#### S19-3: Phase 2 E2Eテスト
- [ ] ライブマイグレーション + DRS + ストレージドレインの複合シナリオ
- [ ] ホストプロファイルロールアウト中のVM可用性
- [ ] cirrus-sim medium環境での全機能テスト
- [ ] 障害注入テスト強化

**デプロイ確認**: /metricsでPrometheusスクレイプ可能 + Phase 2全機能E2Eパス

---

### Sprint 19.5: Controller HA（docs/controller-ha.md 参照）

**ゴール**: Controllerを複数インスタンスActive/Active構成で運用でき、1台停止しても自動でリーダー切り替えが行われる。

#### S19.5-1: リーダー選出
- [ ] internal/controller/leader.go: PostgreSQLアドバイザリーロックによるリーダー選出
- [ ] `pg_try_advisory_lock` で排他。セッション切断で自動解放
- [ ] リーダー選出の試行間隔（デフォルト5秒）
- [ ] リーダー状態の公開（/healthzにリーダーフラグ追加）

#### S19.5-2: シングルトンジョブのリーダー限定実行
- [ ] HeartbeatMonitor: リーダーのみ起動/停止
- [ ] ReconcileLoop（Network/Storage）: リーダーのみ起動/停止
- [ ] HostFaultyHandler: リーダーのみ実行
- [ ] DRS: リーダーのみ実行
- [ ] リーダー喪失時のジョブ停止 + 再取得時の再開

#### S19.5-3: Scheduler分散ロック
- [ ] 配置決定時にホストリソースを `SELECT FOR UPDATE` で排他
- [ ] 複数controllerが同時にスケジュールしてもリソース競合しない

#### S19.5-4: Worker接続のHA対応
- [ ] ロードバランサ経由の接続（L4 LB）を前提とした設計
- [ ] gRPC接続断時のリトライロジック（agent側）

#### S19.5-5: PostgreSQL接続障害対応
- [ ] フェイルオーバー時の一時503レスポンス
- [ ] pgxpool自動リコネクトの動作確認
- [ ] マイグレーション排他（golang-migrateのロック機能）

#### S19.5-6: テスト
- [ ] 2台構成でリーダー選出が動作すること
- [ ] リーダー停止→別インスタンスが引き継ぐこと
- [ ] 同時スケジューリングでリソース競合しないこと
- [ ] DB接続断→復帰後に正常動作すること

**デプロイ確認**: 2台構成で1台停止→リーダー切り替え→API/gRPC継続動作

---

## Phase 3: 高度なネットワーク・ストレージ（Sprint 20〜24 + CLI）

ゴール: Service Insertion、ゲートウェイHA、複数ストレージドメイン、テナント間サービス公開が動作する。

---

### Sprint 20: Service Insertion（トラフィック経路挿入）

**ゴール**: テナントNetworkのトラフィック経路にサービスVM（FW、IDS等）を挿入できる。Service Insertion用VMはservice_in / service_outの2ポートを持つ。

#### S20-1: Service InsertionモデルのDB
- [ ] マイグレーション: service_insertions テーブル（Sprint 5N-2で作成済みなら流用）
- [ ] intercept_rules: 対象Group間通信の指定（src_group, dst_group）
- [ ] service VMのポート管理: service_in / service_out の2ポート

#### S20-2: Service Insertion管理
- [ ] internal/network/service_insertion/service.go: Service インターフェース定義
- [ ] テナントAPI: POST/GET/DELETE /tenants/{tid}/networks/{nid}/service-insertions
- [ ] CreateServiceInsertion: サービスVM指定 + intercept対象Group間通信定義
- [ ] service_in / service_out ポートの自動作成（role=service_in, role=service_out）

#### S20-3: トラフィックステアリング
- [ ] OpenFlowパイプラインにService Insertion分岐を追加
- [ ] 通常フロー: web VM → OVS → api VM
- [ ] 挿入後: web VM → OVS → FW VM(service_in) → 検査 → FW VM(service_out) → OVS → api VM
- [ ] HostNetworkStateにService Insertionルールを含めて配信
- [ ] ヘルスチェック: サービスVMの死活監視 + 障害時のバイパス

#### S20-4: テスト
- [ ] Service Insertion作成→対象トラフィックがサービスVM経由で転送されること
- [ ] サービスVM障害→バイパス動作確認
- [ ] cirrus-sim環境でのE2Eテスト

**デプロイ確認**: FWサービス挿入→トラフィックがFW VM（service_in→service_out）経由で転送される

---

### Sprint 21: ゲートウェイHA + スケールアウト

**ゴール**: ゲートウェイノードの高可用性とスケールアウトが動作する。re-arch.mdのActive-Standby + BFDモデルに準拠。

#### S21-1: ゲートウェイHA（Active-Standby + BFD）
- [ ] Network単位でGWノードペア（Active-Standby）を割り当て
- [ ] BFDによる死活監視
- [ ] 自動フェイルオーバー: Active障害 → Standby昇格
- [ ] conntrack同期は初期実装では行わず、GW切替時の既存セッション切断を許容
- [ ] フェイルオーバー時のEgress/Ingress継続

#### S21-2: GWノードの無停止移動（Fallbackパターン）
- [ ] 新GWにフロー準備、外部IPを付与
- [ ] 各ホストのフローを新GW向けに切り替え（ACK待ち）
- [ ] 旧GWにdrainフロー設定（ct_state=+estのみ処理、+newは新GWに転送）
- [ ] 既存セッションの自然タイムアウト待ち
- [ ] drain完了後、旧GWからフロー削除
- [ ] Ingress側の切り替えはGratuitous ARPで完結

#### S21-3: ゲートウェイスケールアウト
- [ ] Network数増加に応じたGWペア追加
- [ ] 各ホストのエージェントは「このNetworkの外部通信はどのGWに送るか」をフローで管理
- [ ] GWペアの再配分（Network割り当ての変更）

#### S21-4: テスト
- [ ] ゲートウェイ障害→BFD検出→フェイルオーバー→Egress/Ingress継続
- [ ] GWノード無停止移動→drainフロー→既存セッション維持
- [ ] スケールアウト: GWペア追加→Network割り当て→トラフィック分散確認
- [ ] cirrus-sim環境でのE2Eテスト

**デプロイ確認**: ゲートウェイHA構成→障害注入→BFDフェイルオーバー + 無停止移動→drain完了 + スケールアウト→負荷分散

---

### Sprint 22: 複数ストレージドメイン + レプリケーション

**ゴール**: 複数ストレージドメインとリージョン間レプリケーションが動作する。

#### S22-1: マルチストレージドメイン
- [ ] 複数ストレージドメインの管理
- [ ] バックエンドのドメイン所属管理
- [ ] スケジューラ: ドメインを考慮したバックエンド選定

#### S22-2: レプリケーション
- [ ] マイグレーション: replication_policies テーブル
- [ ] レプリケーションポリシー定義（対象、宛先、頻度、保持世代数）
- [ ] バックエンドの差分転送capability判定
- [ ] 定期レプリケーション実行

#### S22-3: テスト
- [ ] cirrus-sim medium環境（2ストレージバックエンド: SSD/HDD）
- [ ] レプリケーションポリシー設定→定期実行→レプリカ確認
- [ ] 差分転送 vs フルコピーの切り替え確認

**デプロイ確認**: リージョン間レプリケーション設定→定期実行→レプリカ作成

---

### Sprint 23: Service Endpoint（テナント間サービス公開）

**ゴール**: テナントが自身のサービスを他テナントに公開し、消費側テナントがDNS名で安全に接続できる。双方向NATによるIPアドレス空間の完全隔離。

#### S23-1: Service EndpointモデルのDB
- [ ] マイグレーション: service_endpoints, endpoint_connections テーブル
- [ ] service_endpoints: 公開側テナントのサービス定義（ターゲットGroup/VM、プロトコル、ポート）
- [ ] endpoint_connections: 消費側テナントの接続（承認フロー付き）
- [ ] CIDRプール: 100.127.0.0/16（Service Endpoint用VIP割当）

#### S23-2: サービス公開
- [ ] internal/network/endpoint/service.go: Service インターフェース定義
- [ ] テナントAPI: POST/GET/DELETE /tenants/{tid}/networks/{nid}/service-endpoints
- [ ] CreateServiceEndpoint: サービスを公開（ターゲットGroup/VM、承認ポリシー）
- [ ] 公開範囲: 全テナント / 指定テナントのみ

#### S23-3: サービス消費
- [ ] テナントAPI: POST/GET/DELETE /tenants/{tid}/networks/{nid}/service-connections
- [ ] CreateEndpointConnection: 消費側テナントが接続をリクエスト
- [ ] 承認フロー: 自動承認 or 公開側テナントの手動承認
- [ ] 双方向NAT（DNAT + SNAT）: テナント間のIPアドレス空間を完全隔離したまま通信
- [ ] DNS統合: my-api.tenant-b.service.internal → 100.127.x.x（VIP）

#### S23-4: テスト
- [ ] テナントA: サービス公開 → テナントB: 接続リクエスト → 承認 → DNS名で通信確認
- [ ] 双方向NAT: CIDRが重複するテナント間でも通信可能なこと
- [ ] 承認拒否テスト
- [ ] サービスエンドポイント削除→消費側接続の自動クリーンアップ
- [ ] cirrus-sim medium環境でのE2Eテスト

**デプロイ確認**: テナント間Service Endpoint接続→DNS名でのサービスアクセス→双方向NATによるIP隔離

---

### Sprint 24: Phase 3 安定化

**ゴール**: Phase 3全機能の結合テストが通り、安定してデプロイできる状態。

#### S24-1: 外部IPAM連携
- [ ] IPAM インターフェースの外部実装（NetBox, Infoblox）
- [ ] 内蔵IPAM → 外部IPAMの切り替え設定

#### S24-2: NetBoxトポロジ同期
- [ ] internal/hook/netbox/: NetBox REST API同期アダプタ
- [ ] 定期同期: NetBoxのサイト/ラック/デバイス → Cirrusロケーションツリー
- [ ] cirrus-sim netbox-simとの接続テスト

#### S24-3: Phase 3 E2Eテスト
- [ ] Service Insertion + ゲートウェイHA + レプリケーション + Service Endpointの複合シナリオ
- [ ] cirrus-sim medium/large環境でのテスト

**デプロイ確認**: Phase 3全機能がcirrus-sim medium環境でE2Eパス

---

## Phase 4: 追加機能（Sprint 25〜29 + CLI）

ゴール: ファイルストレージ、オブジェクトストレージ連携、QoS、ポリシーエンジン、CMDB同期。

---

### Sprint 25: ファイルストレージ

**ゴール**: NFS/CIFS共有ボリュームがAPIで管理できる。

- [ ] ファイルストレージバックエンドドライバ
- [ ] 共有ボリュームのACL管理
- [ ] VMからのマウント
- [ ] テスト

---

### Sprint 26: オブジェクトストレージ連携

**ゴール**: S3互換APIでオブジェクトストレージが使用でき、テンプレートのバックストアとして機能する。

- [ ] MinIO等の外部サービス連携
- [ ] Cirrus認証基盤との統合
- [ ] テンプレートのオブジェクトストレージ保存
- [ ] テスト

---

### Sprint 27: QoS + 帯域管理

**ゴール**: ボリュームQoSとネットワーク帯域管理が動作する。

- [ ] ボリュームタイプQoSポリシーの実効化（バックエンドドライバ経由）
- [ ] ネットワーク帯域制限（データプレーンQoS機能）
- [ ] noisy neighbor制御のテスト
- [ ] テスト

---

### Sprint 28: ポリシーエンジン連携

**ゴール**: OPA等の外部ポリシーエンジンで認可判定が行える。

- [ ] Authorizer インターフェースのOPA実装
- [ ] RBAC → OPAへの段階的移行パス
- [ ] リソース属性に基づく判定（ABACサポート）
- [ ] テスト

---

### Sprint 29: CMDB同期 + Phase 4 安定化

**ゴール**: 外部CMDBとの双方向同期が動作する。全Phase完了。

- [ ] NetBox双方向同期（Cirrus → NetBox のステータス反映）
- [ ] 他CMDB対応（同期アダプタインターフェースの汎用化）
- [ ] 全Phase E2Eテスト（cirrus-sim large環境）
- [ ] 負荷テスト: 2,500+ホスト環境でのスケジューラ性能
- [ ] ドキュメント最終更新

**最終デプロイ確認**: cirrus-sim large環境で全機能のE2Eテストパス

---

## cirrus-sim統合ロードマップ（cirrus-simリポジトリから移行）

以下はcirrus-simリポジトリで完了済みの機能。Sprint 5Sでcirrusリポジトリに統合する。

### 完了済み（cirrus-simリポジトリ）
- [x] libvirt-sim: libvirt RPC（接続管理、ドメインCRUD、状態遷移、ライブマイグレーション4フェーズ、イベント通知）
- [x] ovn-sim: OVSDBプロトコル（transact, monitor）、NB DB 13テーブル → **廃止**（OVS実物に置換）
- [x] storage-sim: バックエンド登録、ボリュームCRUD、エクスポート、スナップショット、クローン、マイグレーション
- [x] awx-sim: ジョブテンプレート、非同期ジョブ実行、コールバック
- [x] netbox-sim: サイト/ラック/デバイスの階層 → **Phase 3で外部IPAM/CMDB連携として対応**
- [x] common: イベントログ、障害注入エンジン、データジェネレータ、状態スナップショット
- [x] load-gen: ワークロード定義と実行エンジン
- [x] environments: small(10)/medium(400)/large(2500+)環境定義
- [x] 統一バイナリ、embedded PostgreSQL、ダッシュボードWebUI、portman連携

### 統合時の変更
- libvirtd-sim: ホスト単位に分割、VM実体をnetwork namespace + vethに変更（Sprint 5S-2）
- OVN-sim: 廃止。OVSは結合テストで実物を使用
- storage-sim: test/sim/storage/ に移行（API互換維持、Sprint 5S-1）
- awx-sim: test/sim/awx/ に移行（API互換維持、Sprint 5S-1）
- docker-compose + Dockerfile.worker: 結合テスト基盤構築（Sprint 5S-3）
- OVSモッククライアント: レイヤー2テスト用（Sprint 5S-4）
- 障害注入: 各シミュレータのハンドラでfault.Check()呼び出し統合（Sprint 5S-7）
- CLIツール: cmd/cirrus-sim-ctl/ に移行（Sprint 5S-6）

---

## スプリント依存関係

```
Sprint 1 (骨格)
├── Sprint 2 (Identity)
│   ├── Sprint 3 (Host + Worker)
│   │   ├── Sprint 4 (Topology)
│   │   │   ├── Sprint 5 (Network/OVN) [Sprint 5Nで置換]
│   │   │   │   └── Sprint 5.5 (AZ導入)
│   │   │   ├── Sprint 5S (cirrus-sim統合 + テスト基盤) ← Sprint 5.5
│   │   │   ├── Sprint 5N (VPCモデル) ← Sprint 5S, Sprint 5.5
│   │   │   └── Sprint 6 (Storage) ← Sprint 5.5にも依存
│   │   │       └── Sprint 7 (Scheduler + VM作成 + Flavor) ← Sprint 5N, Sprint 5.5
│   │   │           ├── Sprint 8 (VMライフサイクル)
│   │   │           │   ├── Sprint 8.5a (ホスト状態遷移 + Heartbeat監視)
│   │   │           │   └── Sprint 8.5b (DriftEvent + Heartbeat Reconciler)
│   │   │           └── Sprint 10 (Quota)
│   │   └── Sprint 15 (Profile + Hook)
│   ├── Sprint 11 (Egress + Ingress + ゲートウェイ) ← Sprint 5N
│   └── Sprint 12 (Phase 1 安定化) ← Sprint 8, 10, 11 全て完了後
│
├── Sprint 13 (ライブマイグレーション) ← Sprint 8
│   ├── Sprint 13.5 (HA Failover)
│   └── Sprint 14 (DRS)
├── Sprint 16 (スナップショット) ← Sprint 6
│   └── Sprint 17 (ストレージドレイン)
├── Sprint 18 (テンプレート) ← Sprint 6
├── Sprint 19 (メトリクス + Phase 2 安定化) ← Sprint 13-18 全て完了後
│   └── Sprint 19.5 (Controller HA)
│
├── Sprint 20 (Service Insertion) ← Sprint 12
│   └── Sprint 21 (ゲートウェイHA + スケールアウト) ← Sprint 11
├── Sprint 22 (マルチストレージ + レプリケーション) ← Sprint 12, 17
├── Sprint 23 (Service Endpoint) ← Sprint 12
├── Sprint 24 (Phase 3 安定化) ← Sprint 20-23 全て完了後
│
├── Sprint 25 (ファイルストレージ) ← Sprint 12
├── Sprint 26 (オブジェクトストレージ) ← Sprint 18
├── Sprint 27 (QoS) ← Sprint 12
├── Sprint 28 (ポリシーエンジン) ← Sprint 2
└── Sprint 29 (CMDB同期 + 最終安定化) ← Sprint 24-28 全て完了後
```
