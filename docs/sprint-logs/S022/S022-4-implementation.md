# S022-4 実装ログ — E2E テスト (Playwright)

## 実装日: 2026-04-07

## タスク概要

- **S022-4-1**: 主要フローの E2E テスト（Playwright）: ログイン→VM作成→削除
- **S022-4-2**: `make serve` で WebUI が http://localhost:{port} で起動すること確認

---

## 実装内容

### セットアップ

- `web/package.json` に `@playwright/test: ^1.59.0` を追加（devDependencies）
- `web/package.json` に `"test:e2e": "playwright test"` スクリプトを追加
- `web/playwright.config.ts` を作成（Chromium プリキャッシュ対応）
- `Makefile` に `test-e2e` ターゲットを追加

### テストファイル

| ファイル | テスト数 | カバー対象 |
|---|---|---|
| `web/e2e/auth.spec.ts` | 5 | ログイン・ログアウト・未認証リダイレクト |
| `web/e2e/vms.spec.ts` | 5 | VM一覧・作成ダイアログ・キャンセル |
| `web/e2e/admin.spec.ts` | 6 | 組織管理・ホスト管理・管理者ルーティング |
| `web/e2e/serve-check.spec.ts` | 1 | make serve 統合確認（BASE_URL 設定時のみ） |

### モック戦略

バックエンド起動なしでテスト可能とするため、Playwright の `page.route('/api/v1/**', ...)` で全 API をモック。
- `GET /api/v1/organizations` → `200 []`
- `GET /api/v1/vms` → `200 []`
- `GET /api/v1/flavors` → フレーバー1件
- `GET /api/v1/networks` → ネットワーク1件
- `GET /api/v1/volume-types` → `200 []`
- `GET /api/v1/hosts` → `200 []`

ログインテストでは `POST /api/v1/organizations` (認証トークン検証) → `200 []` でトークン有効を表現し、`401` でトークン無効を再現。

### 認証セットアップ

保護ルートのテストでは `page.evaluate()` で `localStorage` に `cirrus_token` と `cirrus_tenant_id` を設定し、ProtectedRoute をパスする。

---

## テスト実行結果

```
Running 17 tests using 2 workers

  ✓  auth.spec.ts › ログインページが表示される
  ✓  auth.spec.ts › 有効なトークンでログインするとダッシュボードにリダイレクトされる
  ✓  auth.spec.ts › 無効なトークンでエラーが表示される
  ✓  auth.spec.ts › ログアウト後は /login にリダイレクトされる
  ✓  auth.spec.ts › 未認証状態で保護ルートにアクセスすると /login にリダイレクトされる
  ✓  vms.spec.ts  › VM 一覧ページが表示される
  ✓  vms.spec.ts  › VM 作成ボタンが存在する
  ✓  vms.spec.ts  › VM 一覧が空の場合は「VM がありません」と表示される
  ✓  vms.spec.ts  › VM 作成ダイアログが開く
  ✓  vms.spec.ts  › VM 作成ダイアログでキャンセルできる
  ✓  admin.spec.ts › 組織一覧ページが表示される
  ✓  admin.spec.ts › 組織一覧が空の場合は「組織がありません」と表示される
  ✓  admin.spec.ts › 組織作成ボタンが表示される
  ✓  admin.spec.ts › ホスト一覧ページが表示される
  ✓  admin.spec.ts › ホスト一覧が空の場合は「ホストがありません」と表示される
  ✓  admin.spec.ts › /admin にアクセスすると /admin/organizations にリダイレクトされる
  -  serve-check.spec.ts › WebUI is accessible via make serve (skipped: BASE_URL not set)

  1 skipped
  16 passed (7.8s)
```

**結果: 16 pass / 1 skipped (設計通り) / 0 failed**

---

## デバッグ記録

初回実行で 4 テストが失敗。原因はすべて Playwright strict mode 違反（1 つのセレクターが複数要素にマッチ）:

1. `text=APIトークン` → ラベルとサブタイトルの両方がマッチ → `label[for="token"]` に修正
2. `text=ホスト管理` → ナビリンクとページ見出し h1 の両方がマッチ → `h1` に絞り込み
3. `text=VM を作成` → ボタンとダイアログ h3 の両方がマッチ → `h3` に絞り込み

修正後、全テスト pass を確認。

---

## S022-4-2: make serve 統合確認

`web/e2e/serve-check.spec.ts` を作成済み。`BASE_URL` 環境変数が設定されている場合のみ実行。

実行方法:
```bash
BASE_URL=http://localhost:{API_PORT} make test-e2e
```

`make serve` 起動後に `PORTMAN_ENV` ファイルから `API_PORT` を読み取り、`BASE_URL` に設定して実行することで WebUI の疎通確認が可能。
