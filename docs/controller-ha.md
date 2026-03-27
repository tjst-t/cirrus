# Controller HA（高可用性）

## 概要

Cirrus Controllerは現在シングルインスタンスで動作するが、本番環境ではSPOF（単一障害点）を排除するためActive/Active構成の複数インスタンスで運用する。

PostgreSQLも同様にHA構成が前提となる。ただしDB HAはCirrus外部のインフラ層で提供される想定であり、Cirrusとしては接続障害への対応を設計する。

## 設計方針

### Active/Active Controller

全controllerインスタンスが同時にAPI/gRPC/Heartbeat処理を受け付ける。性能ネック時にインスタンスを追加するだけで水平スケールできる。

シングルトンジョブ（1台だけが実行すべき処理）はPostgreSQLアドバイザリーロックによるリーダー選出で排他制御する。

```
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ Controller-1 │  │ Controller-2 │  │ Controller-3 │
│  API ✓       │  │  API ✓       │  │  API ✓       │
│  gRPC ✓      │  │  gRPC ✓      │  │  gRPC ✓      │
│  Heartbeat ✓ │  │  Heartbeat ✓ │  │  Heartbeat ✓ │
│  Leader: ✓   │  │  Leader: ✗   │  │  Leader: ✗   │
│  (Monitor)   │  │              │  │              │
│  (Reconcile) │  │              │  │              │
│  (Scheduler) │  │              │  │              │
└──────┬───────┘  └──────┬───────┘  └──────┬───────┘
       │                 │                 │
       └────────────┬────┴────────────┬────┘
                    │                 │
              ┌─────┴─────┐   ┌──────┴──────┐
              │ PostgreSQL │   │ PostgreSQL  │
              │  Primary   │──▶│  Standby    │
              └───────────┘   └─────────────┘
```

### 処理の分類

| 処理 | 方式 | 理由 |
|------|------|------|
| API（REST） | 全インスタンスで処理 | ステートレス。ロードバランサで分散 |
| gRPC Heartbeat受信 | 全インスタンスで処理 | ステートレス。DB更新のみ |
| gRPC RegisterHost | 全インスタンスで処理 | DBのON CONFLICTで冪等 |
| HeartbeatMonitor | リーダーのみ | last_heartbeatの定期スキャン。重複実行は不正なfaulty遷移を招く |
| ReconcileLoop | リーダーのみ | OVN/Storage問い合わせ。重複実行はDriftEvent重複を招く |
| HostFaultyHandler | リーダーのみ | カスケード状態更新。重複実行は不要 |
| Scheduler | 分散ロック | 配置決定時にホストリソースを `SELECT FOR UPDATE` で排他。複数インスタンスが同時にスケジュールしてもリソース競合しない |
| DRS | リーダーのみ | 周期実行。重複実行はマイグレーション計画の競合を招く |

### リーダー選出

PostgreSQLのアドバイザリーロック（`pg_try_advisory_lock`）を使用する。外部インフラ（etcd等）を追加する必要がない。

```go
// リーダー選出（起動時、定期更新）
func (c *Controller) tryBecomeLeader(ctx context.Context) (bool, error) {
    var acquired bool
    err := c.pool.QueryRow(ctx,
        "SELECT pg_try_advisory_lock($1)", leaderLockID,
    ).Scan(&acquired)
    return acquired, err
}
// ロックはセッション（DB接続）に紐づく。
// 接続が切れるとロックは自動解放され、他のインスタンスが取得できる。
```

- リーダーはシングルトンジョブを実行する
- リーダーが停止すると、DB接続切断でロックが解放される
- 他のインスタンスが周期的に`pg_try_advisory_lock`を試行し、ロックを取得した者が新リーダーになる
- リーダー切り替え時間 = ロック解放 + 試行間隔（デフォルト5秒）

### Worker接続

workerは `--controller` フラグでcontrollerのgRPCアドレスを指定する。HA構成では:

- **ロードバランサ経由**: workerは単一のVIP/DNSを指定。L4ロードバランサが背後のcontrollerに分散
- **gRPC client-side LB**: 複数アドレスをカンマ区切りで指定し、gRPCのラウンドロビンで分散（将来の拡張）

Phase 1ではロードバランサ方式を前提とする。

## PostgreSQL HA

### Cirrusの前提

PostgreSQL HAはCirrus外部のインフラ層で提供される:

- **マネージドDB**: AWS RDS, Cloud SQL等 — 自動フェイルオーバー
- **Patroni + PostgreSQL**: 自前構築の場合 — HAProxy経由のVIP

Cirrus controllerは接続先として単一のエンドポイント（VIP or マネージドDBエンドポイント）を使用し、背後のフェイルオーバーを意識しない。

### Cirrus側の対応

| 項目 | 設計 |
|------|------|
| 接続断時のリトライ | pgxpoolの自動リコネクト。追加設定不要 |
| フェイルオーバー時の一時エラー | API/gRPCは一時的に503を返し、クライアントがリトライ |
| リードレプリカ活用 | 将来の最適化。リスト系クエリをリードレプリカに振り分け |
| マイグレーション実行 | 単一インスタンスが実行（`golang-migrate`のロック機能で排他） |

### 接続設定

```
# Primary（読み書き）
DB_DSN=postgres://cirrus:xxx@db-primary:5432/cirrus?sslmode=require

# Read replica（将来、リスト系クエリの負荷分散用）
DB_READ_DSN=postgres://cirrus:xxx@db-replica:5432/cirrus?sslmode=require
```

## Controller停止時の影響（HA構成）

| 障害 | 影響 | 復旧 |
|------|------|------|
| 1台のcontroller停止 | ロードバランサがトラフィックを他へ振り分け。リーダーなら数秒後に別インスタンスが引き継ぎ | 自動 |
| 全controller停止 | API/gRPC停止。既存VM稼働継続（worker自律）。ホスト障害検出不可 | controller再起動で復旧 |
| PostgreSQL Primary停止 | 全controller一時停止。マネージドDB/Patroniが数秒〜数十秒でフェイルオーバー | 自動（DB HA依存） |
| PostgreSQL完全停止 | 全controller停止。既存VM稼働継続 | DB復旧で復旧 |

## 設定パラメータ

| パラメータ | デフォルト | 説明 |
|-----------|-----------|------|
| `leader-election-interval` | 5s | リーダー選出の試行間隔 |
| `leader-lock-id` | 1 | アドバイザリーロックのID |
