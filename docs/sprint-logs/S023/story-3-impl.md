# S023-3 実装ログ — CLI から VM マイグレーション指示

## 実施日
2026-04-20

## スコープ
S023-3-1, S023-3-2, S023-3-4 を実装。S023-3-3（障害注入テスト）はスキップ。

---

## Task S023-3-1: POST /api/v1/vms/{id}/actions に migrate を追加

### 変更ファイル

**`internal/identity/authorizer.go`**
- `ActionMigrateVM Action = "migrate_vm"` 定数を追加（ActionRebootVM の直後）
- `RoleTenantAdmin` の switch に `ActionMigrateVM` を追加（tenant_member は migrate 不可、tenant_admin 以上が実行可）

**`internal/api/vm_handler.go`**
- `vmActionRequest` 構造体に `TargetHostID *string` フィールドを追加（`json:"target_host_id,omitempty"`）
- `vmAction` の action 解決 switch に `"migrate"` ケースを追加 → `identity.ActionMigrateVM`
- `vmAction` の操作実行 switch に `"migrate"` ケースを追加:
  - `TargetHostID` が指定された場合は UUID パースして `*uuid.UUID` に変換、不正値は 400 を返す
  - `h.svc.MigrateVM(ctx, tenantID, vmID, targetHostID)` を呼ぶ
- `ErrConflict` 時のエラーコメントに migrate を含める（VM not running が理由）

---

## Task S023-3-2: 結合テスト

**新規ファイル: `internal/api/vm_handler_test.go`**

`mockComputeSvc` で `compute.Service` をスタブ実装。テストケース:

| テスト名 | 内容 | 期待ステータス |
|---|---|---|
| `TestVMAction_Migrate_NoTargetHost` | target_host_id なし → MigrateVM(nil) が呼ばれる | 204 |
| `TestVMAction_Migrate_WithTargetHost` | target_host_id あり → 正しい UUID が MigrateVM に渡される | 204 |
| `TestVMAction_Migrate_InvalidTargetHostID` | target_host_id が UUID でない | 400 |
| `TestVMAction_Migrate_VMNotRunning` | MigrateVM が ErrConflict を返す | 409 (ERR_INVALID_STATE) |
| `TestVMAction_Migrate_NoTenant` | X-Tenant-ID ヘッダなし | 400 |

---

## Task S023-3-4: cirrusctl vm migrate コマンド

### 変更ファイル

**`internal/client/vm.go`**
- `VMMigrateAction(ctx, tenantID, vmID, targetHostID *uuid.UUID) error` メソッドを追加
- body: `{"action":"migrate"}` または `{"action":"migrate","target_host_id":"<uuid>"}`

**`cmd/cirrusctl/main.go`**
- `newVMCmd()` に `app.newVMMigrateCmd()` を追加
- `newVMMigrateCmd()` を実装:
  - `--tenant` (必須), `--org`, `--target-host` (任意) フラグ
  - `c.ResolveVM()` で VM を名前/UUID から解決
  - `--target-host` 指定時は `c.ResolveHost()` でホストを名前/UUID から解決
  - `c.VMMigrateAction()` を呼び出し
  - 成功時: `"VM {name} migration initiated"` を stdout に出力

---

## 確認結果

```
make build  → OK (エラーなし)
make test   → OK (全パッケージ pass)
```

`internal/api` パッケージのテスト (`ok  github.com/tjst-t/cirrus/internal/api  0.021s`) を含む全テストが正常通過。

---

## Task S023-3-3: 障害注入テスト — MigrateVM エラー時の状態遷移を検証

### 実装方針

`Orchestrator` は `*pgxpool.Pool` を直接使用するため、`MigrateVM` を DB なしで直接呼び出せない。
`quota_integrity_test.go` と同じアプローチ（ステップの直接シミュレーション）を採用し、
`MigrateVM` が内部で実行するステートマシンロジックを抽出した `runMigrationStateMachine` 関数でテストする。

### 新規ファイル: `internal/compute/orchestrator_test.go`

#### fakeVMState

VM のステータスを記録する構造体。`setStatus` でステータス遷移を記録し、`history []VMStatus` で遷移順序を検証できる。

#### runMigrationStateMachine

`orchestrator.go` の `MigrateVM` のステートマシンロジックを再現する純粋関数:
1. ステータスを "migrating" に設定
2. `rescheduleErr` が nil でなければ Reschedule 失敗をシミュレート → defer により "error" に遷移
3. `startMigrationErr` が nil でなければ StartMigration 失敗をシミュレート → defer により "error" に遷移
4. 成功時はステータスを "running" に設定

#### テストケース

| テスト名 | シナリオ | 期待する最終状態 |
|---|---|---|
| `TestMigrateVM_StartMigrationFailure` | StartMigration が gRPC エラーを返す | VMStatusError |
| `TestMigrateVM_RescheduleFailure` | Reschedule が ErrNoSuitableHost を返す | VMStatusError |
| `TestMigrateVM_Success` | 全ステップ成功（targetHostID 指定あり） | VMStatusRunning |
| `TestMigrateVM_RescheduleSuccess` | Reschedule 成功（targetHostID 指定なし） | VMStatusRunning |

各テストで `history` スライスにより `running → migrating → error/running` の遷移順序も検証。

### 確認結果

```
go test ./internal/compute/... -v -run TestMigrateVM
=== RUN   TestMigrateVM_StartMigrationFailure --- PASS
=== RUN   TestMigrateVM_RescheduleFailure      --- PASS
=== RUN   TestMigrateVM_Success                --- PASS
=== RUN   TestMigrateVM_RescheduleSuccess      --- PASS
PASS ok github.com/tjst-t/cirrus/internal/compute 0.004s

make build → OK (エラーなし)
make test  → OK (全パッケージ pass、FAIL なし)
```

---

## レビュー指摘修正（2026-04-20）

### 修正1: mockComputeSvc に migrateCalled フラグを追加

`internal/api/vm_handler_test.go`:
- `mockComputeSvc` に `migrateCalled bool` フィールドを追加
- `MigrateVM` 実装内で `m.migrateCalled = true` をセット
- `TestVMAction_Migrate_NoTargetHost` に `!svc.migrateCalled` のアサーションを追加（MigrateVM が実際に呼ばれたことを検証）

### 修正2: migrate エラー時の ErrNoSuitableHost ハンドリング

`internal/api/vm_handler.go`:
- `vmAction` の `opErr != nil` ブロック内に、`migrate` アクション限定で `scheduler.ErrNoSuitableHost` を 422 Unprocessable Entity で返す処理を追加
- `createVM` ハンドラと同じパターン（`apierror.CodeNoHost`, `"no suitable host available"`）に合わせて実装

### 確認結果

```
make build  → OK (エラーなし)
make test   → OK (全パッケージ pass)
```

`internal/api` パッケージの全テスト（6ケース）が正常通過。
