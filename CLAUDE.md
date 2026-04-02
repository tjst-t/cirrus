# Cirrus

> IaaS プラットフォーム。Go モジュラーモノリス（controller/worker 構成）。

## Tech Stack

Go 1.25, PostgreSQL, gRPC, libvirt, OVS/OpenFlow, cobra (CLI), chi (HTTP), golang-migrate

## Commands

- `make serve` — controller + worker + cirrus-sim を一括起動（バックグラウンド）
- `make stop` — 全プロセス停止
- `make build` — ビルド
- `make test` — テスト
- `make lint` — lint
- `make logs` / `make logs-worker` / `make logs-sim` — ログ確認
- 再度 `make serve` を実行すると、古いプロセスを自動で停止してから起動する

## Development Rules

- **ポート番号を直接指定しない**。全ポートは portman が自動割り当て
- **モジュール間はインターフェース経由のみ**。直接依存禁止。詳細は docs/ARCHITECTURE.md
- `.env` ファイルを git commit しない
- **OVS 制御は antrea-io/ofnet ライブラリを使う**（CLI ラッパー禁止）
- **Agent 機能はドメインごとにモジュール分離**し、実行バイナリで統合

### cirrusctl コマンド規則

- コマンド構造: テナント利用者向けはトップレベル、管理者向けは `admin` サブコマンド配下
- **リソース指定は UUID と名前の両方を受け付ける**: UUID パース失敗 → 名前でリスト取得 → フィルタ
  - 複数マッチ → エラーにして UUID 指定を促す
  - 名前解決に親リソースが必要な場合は `--org` 等フラグで補完
  - `internal/client` パッケージに `Resolve*` メソッドを置く
- 新規リソース追加時もこのパターンを踏襲

## Server

- portman でポート自動割り当て。サーバー起動スクリプト変更時は以下を参照:
  https://raw.githubusercontent.com/tjst-t/port-manager/main/docs/CLAUDE_INTEGRATION.md
- 起動の仕組みの詳細: [docs/serve.md](docs/serve.md)

## UI

UIを実装する際は design-system リポジトリに従う:
https://raw.githubusercontent.com/tjst-t/design-system/main/DESIGN_SYSTEM.md

## References

実装前に必ず該当ドキュメントを読むこと。

- **Architecture**: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — コンポーネント構成・データフロー
- **詳細設計** (docs/architecture.md) — モジュール間 IF、インターフェース定義
- **Sprint roadmap**: [`docs/ROADMAP.md`](docs/ROADMAP.md)
- **基本思想**: [`docs/README.md`](docs/README.md) — Phase 定義（**最初に読む**）
- ドメイン別: [host.md](docs/host.md) | [storage.md](docs/storage.md) | [network.md](docs/network.md) | [multitenancy.md](docs/multitenancy.md) | [tenant-model.md](docs/tenant-model.md)
- 実装詳細: [database.md](docs/database.md) | [api.md](docs/api.md) | [sequences.md](docs/sequences.md)
- 状態整合性: [reconciliation.md](docs/reconciliation.md)
- テスト戦略: [testing.md](docs/testing.md)
- 残タスク: [todo.md](docs/todo.md)
