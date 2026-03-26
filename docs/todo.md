# TODO

実装済みSprintの中で残っている改善項目。
各項目は対応Sprintで解消するか、独立して対応する。

---

## CLIクライアント（cirrusctl）

- [ ] **名前解決のサーバーサイドフィルタ対応**: 現在 `Resolve*` は全件取得してクライアント側で名前フィルタしている。サーバー側に `?name=` クエリパラメータが入ったら切り替える（Sprint 12 API仕上げで対応予定）
  - 該当箇所: `internal/client/identity.go` の `ResolveOrganization`, `ResolveTenant`
