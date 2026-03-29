# TODO

実装済みSprintの中で残っている改善項目。
各項目は対応Sprintで解消するか、独立して対応する。

---

## CLIクライアント（cirrusctl）

- [ ] **名前解決のサーバーサイドフィルタ対応**: 現在 `Resolve*` は全件取得してクライアント側で名前フィルタしている。サーバー側に `?name=` クエリパラメータが入ったら切り替える（Sprint 12 API仕上げで対応予定）
  - 該当箇所: `internal/client/identity.go` の `ResolveOrganization`, `ResolveTenant`

## データベース

- [ ] **UUID v7移行**: 設計（database.md）は「UUID v7（時系列ソート可能）」だが実装は `gen_random_uuid()`（v4）。新規マイグレーションで `gen_random_uuid()` のデフォルトをUUID v7生成関数に差し替える
- [ ] **resource_used JSONB列の設計整合**: database.mdでは「vmsテーブルから集計」と記載しているが、実装はheartbeatで直接上書き。heartbeatによるリアルタイム更新が正しい方式なので、database.mdの記載を修正する

## ホスト管理

- [ ] **Service/Store分離**: 現在 `host.Service` インターフェースを `host.Store` が直接実装している。ビジネスロジック層（状態遷移ルール、active→maintenance時のVM数チェック等）を `host.Manager` に分離し、Store はデータアクセスのみに限定する
- [ ] **Heartbeat Serviceインターフェースの型統一**: `Heartbeat(ctx, hostID string, ...)` の `hostID` が `string` で、他メソッドの `uuid.UUID` と不整合。gRPC境界でUUID変換し、Service層は `uuid.UUID` を受け取るように統一する

## API

- [ ] **PUT /api/v1/hosts/{id} 未実装**: api.mdに定義があるがエンドポイント未実装。ホスト属性（address等）の更新用

## ネットワーク

- [ ] **ネットワークモジュールのOVN→VPCモデル移行（Sprint 5N）**: Sprint 5の既存OVN実装を新しいVPCモデル（Network/Group/Policy + OVSデータプレーン）に全面書き換え。前提としてSprint 5S（cirrus-sim統合）を先に完了させる

## OVN→VPC移行の残タスク（Sprint 5Sで一部先行対応済み）

Sprint 5Sで以下を変更済み。Sprint 5Nで `network_domain` 概念自体を削除する際に合わせて対応すること:

- [ ] **NetworkDomain.OVNNBConnection フィールド削除**: `internal/topology/models.go` の `OVNNBConnection` フィールド、`internal/state/migrations/000004_topology.up.sql` の `ovn_nb_connection` カラムを削除。Sprint 5Sでバリデーションを任意に緩和済み（`internal/api/topology_handler.go`）
- [ ] **controller の --ovn-nb フラグ削除**: `internal/config/config.go` の `OVNNBConnection` 設定項目を削除
- [ ] **client の OVNNBConnection 参照削除**: `internal/client/topology.go` の関連コード
- [ ] **cirrus-sim リポジトリのアーカイブ**: 移行完了・動作確認後にアーカイブ化
