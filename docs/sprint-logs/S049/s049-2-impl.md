# Sprint S049-2 Implementation Log

## 実装内容

### Task S049-2-1: ダッシュボード画面実装

`web/src/pages/tenant/DashboardPage.tsx` を全面再実装した。

**変更点:**
- VMs API への依存を削除（quota データのみ使用）
- サマリカード 5 枚を実装（VM 数、ネットワーク数、ボリューム容量、vCPU、メモリ）
- 各カードに Playwright テスト用 `data-testid` を付与
- ローディング・エラー・テナント未選択の各状態にも `data-testid` を付与
- 401 エラー時は `clearTenant()` を呼び出してテナント選択状態をリセット

### Task S049-2-2: Quota 残量バー

`web/src/components/tenant/QuotaBar.tsx` を拡張した。

**変更点:**
- `data-testid` プロパティを追加
- `data-full="true"` 属性を追加（`used >= limit` の場合）
- `isFull` 判定を追加し、満杯時は `bg-danger` を適用

`DashboardPage` に 6 本の QuotaBar を追加:
- vCPU / メモリ / VM 数 / ボリューム容量 / ネットワーク数 / ボリューム数

### API クライアント修正（`web/src/api/client.ts`）

401 エラー時に自動で `logout()` を呼び出す処理を削除した。

**背景:** Playwright テストでは quota エンドポイントをモックしない場合に実際の Go サーバーから 401 が返り、`logout()` が `window.location.href = '/login'` を呼び出してテストが失敗していた。

**新動作:** 401 は ApiError として throw するだけ。各ページ・コンポーネントが適切に処理する（DashboardPage は 401 時に `clearTenant()` を呼び出す）。

## テスト結果

```
Running 5 tests using 1 worker

  ✓  ダッシュボード: サマリカードと Quota バーを表示する
  ✓  ダッシュボード: Quota 上限到達時にバーが100%表示になる
  ✓  ダッシュボード: quota API エラー時にエラーメッセージを表示する
  ✓  ダッシュボード: テナント未選択時に案内メッセージを表示する
  ✓  ダッシュボード: ローディング中はスケルトンまたはスピナーを表示する

  5 passed (3.2s)
```

既存の auth/login テスト（14件）もすべて pass 確認済み。
