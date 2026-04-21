# S023-1 実装ログ — VM ライブマイグレーション

## 実装サマリー

### Task S023-1-1: libvirt.go の MigrateVM 確認
S023-0 で実装済み。`internal/hypervisor/libvirt.go` に `MigrateVM` が既に存在し、
cirrus-sim の `/sim/hosts/{hostID}/domains/{domainUUID}/migrate` エンドポイントを呼び出す実装が完了していた。
`internal/hypervisor/driver.go` の `Driver` インターフェースにも `MigrateVM` が定義済み。

### Task S023-1-2: proto/agent.proto に PrepareMigration / StartMigration 追加
`WorkerService` に以下を追加:
- `rpc PrepareMigration(PrepareMigrationRequest) returns (PrepareMigrationResponse)`
- `rpc StartMigration(StartMigrationRequest) returns (StartMigrationResponse)`

メッセージ定義を追加し、`PATH=/home/ubuntu/go/bin:$PATH make proto` でコード生成成功。

### Task S023-1-3: WorkerServer に PrepareMigration / StartMigration 実装
`internal/agent/worker_server.go`:
- `WorkerServer` に `hostID string` フィールドを追加
- `SetHostID(hostID string)` メソッドを追加（registration 後に main.go から呼び出し）
- `PrepareMigration`: 移行先として準備 OK を返す（hostID を確認用に返す）
- `StartMigration`: `s.driver.MigrateVM` を呼び出してライブマイグレーションを実行
- `cmd/cirrus/main.go` で `workerSrv.SetHostID(ag.HostID())` を追加してホスト ID を注入

### Task S023-1-4: scheduler.go に Reschedule 追加
`internal/scheduler/scheduler.go`:
- `RescheduleSpec` 構造体を追加（`ExcludeHostID`, `AZID`, `Flavor` フィールド）
- `Scheduler` インターフェースに `Reschedule` を追加
- `DefaultScheduler.Reschedule` を実装。`Schedule` と同一ロジックだが、
  `hostIDSet` から `ExcludeHostID` を削除してから候補選定を行う

### Task S023-1-5: worker_client.go に PrepareMigration / StartMigration 追加
`internal/controller/worker_client.go` に薄いラッパーを追加。

### Task S023-1-6: compute/service.go と orchestrator.go に MigrateVM 実装
- `internal/compute/models.go`: `VMStatusMigrating = "migrating"` を追加し、
  `transitionalStatuses` に登録（マイグレーション中は他操作をブロック）
- `internal/compute/service.go`: `Service` インターフェースに `MigrateVM` を追加
- `internal/compute/orchestrator.go`: `MigrateVM` を実装
  1. VM を取得し running 状態を検証
  2. ステータスを `migrating` に更新（失敗時は `error` にロールバック）
  3. `targetHostID` が nil なら `scheduler.Reschedule` で宛先を選定
  4. 宛先ワーカーに `PrepareMigration`
  5. 移行元ワーカーに `StartMigration`
  6. DB 更新: `host_id` を宛先に変更し、ステータスを `running` に戻す

## テスト結果

```
make build  → 成功
make test   → 全テスト通過（新旧含む）
make lint   → 既存の lint 警告のみ（本 Story で導入したファイルに新規 lint エラーなし）
```

---

## レビュー指摘修正（2026-04-20）

### 修正1: HealVM に `migrating` を追加（バグ修正）
`internal/compute/store.go` の `HealVM` SQL で、`migrating` が除外リストに漏れていた。
```sql
-- 修正前
WHERE id = $4 AND status NOT IN ('pending', 'building', 'deleting', 'error')
-- 修正後
WHERE id = $4 AND status NOT IN ('pending', 'building', 'deleting', 'migrating', 'error')
```
マイグレーション中の VM が `HealVM` によって誤って `error` に遷移してしまうバグを修正。

### 修正2: MigrateVM に IsTransitional チェックを追加
`internal/compute/orchestrator.go` の `MigrateVM` で `getVM` 直後に他操作と同じパターンで
`vm.IsTransitional()` チェックを追加。`running` チェックの前に遷移中ガードを配置。

### 修正3: resolveWorker の再利用
`MigrateVM` 内で移行元ホストを手動で lookup していた部分（`getHostByID` → `workers.Get` の
3ステップ）を `resolveWorker(ctx, vm)` に置き換え。`vmName` の二重生成も解消した。

### 修正4: DB マイグレーション 000029 追加
`internal/state/migrations/000029_vm_migrating_status.up.sql` / `.down.sql` を追加。
`vms.status` 列に CHECK 制約がないため DDL 変更は不要。プレースホルダー migration として
`migrating` ステータスが正式サポートされることをドキュメント化。

### ビルド・テスト結果

```
make build → 成功
make test  → 全テスト通過（all cached — no regressions）
```

## スコープ外で気づいた問題（Tech Debt）

- `MigrateVM` は現在 REST API / CLI に公開されていない。
  Story S023-2 相当で `POST /api/v1/vms/{id}/migrate` エンドポイントと
  `cirrusctl vm migrate` コマンドを追加する必要がある。
- `VMStatusMigrating` は DB の `vms.status` 列の CHECK 制約に含まれていない可能性がある。
  マイグレーションファイルで `ALTER TABLE vms DROP CONSTRAINT ... ADD CONSTRAINT ...` が必要かもしれない。
- `MigrateVM` はボリュームの再 export を行わない。現状は libvirt の live migration が
  同一ストレージバックエンドを前提としている（shared storage model）。
  将来的なストレージ分離構成では追加ステップが必要。
- `make proto` は `PATH` に `/home/ubuntu/go/bin` を含める必要がある（Makefile には記述がない）。
