# S023-0 実装ログ — cirrus-sim ライブマイグレーション対応

## 実装サマリー

### Task S023-0-1: sim に migrate エンドポイント追加

`test/sim/libvirt/internal/handler/management.go` に以下を追加:

- `MigrateDomainRequest` 構造体 (`dest_host_id` フィールド)
- `POST /sim/hosts/{host_id}/domains/{uuid}/migrate` ルート登録
- `handleMigrateDomain` ハンドラ: store の `MigratePrepare` → `MigratePerform` → `MigrateFinish` → `MigrateConfirm` を順次呼び出し
- エラー処理: 404 Not Found / 409 Conflict / 500 Internal Server Error を適切に返却
- 成功時: 200 OK + 移行先ホストの domain 情報 JSON

### Task S023-0-2: Driver インターフェースに MigrateVM 追加

`internal/hypervisor/driver.go`:
- `MigrateVM(ctx context.Context, vmName string, destHostID string) error` をインターフェースに追加

`internal/hypervisor/libvirt.go`:
- `LibvirtDriver.MigrateVM` を実装: `lookupDomainUUID` でUUID取得後、`POST {baseURL}/sim/hosts/{hostID}/domains/{uuid}/migrate` を呼ぶ

### Task S023-0-3: 共有 fault.Engine を cirrus-sim に配線

`test/sim/common/app.go`:
- `NewWithFaultEngine(port, fe, logger)` 関数を追加。`New` は内部でこれを呼ぶ

`test/sim/libvirt/app.go`:
- `Server.SetFaultEngine(e *fault.Engine)` メソッドを追加

`cmd/cirrus-sim/main.go`:
- `sharedFaultEngine := fault.New()` を1つ作成
- `libvirtSim.SetFaultEngine(sharedFaultEngine)` で libvirt sim に渡す
- `common.NewWithFaultEngine(...)` で common sim にも同じ engine を渡す

これにより、common-sim の REST API で登録したフォルトルールが libvirt-sim の RPC レベルにも適用される。

## レビュー対応 (post-implementation)

### 修正: strings.Contains → errors.Is（指摘1）

`handleMigrateDomain` 内の `MigratePrepare` エラー判定を修正。

**変更前:**
```go
if strings.Contains(err.Error(), "not found") {
    m.writeError(w, http.StatusNotFound, err.Error())
```

**変更後:**
```go
if errors.Is(err, state.ErrHostNotFound) {
    m.writeError(w, http.StatusNotFound, err.Error())
```

`state.ErrHostNotFound` は `MigratePrepare` が `GetHost` 失敗時に `%w` でラップして返すため、`errors.Is` による unwrap が正しく機能する。あわせて `"errors"` をインポートに追加。

`handleCreateHost` の `strings.Contains(err.Error(), "already")` は S023-0 実装前から存在する pre-existing コードのため今回は修正しない（バックログ候補として記録）。

### 確認: 指摘2・指摘3 は pre-existing

`git diff HEAD` で確認した結果、以下は S023-0 の新規コードには含まれない:

- `RebootVM` 実装 / `handleReset` での netns 残留 / `HostInstance.Start()` 非対称性 — 指摘2 は全て pre-existing
- `GetHostInfo`, `ListVMs Name`, DB 永続化, 重複関数, インデント等 — 指摘3 も S023-0 差分に含まれない

## テスト結果（レビュー対応後）

```
make build: SUCCESS
make test: 全パッケージ PASS
- test/sim/libvirt/internal/handler: 0.005s PASS
- 全その他パッケージ: PASS (cached)
```

## バックログ候補（pre-existing な問題）

以下は S023-0 実装前から存在する技術的負債。今スプリントでは修正せず、将来のリファクタリングスプリントで対応する:

1. **handleCreateHost の strings.Contains** (`management.go:123`): `strings.Contains(err.Error(), "already")` を `errors.Is(err, state.ErrHostExists)` または `errors.Is(err, state.ErrPortInUse)` に置き換えるべき。
2. **RebootVM の実装**: `Stop` + `Start` の組み合わせで実装されており、atomic でない。libvirt `virDomainReboot` 相当の操作が必要。
3. **handleReset での netns 残留**: `POST /sim/reset` で `store.Reset()` するが、既存 VM の network namespace (`/var/run/netns/vm-{uuid}`) が残留する可能性がある。
4. **HostInstance.Start() の非対称性**: `Start` でホスト登録するが `Stop` でホスト登録解除しない。再起動後に重複登録を試みる可能性がある。
5. **sync.RWMutex を含む Host 構造体の値コピー** (`state/db.go:91,186`): `go vet` 警告。ポインタ渡しに統一すべき。

## スコープ外で気づいた問題

- `handleMigrateDomain` は現状アトミックではない (Prepare 後に Perform が失敗した場合のロールバック処理なし)。本番 libvirt の migrate は非同期/複雑だが、sim 用途では許容範囲と判断
