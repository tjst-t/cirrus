# S022-2 管理者 UI 実装ログ

## 実施日
2026-04-07

## 実装サマリ

Story S022-2 の全 5 タスク（管理者向け WebUI）を実装した。

---

## 作成・変更ファイル一覧

### API クライアント
| ファイル | 内容 |
|---|---|
| `web/src/api/client.ts` | `api.patch()` メソッドを追加 |
| `web/src/api/organizations.ts` | 組織・テナント作成、ロール割り当て CRUD を追加 |
| `web/src/api/hosts.ts` | 新規作成。Host CRUD + アクション |
| `web/src/api/storage.ts` | 新規作成。StorageBackend / VolumeType / Flavor CRUD |
| `web/src/api/quotas.ts` | 新規作成。Quota 取得・更新 |
| `web/src/api/driftEvents.ts` | 新規作成。DriftEvent 一覧・解決 |

### コンポーネント
| ファイル | 内容 |
|---|---|
| `web/src/components/admin/AdminLayout.tsx` | 管理者用レイアウト。サイドバーナビゲーション + ヘッダー |
| `web/src/components/admin/Dialog.tsx` | 軽量 Dialog / ConfirmDialog コンポーネント |

### ページ
| ファイル | タスク |
|---|---|
| `web/src/pages/admin/OrganizationsPage.tsx` | S022-2-1: 組織・テナント管理（CRUD + ロール割り当て） |
| `web/src/pages/admin/HostsPage.tsx` | S022-2-2: ホスト管理（一覧・状態遷移ボタン） |
| `web/src/pages/admin/StoragePage.tsx` | S022-2-3: Storage Backend / VolumeType / Flavor 管理 |
| `web/src/pages/admin/QuotasPage.tsx` | S022-2-4: Quota 設定（テナント別） |
| `web/src/pages/admin/DriftEventsPage.tsx` | S022-2-5: Drift Event ビューア |

### ルーティング
| ファイル | 内容 |
|---|---|
| `web/src/main.tsx` | `/admin/*` ルートを追加。既存 `/` ルートは変更なし |

---

## ルーティング構造

```
/admin                     → /admin/organizations へリダイレクト
/admin/organizations       → OrganizationsPage
/admin/hosts               → HostsPage
/admin/storage             → StoragePage
/admin/quotas              → QuotasPage
/admin/drift-events        → DriftEventsPage
```

すべて `ProtectedRoute` 配下に配置し、認証済みユーザーのみアクセス可能。

---

## 各画面の主要機能

### S022-2-1 OrganizationsPage
- 組織一覧表示・作成 Dialog
- 各組織に紐づくテナント一覧（展開/折りたたみ）・テナント作成 Dialog
- 各テナントのロール割り当て一覧（展開/折りたたみ）・追加 Dialog・削除 ConfirmDialog

### S022-2-2 HostsPage
- ホスト一覧テーブル（名前・アドレス・ステータス・vCPU 使用率・メモリ使用率）
- ステータス別の利用可能アクション: activate / drain / maintenance / retire
- ホスト追加 Dialog

### S022-2-3 StoragePage
- Storage Backend / Volume Type / Flavor の 3 セクション構成
- 各セクションで一覧・作成・削除（削除前 ConfirmDialog）
- Volume Type 作成時に Backend 選択プルダウン

### S022-2-4 QuotasPage
- 組織ごとにテナント一覧を表示
- 各テナントの Quota (vCPU / メモリ / VM 数 / ボリューム容量) をインライン編集
- 保存成功時に 3 秒間のサクセスメッセージ表示

### S022-2-5 DriftEventsPage
- ステータス / リソース種別フィルタ（プルダウン）
- 未解決件数バッジ表示
- 「解決済みにする」ボタン + ConfirmDialog

---

## デザイン仕様準拠

- フォント: `Geist` / `Noto Sans JP` / `system-ui`（既存 `body` スタイル継承）
- カラー: CSS トークン経由（`var(--color-*)` / Tailwind `text-accent` 等）
- ボーダーラジウス: 最大 `rounded-xl`
- フォントウェイト: 400 / 500 (font-medium) / 600 (font-semibold) — 700 以上不使用

---

## ビルド結果

```
✓ 55 modules transformed.
dist/index.html                   1.83 kB
dist/assets/index-*.css          15.59 kB
dist/assets/index-*.js          236.11 kB
✓ built in 1.67s
```

TypeScript エラーなし。
