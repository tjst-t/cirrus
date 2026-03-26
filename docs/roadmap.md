# 実装ロードマップ

各スプリントは独立してデプロイ・動作確認可能な単位で設計している。
cirrus-simに接続して実際のプロトコルでテストできる状態を各スプリントのゴールとする。

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
- [x] controller起動時にcirrus-sim（ovn-sim）へTCP接続確認
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

### Sprint 4: Topology（到達性ドメイン・ロケーション）

**ゴール**: ストレージ/ネットワークドメイン、ロケーションツリーが登録でき、コンピュートプールが導出される。

#### S4-1: ドメインモデルのDB
- [ ] マイグレーション: storage_domains, network_domains, locations テーブル
- [ ] locations: parent_id自己参照、type (site/floor/row/rack/unit)、fault_attributes JSONB

#### S4-2: Topology Service
- [ ] internal/topology/service.go: Service インターフェース定義
- [ ] CreateStorageDomain, CreateNetworkDomain
- [ ] ホスト⇔ストレージドメイン関連付け（host_storage_domains）
- [ ] ホスト⇔ネットワークドメイン関連付け（hosts.network_domain_id）

#### S4-3: コンピュートプール導出
- [ ] GetComputePool: ストレージドメイン ∩ ネットワークドメインのホスト集合
- [ ] ListReachableHosts(backendID): バックエンドの所属ドメインから到達可能ホスト
- [ ] ListReachableBackends(hostID): ホストの所属ドメインから到達可能バックエンド

#### S4-4: ロケーション管理
- [ ] ロケーションツリーCRUD
- [ ] WITH RECURSIVE によるパス取得、サブツリー検索
- [ ] ゾーン導出（指定階層でのグルーピング）

#### S4-5: APIエンドポイント + テスト
- [ ] POST/GET /api/v1/storage-domains
- [ ] POST/GET /api/v1/network-domains
- [ ] POST/GET /api/v1/locations
- [ ] GET /api/v1/compute-pools（導出結果）
- [ ] 結合テスト: ドメイン作成→ホスト関連付け→コンピュートプール導出確認

#### S4-6: CLIクライアント
- [ ] cirrusctl storage-domain/network-domain/location コマンド追加
- [ ] cirrusctl compute-pool list コマンド追加

**デプロイ確認**: ドメイン・ロケーション登録→コンピュートプールAPIで導出結果確認

---

### Sprint 5: Network基盤（OVN）

**ゴール**: テナントネットワーク/サブネット/ポートをAPIで作成でき、OVN（cirrus-sim ovn-sim）に反映される。

#### S5-1: OVNクライアント
- [ ] internal/network/ovn/client.go: OVNClient インターフェース定義
- [ ] internal/network/ovn/ovsdb.go: OVSDBプロトコル実装（JSON-RPC over TCP）
- [ ] CreateLogicalSwitch, CreateLogicalSwitchPort, DeleteLogicalSwitch, DeleteLogicalSwitchPort
- [ ] cirrus-sim ovn-simへの接続テスト

#### S5-2: ネットワークモデルのDB
- [ ] マイグレーション: networks, subnets, ports テーブル
- [ ] networks.network_domain_id 外部キー

#### S5-3: IPAM
- [ ] internal/network/ipam/ipam.go: IPAM インターフェース定義
- [ ] internal/network/ipam/builtin.go: DB上のCIDR演算による内蔵IPAM
- [ ] AllocateIP, ReleaseIP
- [ ] MACアドレス生成（02:xx:xx:xx:xx:xx、UNIQUE制約で衝突防止）

#### S5-4: Network Service
- [ ] internal/network/service.go: Service インターフェース定義
- [ ] CreateNetwork → DB + OVN Logical Switch作成
- [ ] CreateSubnet → DB + OVN DHCP Options作成
- [ ] CreatePort → DB + IPAM IP払い出し + OVN LSP作成
- [ ] DeleteNetwork/Subnet/Port（逆順の削除）

#### S5-5: APIエンドポイント + テスト
- [ ] POST/GET/DELETE /api/v1/networks
- [ ] POST/GET/DELETE /api/v1/networks/{id}/subnets
- [ ] POST/GET/DELETE /api/v1/ports
- [ ] 結合テスト: ネットワーク作成→ovn-simにLogical Switchが作成される

#### S5-6: CLIクライアント
- [ ] cirrusctl network/subnet/port コマンド追加

**デプロイ確認**: APIでネットワーク/サブネット/ポート作成→ovn-simの管理APIで確認

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
- [ ] POST/GET /api/v1/volume-types（インフラ管理者）
- [ ] POST/GET/DELETE /api/v1/volumes（テナント操作）
- [ ] POST /api/v1/volumes/{id}/attach, /detach

#### S6-5: テスト
- [ ] 結合テスト: バックエンド登録→ボリューム作成→storage-simに作成される
- [ ] ボリュームタイプのcapabilityマッチングテスト

#### S6-6: CLIクライアント
- [ ] cirrusctl storage-backend/volume-type/volume コマンド追加

**デプロイ確認**: バックエンド登録→ボリューム作成→storage-simの管理APIで確認

---

### Sprint 7: Scheduler + VM作成

**ゴール**: VM作成APIが動作し、スケジューラがホストとバックエンドを選定し、cirrus-sim上でVMが起動する。

#### S7-1: Scheduler
- [ ] internal/scheduler/scheduler.go: Scheduler インターフェース定義
- [ ] internal/scheduler/filter.go: フィルタリング
  - コンピュートプール（到達性フィルタ）
  - Capabilityマッチング
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
  - Network.CreatePort（IP/MAC払い出し、OVN LSP）
  - Storage.CreateVolume（バックエンドドライバ経由）
  - Scheduler.Schedule（ホスト+バックエンド選定）
  - Storage.ExportVolume（ホストへのエクスポート）
  - Agent.CreateVM（gRPC → worker）
  - Network.BindPort（OVN LSPをホストにバインド）
- [ ] 非同期ジョブ実行（goroutine + channel）
- [ ] DB: vms テーブルのステータス遷移管理

#### S7-6: APIエンドポイント + テスト
- [ ] POST /api/v1/vms（202 Accepted）
- [ ] GET /api/v1/vms, GET /api/v1/vms/{id}
- [ ] マイグレーション: vms, vm_volumes テーブル
- [ ] 結合テスト: VM作成→スケジューラ→worker→cirrus-simでドメイン起動確認

#### S7-7: CLIクライアント
- [ ] cirrusctl vm create/list/show コマンド追加

**デプロイ確認**: VM作成API → スケジューリング → ovn-simにLSP + storage-simにボリューム + libvirt-simにドメイン

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

### Sprint 9: セキュリティグループ

**ゴール**: セキュリティグループをAPIで管理でき、OVN ACLとして反映される。

#### S9-1: SGモデルのDB
- [ ] マイグレーション: security_groups, security_group_rules, port_security_groups テーブル

#### S9-2: SG → OVN ACL変換
- [ ] internal/network/manager.go: CreateSecurityGroup, AddSecurityGroupRule
- [ ] SGルール → OVN ACL変換ロジック（direction, protocol, port range, remote prefix）
- [ ] リモートグループ参照 → OVN Address Set変換
- [ ] conntrack有効化（ステートフル）

#### S9-3: デフォルトSG
- [ ] テナント作成時にデフォルトSG自動作成
- [ ] デフォルトルール: 同一グループ内ingress許可、全egress許可

#### S9-4: ポートへのSG適用
- [ ] ポート作成時にSG指定 → OVN ACL適用
- [ ] ポート更新でSG変更 → OVN ACL更新
- [ ] VM作成フローにSG統合

#### S9-5: APIエンドポイント + テスト
- [ ] POST/GET/DELETE /api/v1/security-groups
- [ ] POST/GET/DELETE /api/v1/security-groups/{id}/rules
- [ ] 結合テスト: SG作成→ルール追加→ポートに適用→ovn-simでACL確認

#### S9-6: CLIクライアント
- [ ] cirrusctl security-group コマンド追加

**デプロイ確認**: SG作成→ルール追加→VM作成時にSG指定→ovn-simでACL確認

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
- [ ] ネットワーク数, フローティングIP数
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

### Sprint 11: Router + Floating IP

**ゴール**: テナント内L3ルーティングと外部接続（フローティングIP）が動作する。

#### S11-1: ルータモデルのDB
- [ ] マイグレーション: routers, router_interfaces テーブル

#### S11-2: Router → OVN
- [ ] CreateRouter → OVN Logical Router作成
- [ ] AddRouterInterface → OVN Logical Router Port + サブネット接続
- [ ] RemoveRouterInterface → 逆順の削除
- [ ] OVNクライアントにRouter関連操作追加

#### S11-3: 外部ネットワーク
- [ ] マイグレーション: external_networks テーブル（管理者定義）
- [ ] ルータへの外部ゲートウェイ設定 → OVN Logical Router Gateway Port
- [ ] SNAT設定 → OVN NAT Rule

#### S11-4: Floating IP
- [ ] マイグレーション: floating_ips テーブル
- [ ] CreateFloatingIP → 外部ネットワークからIP確保
- [ ] AssociateFloatingIP → OVN DNAT Rule作成
- [ ] DisassociateFloatingIP → OVN DNAT Rule削除

#### S11-5: APIエンドポイント + テスト
- [ ] POST/GET/DELETE /api/v1/routers
- [ ] POST/DELETE /api/v1/routers/{id}/interfaces
- [ ] POST/GET/DELETE/PUT /api/v1/floating-ips
- [ ] 結合テスト: ルータ作成→サブネット接続→FIP作成→FIP関連付け→ovn-simで確認

#### S11-6: CLIクライアント
- [ ] cirrusctl router/floating-ip コマンド追加

**デプロイ確認**: ルータ+FIP構成→ovn-simでLogical Router + NAT Rule確認

---

### Sprint 12: Phase 1 安定化

**ゴール**: Phase 1全機能の結合テストが通り、安定してデプロイできる状態。

#### S12-1: E2Eテストスイート
- [ ] テナント作成からVM削除までのフルフローE2Eテスト
- [ ] マルチテナントシナリオ（テナントA/Bの隔離確認）
- [ ] cirrus-sim medium環境でのテスト

#### S12-2: エラーハンドリング強化
- [ ] 全APIエンドポイントのバリデーション強化
- [ ] 非同期ジョブの失敗時クリーンアップ網羅
- [ ] 部分的に作成されたリソースの整合性修復

#### S12-3: API仕上げ
- [ ] ページネーション（全リスト系API）
- [ ] フィルタリング（status, name等）
- [ ] ソート

#### S12-4: Reconcile
- [ ] worker起動時のreconcile: libvirt実態とDB状態の差分検出
- [ ] OVN状態とDB状態の差分検出
- [ ] 不整合検出時のアラート

**デプロイ確認**: cirrus-sim medium環境でE2Eテストスイートが全件パス

---

## Phase 2: 運用機能（Sprint 13〜19 + CLI）

ゴール: ライブマイグレーション、DRS、ホストプロファイル管理、スナップショット、テンプレートが動作する。

---

### Sprint 13: ライブマイグレーション

**ゴール**: 同一コンピュートプール内でVMをライブマイグレーションできる。

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
- [ ] 失敗時のロールバック（元ホストで継続稼働）

#### S13-5: APIエンドポイント + テスト
- [ ] POST /api/v1/vms/{id}/actions (action=migrate)
- [ ] 結合テスト: VM作成→マイグレーション→libvirt-simで移行先に存在確認
- [ ] cirrus-sim障害注入: マイグレーション中の失敗

#### S13-6: CLIクライアント
- [ ] cirrusctl vm migrate コマンド追加

**デプロイ確認**: VM作成→別ホストへマイグレーション→移行先で稼働継続

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
- [ ] ゾーン単位のカナリアデプロイ
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
- [ ] ネットワーク: ポート数、SG数
- [ ] スケジューラ: 配置レイテンシ、スケジューリング成功/失敗率

#### S19-2: ヘルスチェック・乖離検出
- [ ] 宣言されたトポロジと実態の乖離検出
  - ストレージ到達性: バックエンドに到達できるか
  - ネットワーク到達性: OVNクラスタに接続できるか
  - ホストCapability: 宣言と実態の差異
- [ ] 乖離アラート（障害アラート + セキュリティ・整合性アラート）

#### S19-3: Phase 2 E2Eテスト
- [ ] ライブマイグレーション + DRS + ストレージドレインの複合シナリオ
- [ ] ホストプロファイルロールアウト中のVM可用性
- [ ] cirrus-sim medium環境での全機能テスト
- [ ] 障害注入テスト強化

**デプロイ確認**: /metricsでPrometheusスクレイプ可能 + Phase 2全機能E2Eパス

---

## Phase 3: マルチドメイン（Sprint 20〜24 + CLI）

ゴール: 複数拠点（OVNクラスタ、ストレージドメイン）を跨いだ運用が可能。

---

### Sprint 20: 複数ネットワークドメイン

**ゴール**: 複数のOVNクラスタを管理し、テナントネットワークがドメインに紐づく。

#### S20-1: マルチOVNクライアント
- [ ] network_domainsテーブルの各レコードからOVN NB接続を管理
- [ ] OVNClientをドメインごとにインスタンス化
- [ ] ネットワーク作成時にnetwork_domain_id指定

#### S20-2: ドメイン間の分離
- [ ] テナントネットワークは1つのネットワークドメインに所属
- [ ] ドメインを跨いだポート作成は不可
- [ ] APIでドメイン一覧・選択可能に

#### S20-3: テスト
- [ ] cirrus-sim medium環境（2 OVNクラスタ: 東京/大阪）
- [ ] 各ドメインで独立にネットワーク/VM作成
- [ ] ドメイン間の隔離確認

**デプロイ確認**: 2つのOVNクラスタで独立にVM運用可能

---

### Sprint 21: OVN Interconnect

**ゴール**: OVN-ICによりドメインを跨いだL2延伸が動作する。

#### S21-1: OVN-IC設定
- [ ] OVN-ICクライアント実装（IC Northbound/Southbound DB）
- [ ] transit switch設定

#### S21-2: ドメイン間ネットワーク延伸
- [ ] テナントネットワークの拠点間延伸リクエスト
- [ ] OVN-IC経由のL2接続確立
- [ ] 延伸されたネットワーク上のVM間通信

#### S21-3: テスト
- [ ] cirrus-sim medium環境でドメイン間L2延伸
- [ ] 延伸ネットワーク上のVM間通信確認

**デプロイ確認**: 東京/大阪のOVNクラスタ間でL2延伸→VM間通信

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

### Sprint 23: 拡張コンピュートプール

**ゴール**: ドメインを跨いだライブマイグレーション（明示的操作）が動作する。

#### S23-1: 拡張コンピュートプール導出
- [ ] ドメイン跨ぎの到達性計算（ストレージレプリケーション + OVN-IC）
- [ ] 拡張プール内のマイグレーション可能性判定

#### S23-2: 拠点間ライブマイグレーション
- [ ] ストレージ移行（レプリカからの切り替え or フルコピー）
- [ ] コンピュートマイグレーション（libvirt）
- [ ] ネットワーク切り替え（OVN-IC経由）
- [ ] 明示的な運用操作としてのAPI（自動DRSの対象外）

#### S23-3: テスト
- [ ] cirrus-sim medium環境でドメイン間マイグレーション
- [ ] マイグレーション中のVM可用性（ダウンタイム計測）

**デプロイ確認**: 東京→大阪へのVM拠点間マイグレーション

---

### Sprint 24: 外部連携 + Phase 3 安定化

**ゴール**: ovn-bgp-agent、外部IPAM、NetBox同期が動作する。Phase 3全機能が安定。

#### S24-1: ovn-bgp-agent連携
- [ ] ovn-bgp-agent設定管理
- [ ] OVNプレフィックスのBGPアドバタイズ確認
- [ ] EVPNファブリックとの連携テスト

#### S24-2: 外部IPAM連携
- [ ] IPAM インターフェースの外部実装（NetBox, Infoblox）
- [ ] 内蔵IPAM → 外部IPAMの切り替え設定

#### S24-3: NetBoxトポロジ同期
- [ ] internal/hook/netbox/: NetBox REST API同期アダプタ
- [ ] 定期同期: NetBoxのサイト/ラック/デバイス → Cirrusロケーションツリー
- [ ] cirrus-sim netbox-simとの接続テスト

#### S24-4: Phase 3 E2Eテスト
- [ ] マルチドメイン環境での全機能テスト
- [ ] 拠点間マイグレーション + レプリケーション + OVN-ICの複合シナリオ
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
- [ ] ネットワーク帯域制限（OVN QoS機能）
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

## スプリント依存関係

```
Sprint 1 (骨格)
├── Sprint 2 (Identity)
│   ├── Sprint 3 (Host + Worker)
│   │   ├── Sprint 4 (Topology)
│   │   │   ├── Sprint 5 (Network)
│   │   │   │   ├── Sprint 9 (SG)
│   │   │   │   └── Sprint 11 (Router + FIP)
│   │   │   └── Sprint 6 (Storage)
│   │   │       └── Sprint 7 (Scheduler + VM作成) ← Sprint 5にも依存
│   │   │           ├── Sprint 8 (VMライフサイクル)
│   │   │           └── Sprint 10 (Quota)
│   │   └── Sprint 15 (Profile + Hook)
│   └── Sprint 12 (Phase 1 安定化) ← Sprint 8, 9, 10, 11 全て完了後
│
├── Sprint 13 (ライブマイグレーション) ← Sprint 8
│   └── Sprint 14 (DRS)
├── Sprint 16 (スナップショット) ← Sprint 6
│   └── Sprint 17 (ストレージドレイン)
├── Sprint 18 (テンプレート) ← Sprint 6
├── Sprint 19 (メトリクス + Phase 2 安定化) ← Sprint 13-18 全て完了後
│
├── Sprint 20 (マルチNWドメイン) ← Sprint 12
│   └── Sprint 21 (OVN-IC)
├── Sprint 22 (マルチストレージ + レプリケーション) ← Sprint 12, 17
│   └── Sprint 23 (拡張コンピュートプール) ← Sprint 21
├── Sprint 24 (外部連携 + Phase 3 安定化) ← Sprint 20-23 全て完了後
│
├── Sprint 25 (ファイルストレージ) ← Sprint 12
├── Sprint 26 (オブジェクトストレージ) ← Sprint 18
├── Sprint 27 (QoS) ← Sprint 12
├── Sprint 28 (ポリシーエンジン) ← Sprint 2
└── Sprint 29 (CMDB同期 + 最終安定化) ← Sprint 24-28 全て完了後
```
