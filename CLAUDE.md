# Cirrus

IaaSプラットフォーム。Goのモジュラーモノリス（controller/worker構成）。

## クイックリファレンス

- `make build` — ビルド
- `make test` — テスト
- `make lint` — lint
- `make serve` — controller + worker + cirrus-sim を一括起動（バックグラウンド）
- `make stop` — 全プロセス停止
- `make logs` / `make logs-worker` / `make logs-sim` — ログ確認
- 再度 `make serve` を実行すると、古いプロセスを自動で停止してから起動する

## サーバー起動

- ポート番号を直接指定してはいけない。全ポートは portman が自動割り当てする
- サーバー起動スクリプトを作成・変更する場合は、portman ガイドを参照すること:
  https://raw.githubusercontent.com/tjst-t/port-manager/main/docs/CLAUDE_INTEGRATION.md
- 起動の仕組みの詳細は [docs/serve.md](docs/serve.md) を参照
- .env ファイルを git commit してはいけない

## 設計ドキュメント

docs/配下に設計ドキュメントがある。実装前に必ず該当ドキュメントを読むこと。

- [docs/README.md](docs/README.md) — 基本思想、概念間の関係、Phase定義（**最初に読む**）
- [docs/architecture.md](docs/architecture.md) — コンポーネント構成、モジュール間IF、ディレクトリ構成
- [docs/roadmap.md](docs/roadmap.md) — 全29スプリントの実装計画

ドメイン別: [host.md](docs/host.md) | [storage.md](docs/storage.md) | [network.md](docs/network.md) | [multitenancy.md](docs/multitenancy.md)
実装詳細: [database.md](docs/database.md) | [api.md](docs/api.md) | [sequences.md](docs/sequences.md)
状態整合性: [reconciliation.md](docs/reconciliation.md) — desired vs actual stateの乖離検出・対応
テスト: [testing.md](docs/testing.md) — cirrus-simによるシミュレーションテスト
改善項目: [todo.md](docs/todo.md) — 実装済みSprintの残タスク

## アーキテクチャ要点

- **Controller**: API, Scheduler, OVN NB操作, Storage Backend操作
- **Worker**: ホストごとに1プロセス。libvirt VM操作 + ボリュームのホスト側アタッチ
- 物理インフラ管理はhook（AWX等）経由で外部委譲。仮想化層はCirrusが直接制御
- モジュール間はインターフェース経由のみ。詳細は docs/architecture.md

## テスト

[cirrus-sim](https://github.com/tjst-t/cirrus-sim) で本番同一プロトコルのシミュレータに接続してテスト。モック不要。詳細は docs/testing.md

## CLIクライアント（cirrusctl）

- コマンド実装は `cmd/cirrusctl/`、APIクライアントは `internal/client/`
- **コマンド構造は利用者/管理者で分離**:
  - トップレベル: テナント利用者向け（`org`, `tenant`, `role`, 将来の `vm`, `network`, `volume` 等）
  - `admin` サブコマンド配下: インフラ管理者向け（`admin host`, 将来の `admin storage-domain`, `admin host-profile` 等）
- **リソース指定はIDと名前の両方を受け付ける**: UUIDパースを試み、失敗したら名前としてリスト取得→名前フィルタで解決する
  - 名前が複数マッチした場合はエラーにしてUUID指定を促す
  - 名前解決に親リソースが必要な場合（例: tenant名にはorgが必要）は `--org` フラグで補完する
  - `internal/client` パッケージに `Resolve*` メソッドを置く
- 新規リソースのCLIコマンド追加時もこのパターンを踏襲すること

## UI

UIを実装する際は design-system リポジトリに従う:
https://raw.githubusercontent.com/tjst-t/design-system/main/DESIGN_SYSTEM.md
