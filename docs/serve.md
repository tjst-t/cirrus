# サーバー起動ガイド

## 前提

- [portman](https://github.com/tjst-t/port-manager) がインストール済みであること
- [cirrus-sim](https://github.com/tjst-t/cirrus-sim) がローカルにクローン・ビルド済みであること
  - デフォルトパス: `../cirrus-sim`（cirrusリポジトリの隣）
  - 環境変数 `CIRRUS_SIM_DIR` で変更可能
  - cirrus-simのビルドはcirrus-sim側で行う（`cd ../cirrus-sim && make build-unified`）

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
2. cirrus-sim を起動（ビルド済みバイナリが必要。small環境: 10ホスト、1 OVNクラスタ、1ストレージバックエンド）
3. cirrus-simの起動を待機（ヘルスチェック）
4. cirrus controller をビルド・起動（cirrus-simの各ポートを設定に注入）
5. cirrus worker を起動（cirrus-simのホスト一覧からホストごとに1プロセス）

## ポート割り当て

全ポートは portman が自動割り当て。ハードコードしない。

| サービス | portman name | 用途 |
|---|---|---|
| cirrus-sim common | `sim-common` | イベントログ、障害注入API |
| cirrus-sim dashboard | `sim-dashboard` | シミュレータWebUI |
| cirrus-sim libvirt | `sim-libvirt` | libvirt-sim管理API |
| cirrus-sim ovn | `sim-ovn` | ovn-sim管理API（OVSDBポートは別） |
| cirrus-sim awx | `sim-awx` | awx-sim API |
| cirrus-sim netbox | `sim-netbox` | netbox-sim API |
| cirrus-sim storage | `sim-storage` | storage-sim API |
| cirrus controller API | `api:expose` | REST API |
| cirrus controller gRPC | `grpc` | controller→worker通信 |
| PostgreSQL | `db` | 開発用DB |

cirrus-simが内部で使うホストごとのlibvirt RPCポート、OVSDBポートは `--range` で確保する。

## 整合性の確保

起動スクリプトは以下の順序で整合性を保証する:

```
1. cirrus-sim 起動
   ├─ portman でsimの全ポート確保
   └─ cirrus-sim プロセス起動 + ヘルスチェック待機

2. cirrus-sim からホスト一覧取得
   └─ GET /sim/hosts → [{host_id, libvirt_port, ...}, ...]

3. cirrus controller 起動
   ├─ portman で controller ポート確保
   ├─ sim のポート情報を設定に注入
   │   ├─ OVN NB接続先 = sim-ovnのOVSDBポート
   │   ├─ Storage API = sim-storageのポート
   │   ├─ AWX endpoint = sim-awxのポート
   │   └─ NetBox endpoint = sim-netboxのポート
   └─ controller プロセス起動

4. cirrus worker 起動（ホストごと）
   ├─ sim のホスト一覧をループ
   └─ 各ホストに対してworkerプロセスを起動
       ├─ --host-id = sim のホストID
       └─ libvirt接続先 = sim のホストごとのlibvirtポート
```

## 環境変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `CIRRUS_SIM_DIR` | `../cirrus-sim` | cirrus-simリポジトリのパス |
| `CIRRUS_SIM_ENV` | `small` | cirrus-sim環境（small/medium/large） |
| `CIRRUS_DB_DSN` | `postgres://cirrus:cirrus@localhost:$DB_PORT/cirrus?sslmode=disable` | DB接続文字列 |

## トラブルシューティング

```bash
# ポートリース確認
portman list

# 全プロセス強制停止
make stop

# 状態リセット（PIDファイル、ログ削除）
make clean-dev
```
