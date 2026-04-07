# S022-3 テナント UI 実装ログ

## 実装日: 2026-04-07

## 完了タスク

### S022-3-1: ダッシュボード
- `web/src/pages/tenant/DashboardPage.tsx`
- クォータ使用量（vCPU・メモリ・VM数・ボリューム容量）を `QuotaBar` で表示
- 状態別 VM サマリカード（running/stopped/error）
- 最新 VM 一覧（最大5件、VM 詳細へのリンク付き）

### S022-3-2: VM 管理画面
- `web/src/pages/tenant/VmsPage.tsx` — 一覧・作成・start/stop/reboot/delete
- `web/src/pages/tenant/VmDetailPage.tsx` — 詳細・アクション・削除確認
- 作成フォーム: フレーバー・ネットワーク・ボリュームタイプ選択
- 状態に応じてアクションボタンを有効/無効化

### S022-3-3: ネットワーク管理画面
- `web/src/pages/tenant/NetworksPage.tsx`
- Network CRUD
- アコーディオン展開で Group・Policy の CRUD を同一画面で実施
- Policy 作成フォーム: 方向・プロトコル・ポート範囲・CIDR・アクション

### S022-3-4: ボリューム管理画面
- `web/src/pages/tenant/VolumesPage.tsx`
- 一覧・作成・削除・リサイズ（拡張のみ）
- 使用中ボリュームの削除ボタンを無効化

### S022-3-5: Egress / Ingress 管理画面
- `web/src/pages/tenant/EgressPage.tsx` — Egress ゲートウェイ CRUD
- `web/src/pages/tenant/IngressPage.tsx` — Ingress エンドポイント CRUD（VM・ポート・プロトコル指定）

## 新規ファイル

### API クライアント
- `web/src/api/quota.ts`
- `web/src/api/vms.ts`
- `web/src/api/networks.ts`
- `web/src/api/volumes.ts`
- `web/src/api/egress.ts`
- `web/src/api/ingress.ts`

### コンポーネント
- `web/src/components/tenant/TenantLayout.tsx` — サイドバーナビゲーション（Header統合）
- `web/src/components/tenant/QuotaBar.tsx` — クォータ進捗バー

### ページ
- `web/src/pages/tenant/DashboardPage.tsx`
- `web/src/pages/tenant/VmsPage.tsx`
- `web/src/pages/tenant/VmDetailPage.tsx`
- `web/src/pages/tenant/NetworksPage.tsx`
- `web/src/pages/tenant/VolumesPage.tsx`
- `web/src/pages/tenant/EgressPage.tsx`
- `web/src/pages/tenant/IngressPage.tsx`

## 変更ファイル
- `web/src/main.tsx` — テナントルート追加（管理者ルートは維持）

## 設計判断
- `TenantLayout` はヘッダーを内包する独立コンポーネントとして実装（`Header.tsx` への変更なし）
- 旧 `DashboardPage`（`pages/DashboardPage.tsx`）は `main.tsx` から参照を除去し、テナントダッシュボードに置き換え
- shadcn/ui は未インストール。既存の `Button.tsx`・`Input.tsx` を活用し、テーブル・ダイアログは Tailwind CSS で実装

## ビルド結果
```
✓ 68 modules transformed.
✓ built in 1.78s
```
TypeScript エラーなし。
