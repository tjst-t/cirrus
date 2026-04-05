# Sprint S021 — Story 3: Reconcile 結合テスト 実装ログ

## 変更・作成ファイル一覧

### 変更

- `internal/controller/reconcile/network.go`
  - `OVSFlowVerifier` インターフェース追加（exported）
  - `DriftTypeFlowMissing = "flow_missing"` 定数追加
  - `NetworkReconciler` に `flowVerifier OVSFlowVerifier` フィールド追加
  - `WithOVSFlowVerifier(v OVSFlowVerifier)` メソッド追加（builder パターン）
  - `reconcileOnce` 内に OVS フロー検証ステップ追加

### 新規作成

- `test/integration/reconcile_heartbeat_test.go` — Task S021-3-1
- `test/integration/reconcile_network_test.go` — Task S021-3-2
- `test/integration/reconcile_host_faulty_test.go` — Task S021-3-3

---

## 各タスクの実装概要

### S021-3-1: HeartbeatReconciler DriftEvent 結合テスト

**テスト**: `TestReconcileHeartbeat_DriftEvent`

流れ:
1. `CIRRUS_ENDPOINT` / `CIRRUS_TOKEN` / `CIRRUS_TENANT_ID` でコントローラに接続
2. VM 作成 → running 待ち（最大 60 秒）
3. DB から `host_id` を取得
4. `POST /sim/hosts/{host_id}/domains/{uuid}/destroy` で cirrus-sim から VM を強制停止
   - DB は依然 `status=running` のまま → 状態不整合を注入
5. `drift_events` テーブルを 30 秒間ポーリング
   - `resource_id=<vm_id>` かつ `type IN ('state_mismatch', 'expected_missing')` を確認
6. `layer=compute`, `detected_by=heartbeat_reconciler` をアサート
7. VM が `error` 状態に遷移するか確認（auto-heal は shutoff の場合 alert-only なので非必須）

環境変数:
- `LIBVIRT_SIM_URL` (default: `http://localhost:8100`)

### S021-3-2: OVS フロー検出 + 結合テスト

**実装変更**: `internal/controller/reconcile/network.go`

設計上の決定:
- `OVSFlowVerifier` インターフェースを `reconcile` パッケージに配置（controller 層の責任）
- builder 方式 (`WithOVSFlowVerifier`) を選択: `NewNetworkReconciler` のシグネチャを変えず後付けで注入可能
- Worker 側（`internal/network/agent/`）への変更は不要: sim 環境では mock を使う

**テスト**: `TestReconcileNetwork_FlowMissing`

流れ:
1. DB に直接接続（`TEST_DB_DSN`）
2. `mockOVSFlowVerifier` を作成し、アクティブなホストに flow-missing 条件を設定
3. `NetworkReconciler.Run()` を短期コンテキストで起動（最初のパス完了まで待機）
4. `drift_events` テーブルで `type=flow_missing`, `layer=network`, `detected_by=network_reconciler` を確認

mock の設計:
- `failHosts map[uuid.UUID]string` でホストごとに故障を制御
- `SetFlowMissing` / `ClearFlowMissing` で状態変更
- `reconcile.OVSFlowVerifier` インターフェース準拠を `var _ = (*mockOVSFlowVerifier)(nil)` でコンパイル時検証

### S021-3-3: HostFaultyHandler カスケード結合テスト

**テスト**: `TestReconcileHostFaulty_Cascade`, `TestReconcileHostFaulty_NoActiveVMs`

流れ:
1. VM 作成 → running 待ち（最大 60 秒）
2. DB から `host_id` 取得
3. `UPDATE hosts SET operational_state = 'faulty'` で HeartbeatMonitor を迂回
4. `controller.NewHostFaultyHandler(pool, logger).Handle(ctx, hostID)` を直接呼び出し
5. `vms.status = 'error'` をアサート
6. `ports.status = 'down'` をアサート（ポートが存在する場合）

cleanup で `operational_state` を `active` に戻すことで他テストへの影響を防ぐ。

`TestReconcileHostFaulty_NoActiveVMs` は存在しないホスト UUID を渡してもパニックしないことを確認する回帰テスト。

---

## OVSFlowVerifier の設計決定

| 選択肢 | 採用 | 理由 |
|--------|------|------|
| `reconcile` パッケージにインターフェース定義 | ✅ | controller 層の関心事; worker/agent に依存しない |
| `network/agent` パッケージに実装を置く | 将来作業 | 本実装は sim 環境対象; 実 OVS 実装は Worker 統合時 |
| constructor で必須引数化 | ❌ | 後方互換性を壊す; nil=無効化で十分 |
| `WithOVSFlowVerifier` builder | ✅ | 既存コード変更なし; 省略可能 |

---

## ビルド・テスト結果

```
go build ./...                                    : OK (no output)
go build -tags integration ./test/integration/... : OK (no output)
go vet ./...                                      : OK (no output)
go vet -tags integration ./test/integration/...   : OK (no output)
go test ./...                                     : all PASS (unit tests only)
```

統合テスト自体の実行は `CIRRUS_ENDPOINT`, `CIRRUS_TOKEN`, `CIRRUS_TENANT_ID` などの
環境変数が必要なため、CI 環境での実行を前提としている。
環境変数未設定時は `t.Skip` で自動スキップされる。

---

## 気づいた点・決定した事項

1. **`host.Service` の実装**: `host.NewService` は存在せず `host.NewStore` が直接 `Service` インターフェースを実装している。
2. **HeartbeatReconciler の auto-heal 判定**: `shutoff` 状態は "intentional external stop" とみなされ `alert` のみ。`expected_missing` と `crashed` のみが `auto_heal`。テストでは auto-heal は optional アサーションとした。
3. **DriftHandler のデデュープ**: デデュープ TTL デフォルト 10 分なので、同一ホストへの複数実行はテストごとに固有の UUID (host/vm ID) を使うことで回避。
4. **cirrus-sim の API**: VM の強制停止は `POST /sim/hosts/{host_id}/domains/{uuid}/destroy`（`handleDestroyDomain`）。`/stop` は graceful shutdown。destroy が状態不整合注入に適切。
