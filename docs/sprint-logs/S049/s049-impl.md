# Sprint S049 実装ログ

## 実行日時
2026-04-12

## S049-1: Egress / Ingress 管理

### 変更ファイル
- `web/src/api/egress.ts` — `EgressGateway`（誤）→ `Egress`（正）に修正。`EgressConfig` 追加。作成リクエストを `{type, config}` 形式に修正
- `web/src/api/ingress.ts` — `IngressEndpoint`（誤）→ `Ingress`（正）に修正。`IngressConfig`/`IpPool` 追加。作成リクエストを `{type, public_ip, ip_pool_id, config}` 形式に修正。`listIpPools()` 追加。`ip_pool_id` を必須フィールドに修正（バックエンド必須チェックに合わせて）
- `web/src/pages/tenant/EgressPage.tsx` — バックエンド API 型に合わせて全面再実装。data-testid 付与済み
- `web/src/pages/tenant/IngressPage.tsx` — バックエンド API 型に合わせて全面再実装。IP プール選択・パブリック IP 入力・ターゲット VM 選択フォーム。data-testid 付与済み

### 修正した問題（レビュー後）
- `CreateIngressRequest.ip_pool_id` をオプション→必須に修正
- IngressPage の UI バリデーションに ip_pool_id 未選択チェック追加
- IngressPage の import 順序破損を修正

### テスト結果
`npx playwright test e2e/s049-egress-ingress.spec.ts`: **15/15 pass** → レビュー後修正で **15/15 pass** 維持

---

## S049-2: ダッシュボード

### 変更ファイル
- `web/src/pages/tenant/DashboardPage.tsx` — スタブから全面実装。`GET /api/v1/tenants/{id}/quota` のみ使用
  - サマリカード 5 枚（VM 数・ネットワーク数・ボリューム容量・vCPU・メモリ）
  - Quota バー 8 本（vCPU/メモリ/VM数/ボリューム容量/ネットワーク数/ボリューム数/Egress数/Ingress数）
  - 401 エラー時 → `clearTenant()` でテナント選択に戻す
- `web/src/components/tenant/QuotaBar.tsx` — `data-testid` プロパティ追加、`data-full="true"` 実装
- `web/src/api/client.ts` — 401 時の自動 logout() 除去（各ページで個別ハンドリングに変更）

### 修正した問題（レビュー後）
- MB→GB 変換を `Math.round` → `Math.floor` に修正（使用量の過剰表示防止）
- Egress/Ingress の Quota バーを追加

### テスト結果
`npx playwright test e2e/s049-dashboard.spec.ts`: **5/5 pass** → レビュー後修正で **5/5 pass** 維持

---

## 全体テスト結果
`npx playwright test e2e/s049-egress-ingress.spec.ts e2e/s049-dashboard.spec.ts`: **20/20 pass**

## 設計判断ログ
- **api.list vs api.get**: Egress/Ingress エンドポイントはバックエンドが配列直返し（PagedResponse 非対応）のため `api.get<T[]>` を使用。Networks は PagedResponse 対応のため `api.list` を継続使用
- **401 ハンドリング全ページ統一**: S049 スコープ外。S051（エラー UX 改善）での対応を推奨
- **SummaryCard dead code**: テスト通過済みのためリファクタは別スプリントに持ち越し
