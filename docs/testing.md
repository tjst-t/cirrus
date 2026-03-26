# 開発・テスト方針

## 基本方針

Cirrusの開発・テストには [cirrus-sim](https://github.com/tjst-t/cirrus-sim) を使用する。cirrus-simは各外部システム（libvirt、OVN、ストレージバックエンド、AWX、NetBox）と同一プロトコルで通信するシミュレータ群を提供し、物理インフラなしでフルスタックのIaaS開発・テストを可能にする。

シミュレータは本番と同じプロトコルを話すため、Cirrus側のコードにテスト用の分岐やモックは不要。接続先の設定を切り替えるだけでシミュレータ環境と本番環境を行き来できる。

## シミュレータ構成

| シミュレータ | プロトコル | Cirrusとの対応 |
|---|---|---|
| libvirt-sim | libvirt RPC (XDR/TCP) | ホストエージェント経由のVM操作 |
| ovn-sim | OVSDB (JSON-RPC/TCP) | OVN Northbound DBへのネットワーク制御 |
| storage-sim | REST API | ストレージバックエンドドライバ |
| awx-sim | AWX REST API | hook経由の物理インフラ操作 |
| netbox-sim | NetBox REST API | 障害トポロジの同期アダプタ |

## 環境規模

cirrus-simは用途に応じた環境定義を持つ:

| 環境 | ホスト数 | OVNクラスタ | ストレージ | 用途 |
|---|---|---|---|---|
| small | 10 | 1 | 1 | 日常開発、ユニットテスト |
| medium | 400 | 2 (東京/大阪) | 2 (SSD/HDD) | 結合テスト、マルチドメインテスト |
| large | 2,500+ | 5 | 4 | 負荷テスト、スケーラビリティ検証 |

## テスト戦略

### ユニットテスト

個々のパッケージのロジックテスト。外部依存はインターフェース経由で差し替え可能だが、基本的にcirrus-sim (small環境) に接続してテストする。

```bash
# cirrus-simを起動（別ターミナルまたはバックグラウンド）
cd ../cirrus-sim && make serve

# テスト実行
make test
```

テスト対象の例:
- スケジューラのcapability-basedマッチング
- クォータの階層化チェック
- スナップショット依存関係グラフの整合性検証
- 認可判定ロジック

### 結合テスト

Cirrusのコントローラーを起動し、APIを通じてリソースのライフサイクルを一通り実行する。cirrus-simのmedium環境を使用し、マルチドメイン構成もテストする。

テストシナリオの例:

1. **VM作成→停止→削除の基本フロー**
   - テナント作成 → ネットワーク作成 → サブネット作成 → セキュリティグループ作成 → VM作成
   - OVN論理スイッチポートが作成されたことを確認
   - ストレージバックエンドにボリュームが作成されたことを確認
   - libvirtにドメインが定義・起動されたことを確認

2. **スケジューラのプレースメント検証**
   - capability要件付きVMが適切なホストに配置されるか
   - アンチアフィニティルールが遵守されるか
   - ボリュームタイプ要件とバックエンド到達性が正しく評価されるか

3. **マルチテナンシーの隔離検証**
   - テナントAのリソースがテナントBから見えないこと
   - クォータ超過時にリソース作成が拒否されること
   - ロールに基づくアクセス制御が正しく機能すること

4. **ライブマイグレーション**（Phase 2）
   - 同一コンピュートプール内のVM移行
   - 移行中のネットワーク接続維持（OVNポートバインディング更新）

5. **ストレージドレイン**（Phase 2）
   - バックエンドのドレイン開始→ボリューム移行→退役
   - 依存関係を持つスナップショット/クローンの移行順序

### 障害テスト

cirrus-simの障害注入機能を使い、異常系の振る舞いを検証する。

```bash
# ホストの障害注入（DomainCreateが50%の確率で失敗）
curl -X POST http://localhost:<COMMON_PORT>/api/v1/faults \
  -d '{"target":"host-001","operation":"DomainCreate","failure_rate":0.5}'

# ホストのメンテナンスモード移行
curl -X PUT http://localhost:<SIM_PORT>/sim/hosts/host-001/state \
  -d '{"state":"maintenance"}'
```

テストシナリオの例:
- ホスト障害時のHA failover
- ストレージバックエンド障害時のエラーハンドリング
- OVNクラスタ接続断時の振る舞い
- AWXジョブ失敗時のリトライ・ロールバック

### 負荷テスト

cirrus-simのlarge環境を使い、大規模構成での性能を検証する。

検証項目:
- 2,500ホスト環境でのスケジューラ応答時間
- 同時VM作成リクエストの処理能力
- DRSの再配分計算時間
- DBクエリ性能（ホスト列挙、リソース集計）

## 開発ワークフロー

### ローカル開発

```bash
# 1. cirrus-simを起動（small環境）
cd ../cirrus-sim && make serve

# 2. Cirrusコントローラーを起動
make serve

# 3. APIでリソース操作
curl -X POST http://localhost:<PORT>/api/v1/organizations ...

# 4. cirrus-simの管理APIで状態確認
curl http://localhost:<SIM_PORT>/sim/stats
curl http://localhost:<COMMON_PORT>/api/v1/events?limit=20
```

### CI

CIではcirrus-simをプロセスとして起動し、テストスイートを実行する。

```bash
# cirrus-sim起動
cirrus-sim -env environments/small.yaml &

# テスト実行
make test
make test-integration
```

### イベントログによるデバッグ

cirrus-simは全操作のイベントログを記録する。テスト失敗時にCirrusが発行した操作の順序と内容を確認できる。

```bash
# 直近のイベント取得
curl http://localhost:<COMMON_PORT>/api/v1/events?simulator=libvirt-sim&limit=50
curl http://localhost:<COMMON_PORT>/api/v1/events?simulator=ovn-sim&limit=50
```

## Cirrusとcirrus-simの接続

Cirrusの設定ファイルでシミュレータの接続先を指定する。本番環境との違いは接続先だけ。

```yaml
# cirrus.yaml（開発環境）
hosts:
  # cirrus-simの管理APIからホスト一覧を取得し、各ホストのlibvirt RPCポートに接続
  discovery: "http://localhost:<LIBVIRT_SIM_PORT>/sim/hosts"

network:
  ovn_nb_connection: "tcp:localhost:<OVN_SIM_PORT>"

storage:
  backends:
    - name: "ceph-pool-ssd"
      driver: "cirrus-storage-api"
      endpoint: "http://localhost:<STORAGE_SIM_PORT>"

hooks:
  awx:
    endpoint: "http://localhost:<AWX_SIM_PORT>"

topology_sync:
  netbox:
    endpoint: "http://localhost:<NETBOX_SIM_PORT>"
```

## VM作成の全体フロー（シミュレータ経由）

Cirrusが1台のVMを作成する際、各シミュレータへの操作は以下の順で行われる:

```
1. OVN NB DB (ovn-sim) に Logical_Switch_Port を作成
   → OVSDB transact: insert into Logical_Switch_Port

2. Storage API (storage-sim) でボリュームを作成
   → POST /api/v1/volumes

3. Storage API (storage-sim) でボリュームをエクスポート（ホストに接続）
   → POST /api/v1/volumes/{id}/export

4. libvirt RPC (libvirt-sim) でドメインを定義
   → DomainDefineXMLFlags (interfaceid で OVN LSP と紐付け、rbd で volume と紐付け)

5. libvirt RPC (libvirt-sim) でドメインを起動
   → DomainCreateWithFlags
```

シミュレータ間の直接連携は不要。Cirrusが各シミュレータを独立に操作し、`interfaceid` と `volume_id` で論理的に紐付ける。
