# サーバー起動ガイド

## 前提

- [portman](https://github.com/tjst-t/port-manager) がインストール済みであること
- Docker + docker-compose がインストール済みであること
- Go がインストール済みであること（バイナリビルド用）

シミュレータ（libvirtd-sim, storage-sim, awx-sim）はcirrusリポジトリに統合済み。外部リポジトリのクローンは不要。

PostgreSQLの外部起動は不要。embedded PostgreSQLを使用する。

## 起動

```bash
# controller + worker(3台) + cirrus-sim を一括起動
make serve

# ログ確認
make logs              # controller
make logs-worker       # worker-1/2/3 (docker)
make logs-sim          # cirrus-sim

# 停止
make stop
```

`make serve` は以下を自動で行う:

1. バイナリビルド（`bin/cirrus`, `bin/cirrus-sim`, `bin/libvirtd-sim`）
2. 既存プロセス・コンテナを停止
3. portman で全ポートを一括確保（sim + controller + worker）
4. cirrus-sim をホスト上で起動（embedded PostgreSQL含む）
5. controller をホスト上で起動
6. worker×3 を docker-compose で起動（privileged コンテナ、OVS + libvirtd-sim）
7. トポロジシード（storage-domain, network-domain, location, AZ）
8. 全ホストを自動 activate（開発用）

sim + controller はホスト直接実行（embedded-postgres のキャッシュ利用）。
worker だけ docker コンテナ（OVS + network namespace が必要なため privileged）。

## アーキテクチャ

```
┌─────────────────────────────────────────────────────┐
│  Host (ports assigned by portman)                   │
│                                                     │
│  ┌─────────────┐  ┌──────────────────────────────┐  │
│  │ controller  │  │ sim (cirrus-sim)             │  │
│  │ API + gRPC  │  │ PostgreSQL, common,          │  │
│  └─────────────┘  │ aggregator, libvirt-sim,     │  │
│                    │ awx-sim, storage-sim         │  │
│                    └──────────────────────────────┘  │
└─────────────────────────────────────────────────────┘

┌─────────────────── fabric network (10.100.0.0/24) ──┐
│  (host ports assigned by portman)                   │
│                                                     │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐   │
│  │ worker-1    │ │ worker-2    │ │ worker-3    │   │
│  │ privileged  │ │ privileged  │ │ privileged  │   │
│  │ OVS+br-int  │ │ OVS+br-int  │ │ OVS+br-int  │   │
│  │ libvirtd-sim│ │ libvirtd-sim│ │ libvirtd-sim│   │
│  │ cirrus agent│ │ cirrus agent│ │ cirrus agent│   │
│  └─────────────┘ └─────────────┘ └─────────────┘   │
└─────────────────────────────────────────────────────┘
```

## ポート割り当て

全ポートは portman が自動割り当て。ハードコードされたポートは存在しない。

| サービス | portman name | 用途 |
|---|---|---|
| sim common | `sim-common` | イベントログ、障害注入API |
| sim aggregator | `sim-aggregator:expose` | ダッシュボードWebUI |
| sim libvirt | `sim-libvirt` | libvirt-sim管理API |
| sim awx | `sim-awx` | awx-sim API |
| sim storage | `sim-storage` | storage-sim API |
| sim postgres | `sim-postgres` | embedded PostgreSQL |
| sim postgres-mgmt | `sim-postgres-mgmt` | PostgreSQL管理API |
| worker-1 | `worker-1` | worker-1 libvirtd-sim管理API |
| worker-2 | `worker-2` | worker-2 libvirtd-sim管理API |
| worker-3 | `worker-3` | worker-3 libvirtd-sim管理API |
| controller API | `api:expose` | REST API |
| controller gRPC | `grpc` | worker→controller通信 |

## 環境変数 / Makefile変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `CIRRUS_SIM_ENV` | `small` | シミュレータ環境（small/medium/large） |
| `AUTH_TOKENS` | `dev-token=dev-admin` | 認証トークン |
| `REGISTRATION_TOKEN` | `dev-registration-token` | worker登録トークン |

## トラブルシューティング

```bash
# ポートリース確認
portman list

# コンテナ状態確認
sudo docker ps | grep cirrus

# 全停止
make stop

# 状態リセット（プロセス + コンテナ + PIDファイル削除）
make clean-dev

# DBリセット（cirrus-simが起動中であること）
make reset-db

# フルリセット（停止 → 再起動）
make fresh
```
