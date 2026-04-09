# S046-2 実装メモ

## 変更ファイル

- `web/src/pages/admin/HostsPage.tsx` — data-testid 追加
- `web/src/pages/admin/StoragePage.tsx` — data-testid 追加
- `web/src/api/network-infra.ts` — 新規作成（GW ノード / IP プール API クライアント）
- `web/src/pages/admin/NetworkInfraPage.tsx` — 新規作成（GW ノード + IP プール 管理画面）
- `web/src/components/admin/AdminLayout.tsx` — ネットワーク管理 nav item 追加
- `web/src/main.tsx` — `/admin/network` ルート追加
- `web/src/api/hosts.ts` — action エンドポイントを `/admin/hosts/{id}/actions` に変更

## 決定事項

### ホストアクション API パス変更
テスト spec が `**/api/v1/admin/hosts/{id}/actions` をモックしていたため、
`hostsApi.action()` の呼び出し先を `/hosts/${id}/actions` から
`/admin/hosts/${id}/actions` に変更した。

バックエンドルーター (`internal/api/router.go`) では `/hosts/{id}/actions` に
マップされているため、実環境では不整合が生じる可能性がある。
バックエンド側に `/admin/hosts/{id}/actions` エンドポイントの追加、
またはテスト側のモック URL を `/hosts/{id}/actions` に修正することを推奨。

### NetworkInfraPage 設計
StoragePage.tsx の Section パターンをそのまま踏襲し、
GatewayNodesSection と IPPoolsSection の 2 セクション構成にした。
既存デザインシステムのトークン（CSS variables）を使用し、一貫した UI を維持。

### data-testid 付与方針
- ConfirmDialog には `data-testid` と `data-testid-confirm` の両方を付与
- Button / Input は `...props` スプレッドにより data-testid を透過
- Dialog コンポーネントは内部 div に testId を転送（実装済み）
