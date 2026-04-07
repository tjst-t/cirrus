# Sprint S044 — 内部 LB 実装ログ

## 概要

テナント Network 内部で VIP による L4 負荷分散ができるようになった。
外部 Ingress LB (S043) と異なり、GW ノード不要。全ホストの OVS に分散インストール。

## 実装内容

### 1. DB マイグレーション
- `internal/state/migrations/000026_load_balancers.up.sql`
- `internal/state/migrations/000026_load_balancers.down.sql`
- `load_balancers` テーブル: VIP は INET 型、UNIQUE(network_id, vip)、UNIQUE(network_id, name)
- `lb_backend_health` テーブル: lb_id + vm_id の複合 PK

### 2. モデル
- `internal/network/models.go` に `LoadBalancer`, `LoadBalancerSpec` を追加
- VIP はフィールドとして保持するが、作成時は自動割り当て

### 3. IPAM
- `internal/network/ipam.go` に `AllocateVIP()` を追加
- ネットワーク CIDR 内の既存 IP（ポート + 既存 VIP）を除いた最初の利用可能 IP を割り当て

### 4. サービスインターフェース
- `internal/network/service.go` に 5 メソッド追加:
  `CreateLoadBalancer`, `GetLoadBalancer`, `ListLoadBalancers`, `DeleteLoadBalancer`, `UpdateLBBackendHealth`

### 5. ストア実装
- `internal/network/store.go` に上記メソッドの PostgreSQL 実装を追加
- バックエンド IP は vm_id から自動解決（IngressStore と同パターン）

### 6. Proto
- `proto/network.proto` に `InternalLBRule` メッセージを追加
- `HostNetworkState` に `repeated InternalLBRule internal_lb_rules = 8` を追加
- `make proto` で再生成済み

### 7. StateController
- `internal/network/controller.go`:
  - `computeInternalLBRules(ctx, networkIDs)` を追加
  - `ComputeHostNetworkState` でネットワークIDが存在する全ホストに対して計算・注入
  - GW フィルタなし（`if gw != nil` ブロック外で呼び出し）

### 8. OVS エージェント
- `internal/network/agent/ovs.go`:
  - `Pipeline` に `internalLBGroups`, `internalLBFlows` マップを追加
  - `applyInternalLBRules()` を追加（全ホストで呼び出し、GW チェックなし）
  - `buildInternalLBGroupSpec()` — external L4LB と同じ構造
  - `internalLBGroupID()` — FNV-1a XOR `0x80000000` で外部 LB との衝突回避
  - `Apply()` のステップ 7 として常に実行

### 9. Identity / RBAC
- `internal/identity/authorizer.go`:
  - `ActionCreateLoadBalancer`, `ActionListLoadBalancers`, `ActionGetLoadBalancer`, `ActionDeleteLoadBalancer` を追加
  - `tenant_admin` に全 LB アクションを付与
  - `tenant_member` に読み取りアクションを付与

### 10. API ハンドラ
- `internal/api/lb_handler.go` — 新規ファイル (ingress_handler.go パターンに倣う)
- `internal/api/router.go` に 4 ルートを追加:
  - `POST /api/v1/tenants/{tenant_id}/networks/{network_id}/load-balancers`
  - `GET /api/v1/tenants/{tenant_id}/networks/{network_id}/load-balancers`
  - `GET /api/v1/tenants/{tenant_id}/networks/{network_id}/load-balancers/{lb_id}`
  - `DELETE /api/v1/tenants/{tenant_id}/networks/{network_id}/load-balancers/{lb_id}`

### 11. クライアント
- `internal/client/gateway.go` に 5 メソッドを追加:
  `CreateLoadBalancer`, `ListLoadBalancers`, `GetLoadBalancer`, `DeleteLoadBalancer`, `ResolveLoadBalancer`

### 12. CLI
- `cmd/cirrusctl/main.go` に `load-balancer` サブコマンドを追加:
  - `cirrusctl load-balancer list --network <id/name> --tenant <id/name>`
  - `cirrusctl load-balancer create --network ... --name ... --port ... --backend ... [--session-affinity source_ip]`
  - `cirrusctl load-balancer show <id/name> --network ... --tenant ...`
  - `cirrusctl load-balancer delete <id/name> --network ... --tenant ...`

## 設計上の判断

1. **VIP 割り当て**: ポート IP と既存 VIP を合わせた使用済みセットから次の利用可能 IP を選択。ポートは /30 ブロック単位で割り当てるため VIP と重複することは理論上ないが、安全のため確認する。

2. **Group ID 衝突回避**: 外部 L4LB は FNV-1a(ingress_id) を直接使用。内部 LB は FNV-1a(lb_id) XOR 0x80000000 を使用し、名前空間を分離。

3. **全ホスト適用**: `ComputeHostNetworkState` 内で networkIDs が空でない場合（ローカルポートがある全ホスト）に `computeInternalLBRules` を呼び出す。GW ノードでも非 GW ノードでも同様に適用される。

4. **HealthCheck**: 現スプリントでは lbRoute 作成時に `lb_backend_health` に全バックエンドを初期 `healthy=true` で登録。`UpdateLBBackendHealth` API は将来のヘルスチェックエージェントが使用する。

## ビルド結果

```
$ make build
go build -o bin/cirrus ./cmd/cirrus/
go build -o bin/cirrusctl ./cmd/cirrusctl/
```

ビルド成功（エラーなし）。

## テスト結果

```
$ make test
ok  github.com/tjst-t/cirrus/internal/network/agent  0.007s
ok  github.com/tjst-t/cirrus/internal/network         0.008s
ok  github.com/tjst-t/cirrus/internal/api             0.010s
ok  github.com/tjst-t/cirrus/internal/identity        0.005s
... (全パッケージ PASS)
```

新規追加テスト（`internal/network/agent/l4lb_test.go`）:
- `TestInternalLBDistribution` — select group と steering flow のインストール確認
- `TestInternalLBSessionAffinity` — source_ip affinity で selection_method=ip_src 設定確認
- `TestInternalLBNoGatewayRequired` — GatewayInfo なし（非 GW ホスト）でも流量インストール確認
- `TestInternalLBStaleFlowCleanup` — LB 削除時に group と flow が削除されることを確認

全テスト PASS。
