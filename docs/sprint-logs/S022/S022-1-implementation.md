# S022-1 フロントエンド基盤 実装ログ

実装日: 2026-04-07

## 作成ファイル一覧

### web/ (フロントエンドプロジェクト)

| ファイル | 内容 |
|---|---|
| `web/package.json` | npm プロジェクト定義。依存: react 18, react-router-dom v6, tailwindcss 3, vite 6, shadcn/ui基盤 (radix-ui, clsx, tailwind-merge) |
| `web/tsconfig.json` | TypeScript プロジェクト参照ルート |
| `web/tsconfig.app.json` | アプリ用 TS 設定 (strict, path alias @/*) |
| `web/tsconfig.node.json` | Vite 設定ファイル用 TS 設定 (@types/node 含む) |
| `web/vite.config.ts` | Vite 設定: /api プロキシ (VITE_API_BASE_URL 環境変数対応、デフォルト http://localhost:8080)、@/ エイリアス |
| `web/tailwind.config.ts` | デザイントークン統合: accent/success/warning/danger カラー、4px グリッドスペーシング、最大 8px ボーダーラジウス |
| `web/postcss.config.js` | PostCSS (tailwindcss + autoprefixer) |
| `web/index.html` | エントリ HTML: CSS カスタムプロパティでデザイントークン定義、Noto Sans JP フォント読み込み |
| `web/src/index.css` | Tailwind ディレクティブ + CSS カスタムプロパティ (@layer base) |
| `web/src/main.tsx` | React エントリ: BrowserRouter + Routes (/, /login, /* → /) |
| `web/src/api/client.ts` | fetch ラッパー: cirrus_token → Authorization Bearer、cirrus_tenant_id → X-Tenant-ID、ベースURL /api/v1 |
| `web/src/api/organizations.ts` | Organizations / Tenants API 型定義・クライアント |
| `web/src/lib/auth.ts` | localStorage ラッパー (cirrus_token / cirrus_tenant_id)、logout() |
| `web/src/lib/utils.ts` | cn() (clsx + tailwind-merge) |
| `web/src/hooks/useAuth.ts` | 認証状態フック |
| `web/src/hooks/useTenant.ts` | テナント選択フック |
| `web/src/components/Button.tsx` | ボタン (primary / secondary / danger / ghost、sm / md / lg) |
| `web/src/components/Input.tsx` | テキスト入力 |
| `web/src/components/Header.tsx` | ヘッダー: テナント切り替えドロップダウン + ログアウトボタン |
| `web/src/components/ProtectedRoute.tsx` | 未認証時 /login へリダイレクト (React Router v6 Outlet) |
| `web/src/pages/LoginPage.tsx` | トークン入力フォーム → localStorage 保存 → ダッシュボードへ |
| `web/src/pages/DashboardPage.tsx` | ダッシュボード (テナント ID 表示) |

### Go 側変更

| ファイル | 変更内容 |
|---|---|
| `internal/api/router.go` | spaHandler 追加: web/dist/ を chi で FileServer 配信、SPA フォールバック (非 /api/* パスは index.html)、web/dist が存在しない場合はスキップ |

### Makefile 変更

| ターゲット | 内容 |
|---|---|
| `web-install` | `cd web && npm install` (web/ 非存在時スキップ) |
| `web-build` | `cd web && npm run build` (node_modules なければ install も実行、web/ 非存在時スキップ) |
| `build` | web-build を先行実行してから Go バイナリビルド |

## ビルド結果

```
> cirrus-web@0.1.0 build
> tsc -b && vite build

vite v6.4.2 building for production...
transforming...
✓ 44 modules transformed.
rendering chunks...
computing gzip size...
dist/index.html                   1.83 kB │ gzip:  0.81 kB
dist/assets/index-CjC73swf.css    9.39 kB │ gzip:  2.64 kB
dist/assets/index-DvSllW__.js   190.70 kB │ gzip: 62.50 kB │ map: 833.20 kB
✓ built in 1.57s
```

Go ビルド: `go build ./internal/api/...` および `go build -o bin/cirrus ./cmd/cirrus/` 成功。

## 実装上の決定事項

1. **Vite プロキシターゲット**: portman が自動割り当てするため、デフォルト `http://localhost:8080` を環境変数 `VITE_API_BASE_URL` でオーバーライド可能にした。

2. **spaHandler 実装**: `http.FileServer` のデフォルト動作はディレクトリリストやリダイレクトが煩雑なため、`os.Stat` で存在チェックし、見つからない場合は `index.html` を直接 `http.ServeFile` する独自ハンドラを実装した。

3. **web/dist 存在確認**: コントローラー起動時に `web/dist` が存在しない場合（初回 `npm install` 未実施）でも起動できるよう、`staticDistHandler()` は nil を返し、ルーターへの登録をスキップする設計にした。

4. **フォント**: Geist は npm パッケージが存在するが、Google Fonts 経由の Noto Sans JP と共存させるため、HTML の `<link>` で読み込む方式とし、Geist はシステムフォントとして fallback に置いた（Geist がインストール済み環境では自動適用）。

5. **box-shadow**: デザインシステム規約「box-shadow は重ねない」に従い、カード・ドロップダウンには単層の shadow のみ使用した。

6. **フォントウェイト**: 700 以上禁止のため、tailwind.config.ts で fontWeight は 400/500/600 のみ定義した。
