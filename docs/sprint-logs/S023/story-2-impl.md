# S023-2 実装ログ: ゼロパケットロス移行 (FallbackRoute)

## 目的

VM ライブマイグレーション中にネットワーク断が起きないよう、移行元ホストがトラフィックを移行先ホストへトンネル転送する仕組み (FallbackRoute) を実装した。

## 実装内容

### Task S023-2-1: proto/network.proto に FallbackRoute 追加

- `FallbackRoute` メッセージを追加 (field: port_id, dest_host_ip, dest_vni)
- `HostNetworkState` に `repeated FallbackRoute fallback_routes = 9` を追加
- `make proto` でコード生成 → `proto/networkpb/network.pb.go` 更新

### Task S023-2-2: flow.go に FallbackRoute フロー生成追加

- `FlowContext` に `FallbackRoutes []*pb.FallbackRoute` フィールド追加
- `generateFallbackRouteFlows()` 関数を追加
  - TableDstHostResolution (table 4) に priority=200 のフローを挿入
  - 通常の local 出力フロー (priority=100) より高優先度でトンネル転送を行う
  - マッチ: 移行中 VM の IP 宛トラフィック
  - アクション: tun_dst/tun_id 設定 → TableGeneveEncap へ resubmit
- `GenerateFlows()` に FallbackRoute フロー生成を追加

### Task S023-2-3: ovs.go (Pipeline.Apply) の更新

- `FlowContext` に `FallbackRoutes` を設定
- `ensureFallbackTunnels()` メソッドを追加: FallbackRoute の宛先ホストへの Geneve トンネルポートを自動作成
- ログに `fallback_routes` カウントを追加
- FallbackRoute の差分検出は既存の `DiffFlows` が自動的に処理 (flow key = table/priority/match)

### Task S023-2-3b: state.go (StateCache) の更新

- `fallbackRoutes map[string]*pb.FallbackRoute` フィールドを追加
- `ApplyFull` / `ApplyDelta`: FallbackRoutes を適切に取り込む
- `Snapshot()`: FallbackRoutes を返す HostNetworkState に含める

### Task S023-2-4: DB マイグレーション + StateController の更新

- `000030_migration_fallback_routes.up.sql`: `migration_fallback_routes` テーブル作成
  - カラム: id, port_id (FK ports), src_host_id (FK hosts), dest_host_id (FK hosts), created_at
  - インデックス: src_host_id, port_id
- `internal/network/controller.go` に `getFallbackRoutes()` メソッド追加
  - src_host_id でフィルタし、port の VNI・dest host の fabric_ip と JOIN して返す
- `ComputeHostNetworkState()` に getFallbackRoutes 呼び出しを追加
- `grpc.go` の `stateEqual()` に FallbackRoutes の比較を追加 (変化検出)

### Task S023-2-5: MigrateVM に FallbackRoute 制御統合

`internal/compute/orchestrator.go`:
1. PrepareMigration 前: VM のポートを取得
2. PrepareMigration 後: `insertFallbackRoute()` で DB にレコード挿入 → src host の次の poll で FallbackRoute が配信される
3. 簡略化 ACK 待ち: `time.Sleep(3 * time.Second)` で controller の poll (2s) が確実に走るのを待つ
4. StartMigration 実行
5. DB の host_id を dest host に更新 → 全ホストの RemotePort が自動更新
6. `defer deleteFallbackRoute()` が実行され src host の Fallback フローが次の poll で削除される

`internal/compute/store.go`:
- `insertFallbackRoute()`: migration_fallback_routes にレコード挿入
- `deleteFallbackRoute()`: ID でレコード削除

## 注意事項 (スコープ外)

- ACK 待ち (タイムアウト 30 秒) は実装しなかった。代わりに `time.Sleep(3s)` で代替。本番実装は別スプリント。
- FallbackRoute は port の IP を localPorts から引く設計。IP が見つからない場合 (ポートが既に移動済み) はスキップ。

## 結果

- `go build ./...` → エラーなし
- `go test ./internal/network/... ./internal/network/agent/... ./internal/compute/...` → 全 PASS
- lint: 既存の pre-existing 警告のみ (新規警告なし)

---

## レビュー指摘修正 (自動修正)

### 修正1: トンネルポートのリーク (バグ)

**問題**: `ensureFallbackTunnels()` が FallbackRoute 宛先ホストのトンネルポートを追加するが、
FallbackRoute が不要になった後もそれらのポートが削除されない。
`ensureTunnelPorts()` の `neededHosts` が `remotePorts` のみから構築されているため、
`ensureFallbackTunnels()` で追加したポートは cleanup 対象に含まれなかった。

**修正**:
- `ensureFallbackTunnels()` メソッドを削除
- `ensureTunnelPorts(remotePorts []*pb.RemotePort, fallbackRoutes []*pb.FallbackRoute)` にシグネチャ変更
- `neededHosts` を `remotePorts` と `fallbackRoutes` の両方から構築するよう修正
- `Apply()` の呼び出し元を `p.ensureTunnelPorts(state.RemotePorts, state.FallbackRoutes)` に更新

**ファイル**: `internal/network/agent/ovs.go`

### 修正2: stateEqual の FallbackRoute 比較キー (バグ)

**問題**: `stateEqual()` の FallbackRoute 比較が `port_id` のみだった。
同一ポートに対して移行先ホストが変更された場合 (再マイグレーション等) に変化を検出できない。

**修正**: マップキーを `fr.PortId` から `fr.PortId+"|"+fr.DestHostIp` の複合キーに変更。

**ファイル**: `internal/network/grpc.go`

### 確認結果

- `make build` → 成功
- `make test` → 全 PASS (cached 含む)
