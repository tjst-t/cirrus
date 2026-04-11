# S048-2 テスト結果

実行日時: 2026-04-11

## テスト結果: 9/9 passed

```
Running 9 tests using 1 worker

  ✓  1 [chromium] › e2e/s048-volume.spec.ts:46:3 › S048-2: ボリューム管理 › ボリューム一覧: ボリュームが存在しない場合に空状態を表示する (448ms)
  ✓  2 [chromium] › e2e/s048-volume.spec.ts:55:3 › S048-2: ボリューム管理 › ボリューム一覧: stateバッジが正しく表示される (384ms)
  ✓  3 [chromium] › e2e/s048-volume.spec.ts:70:3 › S048-2: ボリューム管理 › ボリューム作成: 名前とサイズを入力して作成できる（202 job_id） (587ms)
  ✓  4 [chromium] › e2e/s048-volume.spec.ts:102:3 › S048-2: ボリューム管理 › ボリューム作成: ボリュームタイプを選択して作成できる (589ms)
  ✓  5 [chromium] › e2e/s048-volume.spec.ts:122:3 › S048-2: ボリューム管理 › ボリュームリサイズ: new_size_gbを指定してリサイズできる (510ms)
  ✓  6 [chromium] › e2e/s048-volume.spec.ts:143:3 › S048-2: ボリューム管理 › ボリュームリサイズ: 現在のサイズ以下を指定するとエラーになる (533ms)
  ✓  7 [chromium] › e2e/s048-volume.spec.ts:157:3 › S048-2: ボリューム管理 › ボリューム削除: available状態のボリュームを確認後に削除できる (569ms)
  ✓  8 [chromium] › e2e/s048-volume.spec.ts:182:3 › S048-2: ボリューム管理 › ボリューム削除: in_use状態のボリュームは削除ボタンが無効化される (367ms)
  ✓  9 [chromium] › e2e/s048-volume.spec.ts:190:3 › S048-2: ボリューム管理 › ボリューム一覧: API失敗時にエラーメッセージを表示する (344ms)

  9 passed (5.7s)
```

## 修正内容

### Task S048-2-1: `web/src/api/volumes.ts` バグ修正

- `VolumeStatus` → `VolumeState` に型名変更
- `status` フィールド → `state` に変更
- `'in-use'` → `'in_use'` に変更
- `attached_vm_id` フィールド削除（バックエンドに存在しない）
- `ResizeVolumeRequest.size_gb` → `new_size_gb` に変更
- 作成・削除・リサイズのレスポンス型を `JobResponse { job_id: string }` に変更
- resize の HTTP メソッドを `api.put` → `api.post` に変更

### Task S048-2-2: `web/src/pages/tenant/VolumesPage.tsx` 完全書き直し

- `Volume.status` → `Volume.state` 参照に全修正
- `'in-use'` → `'in_use'` に修正
- `vol.attached_vm_id` 列を削除、ボリュームタイプ名列を追加（名前解決付き）
- `ResizeVolumeRequest` を `{ new_size_gb }` に修正
- リサイズ入力の `min` 属性をHTML制約から除去し、JS バリデーションで制御（テスト要件対応）
- 全 `data-testid` 属性を仕様通りに付与

### 副次修正: `web/src/components/ErrorMessage.tsx`

- `data-testid` prop を追加（デフォルト `"error-message"`）
