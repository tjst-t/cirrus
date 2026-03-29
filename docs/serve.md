# サーバー起動ガイド

## 前提

- [portman](https://github.com/tjst-t/port-manager) がインストール済みであること

シミュレータ（libvirtd-sim, storage-sim, awx-sim）はcirrusリポジトリに統合済み。外部リポジトリのクローンは不要。

PostgreSQLの外部起動は不要。embedded PostgreSQLを使用する。

`make serve` はレイヤー1/2（ビジネスロジック + OVSフロー変換）の開発用。レイヤー3（実OVS結合テスト）はdocker-composeを使用する。詳細は [testing.md](testing.md) を参照。

## 起動

```bash
# controller + worker(10台) + cirrus-sim を一括起動
make serve

# ログ確認
make logs              # controller
make logs-worker       # worker
make logs-sim          # cirrus-sim

# 停止
make stop
```

`make serve` は以下を自動で行う:

1. 既存プロセスがあれば全て停止（PIDファイルで追跡）
2. portman で全ポートを一括確保（sim + postgres + controller）
3. 統合シミュレータを起動（embedded PostgreSQL含む。small環境: 10ホスト、1ストレージバックエンド）
4. シミュレータの起動を待機（ヘルスチェック）
5. cirrus controller を起動（シミュレータの各ポートとDB DSNを設定に注入）
6. ホスト一覧を取得し、ホストごとにworkerプロセスを起動
7. 全ホストを自動 activate（開発用）

## ポート割り当て

portmanが全ポートを1回のコマンドで一括割り当てし、1つのenvファイル（`/tmp/cirrus-dev/portman.env`）に出力する。

| サービス | portman name | 用途 |
|---|---|---|
| sim common | `sim-common` | イベントログ、障害注入API |
| sim dashboard | `sim-dashboard:expose` | シミュレータWebUI |
| sim libvirt | `sim-libvirt` | libvirt-sim管理API |
| sim awx | `sim-awx` | awx-sim API |
| sim storage | `sim-storage` | storage-sim API |
| sim postgres | `sim-postgres` | embedded PostgreSQL |
| sim postgres-mgmt | `sim-postgres-mgmt` | PostgreSQL管理API（テーブル閲覧等） |
| sim libvirt hosts | `sim-libvirt-hosts` (range=20) | ホストごとのlibvirt RPCポート |
| cirrus controller API | `api:expose` | REST API |
| cirrus controller gRPC | `grpc` | worker→controller heartbeat通信 |

全ポートがportman管理下にあり、ハードコードされたポートは存在しない。

## 整合性の確保

起動スクリプトは以下の順序で整合性を保証する:

```
1. portman で全ポート一括確保
   └─ sim-* + sim-postgres + api + grpc を1つの env ファイルに出力

2. 統合シミュレータ起動（embedded PostgreSQL 含む）
   ├─ env ファイルからポート読み込み
   ├─ embedded PostgreSQL を起動（データベース cirrus を自動作成）
   ├─ 各シミュレータプロセス起動（libvirtd-sim, storage-sim, awx-sim）
   └─ ヘルスチェックで起動待機

3. cirrus controller 起動
   ├─ env ファイルからポート読み込み
   ├─ DB_DSN を SIM_POSTGRES_PORT から動的構築
   ├─ マイグレーション実行
   ├─ sim のポート情報を CLI フラグで注入
   │   ├─ --storage-endpoint=http://localhost:$SIM_STORAGE_PORT
   │   └─ --awx-endpoint=http://localhost:$SIM_AWX_PORT
   └─ controller プロセス起動

4. cirrus worker 起動（ホストごと）
   ├─ GET /sim/hosts でホスト一覧取得
   └─ 各ホストに対してworkerプロセスを起動
       ├─ --registration-token で自動登録
       ├─ --controller = localhost:$GRPC_PORT
       └─ --libvirt-uri = tcp://localhost:{ホストのlibvirtポート}
```

## 環境変数 / Makefile変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `CIRRUS_SIM_ENV` | `small` | シミュレータ環境（small/medium/large） |

## トラブルシューティング

```bash
# ポートリース確認
portman list

# 全プロセス強制停止
make stop

# 状態リセット（PIDファイル、ログ削除）
make clean-dev

# DBリセット（cirrus-simが起動中であること）
make reset-db
```
