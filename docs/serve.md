# サーバー起動ガイド

## 前提

- [portman](https://github.com/tjst-t/port-manager) がインストール済みであること
- [cirrus-sim](https://github.com/tjst-t/cirrus-sim) がローカルにクローン・ビルド済みであること
  - デフォルトパス: `../cirrus-sim`（cirrusリポジトリの隣）
  - 環境変数 `CIRRUS_SIM_DIR` で変更可能
  - cirrus-simのビルドはcirrus-sim側で行う（`cd ../cirrus-sim && make build-unified`）
- PostgreSQLが起動済みであること
  - デフォルト: `postgres://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable`
  - Makefile変数 `DB_DSN` で変更可能

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
2. portman で全ポートを一括確保（sim + controller）
3. cirrus-sim を起動（ビルド済みバイナリが必要。small環境: 10ホスト、1 OVNクラスタ、1ストレージバックエンド）
4. cirrus-simの起動を待機（イベントAPIへのヘルスチェック）
5. cirrus controller を起動（cirrus-simの各ポートとDB DSNを設定に注入）
6. cirrus-simのホスト一覧を取得し、ホストごとにworkerプロセスを起動

## ポート割り当て

portmanが全ポートを1回のコマンドで一括割り当てし、1つのenvファイル（`/tmp/cirrus-dev/portman.env`）に出力する。

| サービス | portman name | 用途 |
|---|---|---|
| cirrus-sim common | `sim-common` | イベントログ、障害注入API |
| cirrus-sim dashboard | `sim-dashboard:expose` | シミュレータWebUI |
| cirrus-sim libvirt | `sim-libvirt` | libvirt-sim管理API |
| cirrus-sim ovn | `sim-ovn` | ovn-sim管理API |
| cirrus-sim awx | `sim-awx` | awx-sim API |
| cirrus-sim netbox | `sim-netbox` | netbox-sim API |
| cirrus-sim storage | `sim-storage` | storage-sim API |
| cirrus-sim libvirt hosts | `sim-libvirt-hosts` (range=20) | ホストごとのlibvirt RPCポート |
| cirrus-sim ovn clusters | `sim-ovn-clusters` (range=5) | OVNクラスタごとのOVSDBポート |
| cirrus controller API | `api:expose` | REST API |
| cirrus controller gRPC | `grpc` | worker→controller heartbeat通信 |

PostgreSQLポートはportmanの管理外。DBは外部で起動済みの前提。

## 整合性の確保

起動スクリプトは以下の順序で整合性を保証する:

```
1. portman で全ポート一括確保
   └─ sim-* + api + grpc を1つの env ファイルに出力

2. cirrus-sim 起動
   ├─ env ファイルからポート読み込み
   ├─ cirrus-sim プロセス起動
   └─ /api/v1/events への疎通確認で起動待機

3. cirrus controller 起動
   ├─ env ファイルからポート読み込み
   ├─ DB_DSN で PostgreSQL に接続
   ├─ マイグレーション実行
   ├─ sim のポート情報を CLI フラグで注入
   │   ├─ --ovn-nb=tcp:localhost:$SIM_OVN_PORT
   │   ├─ --storage-endpoint=http://localhost:$SIM_STORAGE_PORT
   │   ├─ --awx-endpoint=http://localhost:$SIM_AWX_PORT
   │   └─ --netbox-endpoint=http://localhost:$SIM_NETBOX_PORT
   └─ controller プロセス起動

4. cirrus worker 起動（ホストごと）
   ├─ GET /sim/hosts でホスト一覧取得
   └─ 各ホストに対してworkerプロセスを起動
       ├─ --host-id = sim のホストID
       ├─ --controller = localhost:$GRPC_PORT
       └─ --libvirt-uri = tcp://localhost:{ホストのlibvirtポート}
```

## 環境変数 / Makefile変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `CIRRUS_SIM_DIR` | `../cirrus-sim` | cirrus-simリポジトリのパス |
| `CIRRUS_SIM_ENV` | `small` | cirrus-sim環境（small/medium/large） |
| `DB_DSN` | `postgres://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable` | PostgreSQL接続文字列 |

## トラブルシューティング

```bash
# ポートリース確認
portman list

# 全プロセス強制停止
make stop

# 状態リセット（PIDファイル、ログ削除）
make clean-dev
```
