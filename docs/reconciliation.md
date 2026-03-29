# Reconciliation（状態整合性）

Cirrusの基本思想「宣言的トポロジモデル」に基づき、Controller DBの**desired state**と各サブシステムの**actual state**の乖離を検出・対応する仕組みを定義する。

## 原則

1. **双方向検出**: 「宣言済み・実体無し」（障害）と「未宣言・実体有り」（整合性異常）の両方を検出する
2. **パッシブ優先**: heartbeatに含まれる情報を最大限活用し、追加のポーリングは補完として使う
3. **段階的対応**: Alert → Auto-heal → Failover の3段階。初期はAlertから始め、信頼性の確認されたケースからAuto-healに昇格する
4. **各レイヤ同時実装**: Reconcileは後付けではなく、各レイヤの実装Sprintに組み込む
5. **遷移中リソースの除外**: in-flight操作中のリソースはreconcile対象外とする
6. **冪等かつ安全**: auto-healアクションはデータ損失リスクが無い操作のみ。確信が持てないケースはAlertに留める

## ドリフトの分類

### 存在ドリフト

DB上に存在するが実体が無い、または実体があるがDB上に記録が無い。

| レイヤ | DB有・実体無 | DB無・実体有 |
|--------|-------------|-------------|
| Compute | VMがDBでactiveだがlibvirtに存在しない | libvirtにVMがあるがDBに記録がない |
| Network | OVSフローが期待と異なる / HostNetworkState配信に対するフロー反映漏れ | OVSに未知のフローがある |
| Storage | ボリュームがバックエンドに存在しない | バックエンドに未知のボリュームがある |

### 属性ドリフト

存在するが、状態や属性が一致しない。

| レイヤ | ケース | 例 |
|--------|--------|-----|
| Compute | VMステータス不一致 | DB=active, libvirt=shutoff |
| Compute | リソース使用量乖離 | heartbeat報告値 vs DB期待値 |
| Network | ポートバインディング不一致 | OVSフローのトンネル宛先が期待するhost_ipと異なる |
| Network | Policyフロー不一致 | DB上のPolicyとOVSフローが不一致 |
| Storage | アタッチ状態不一致 | DB=in_use, 実際はデタッチ済み |

### 暗黙ドリフト（間接推定）

直接比較ではなく、兆候から推定する。

| 兆候 | 推定されるドリフト |
|------|-------------------|
| heartbeat途絶 | ホスト障害（全VMが停止している可能性） |
| heartbeat内のVM数 ≠ DB上のVM数 | VM状態の不整合 |
| リソース使用量の急変 | OOMやプロセスクラッシュ |

## 遷移中リソースの除外（in-flight exclusion）

VM作成・削除・マイグレーション中のリソースは、desired stateとactual stateの間に一時的な不一致が生じる。これは正常な動作であり、reconcilerはこれをドリフトとして検出してはならない。

### 遷移中状態の定義

以下のステータスのリソースはreconcile対象から除外する:

| リソース | 除外ステータス |
|---------|---------------|
| VM | `scheduling`, `building`, `deleting`, `migrating`, `stopping`, `starting` |
| Volume | `creating`, `deleting`, `migrating` |
| Port | 関連VMが上記ステータスの場合 |

### 実装方式

heartbeat reconcilerのDB問い合わせで除外ステータスをWHERE条件に含める:

```sql
SELECT id FROM vms
WHERE host_id = $1 AND status IN ('active', 'shutoff', 'error')
-- scheduling, building, deleting, migrating 等は除外
```

### スタック操作のタイムアウト

遷移中状態が長時間続く場合は別の問題（ジョブのハング等）であるため、遷移中状態にもタイムアウトを設ける:

- `building` が 10分以上 → `error` に遷移（ジョブ失敗とみなす）
- `deleting` が 10分以上 → Alert（削除処理のハング）
- `migrating` が 30分以上 → Alert（マイグレーション停滞）

これはreconcilerではなくジョブ管理の責務だが、reconcilerが検出する前にクリーンアップされるべきである。

## 検出メカニズム

### パッシブ検出（Heartbeat Reconciler）

workerから10秒間隔で送信されるheartbeatを、DBのdesired stateと比較する。追加の通信コストなしでCompute層のドリフトを検出できる。

```
heartbeat受信時:
  1. running_vms の VM IDリスト取得
  2. DB上「このホストで安定状態（active/shutoff/error）であるべき VM」のリストを取得
     ※ 遷移中ステータス（scheduling, building, deleting, migrating等）は除外
  3. 比較:
     - DBに有り・heartbeatに無し → DriftEvent(expected_missing)
     - DBに無し・heartbeatに有り → DriftEvent(unexpected_present)
     - ステータス不一致             → DriftEvent(state_mismatch)
```

**前提**: 現在のheartbeatは`running_vms`をprotoで送信しているが、controller側の`ResourceReport`モデルがVM IDリストを保持していない。Sprint 8.5で`ResourceReport`を拡張し、heartbeatハンドラがVMリストをreconcilerに渡すようにする。

**対象**: Compute層（VM状態、リソース使用量）

**実装時期**: Sprint 8.5（Heartbeat監視と同時）

### アクティブ検出（Reconcile Loop）

controllerが定期的に各サブシステムへ問い合わせ、DB状態と照合する。heartbeatではカバーできないNetwork・Storage層を担当する。

```
reconcile loop:
  Network (デフォルト5分間隔):
    1. 各ホストのOVSフロー状態を検査
    2. DB上のnetworks/groups/policies/portsと照合
    3. 差分をDriftEventとして発火
    ※ 遷移中ステータスのリソースに紐づくOVSフローは除外

  Storage (デフォルト5分間隔):
    1. 各バックエンドにListVolumes問い合わせ
    2. DB上のvolumesと照合
    3. 差分をDriftEventとして発火
    ※ 遷移中ステータスのボリュームは除外
```

**スケーラビリティ考慮**:
- Network: 各ホストのOVSフロー状態はheartbeatで報告される情報を活用し、追加ポーリングは補完として使う
- Storage: バックエンドごとに独立したintervalを設定可能にする。低速なバックエンドは長いinterval
- 負荷軽減: 1サイクルで全リソースをスキャンするのではなく、バッチ処理（例: ホスト単位、バックエンド単位）で分割

**対象**: Network層（OVSフロー）、Storage層（バックエンド）

**実装時期**: 各レイヤの実装Sprintに組み込む。初期実装はログ出力のみで、DriftEvent基盤（Sprint 8.5）完成後にDriftEvent発火に移行する
- Network基礎（OVSフロー検査 + Policyフロー）: Sprint 5N
- Storage: Sprint 6
- DriftEvent基盤完成 → 全reconcilerをDriftEvent発火に移行: Sprint 8.5

### Heartbeat Monitor（既存設計）

heartbeatタイムアウトによるホスト死活監視。3回連続タイムアウト（デフォルト30秒無応答）で`faulty`に自動遷移。

**注意**: heartbeat-fail-countの追跡はcontrollerのインメモリ状態。controller再起動時にカウンタはリセットされる。再起動直後は全ホストの`last_heartbeat`がstaleに見えるが、最初のheartbeat受信を待ってからカウンタを開始する。

**対象**: Host層の死活

**実装時期**: Sprint 8.5

## カスケード障害ハンドリング

ホストがfaultyに遷移した場合、そのホスト上の全リソースに影響が波及する。heartbeatが停止するためHeartbeat Reconcilerは発火しない。この状況を`HostFaultyHandler`が処理する。

### HostFaultyHandler

HeartbeatMonitorがfaulty遷移を検出した直後に実行される:

```
HostFaultyHandler(host_id):
  1. DB上でhost_idに紐づく全VMを検索
  2. 遷移中でないVMのステータスを error に更新
  3. 各VMに紐づくポートのステータスを down に更新
  4. DriftEvent(host/host_faulty, severity=critical) を発火
  5. 各VMについて DriftEvent(compute/host_fault_cascade, severity=critical) を発火
  6. [Sprint 13.5以降] HA Failoverをトリガー
```

**ボリュームの扱い**: ホスト障害時、ボリュームのDBステータス(`in_use`)は変更しない。ボリュームはストレージバックエンド上に存在しており、ホスト障害でボリューム自体が消失するわけではない。HA Failover時にボリュームは新ホストに再アタッチされる。

**実装時期**: Sprint 8.5（HA Failover連携はSprint 13.5）

## フェンシング（スプリットブレイン防止）

### 問題

ネットワーク分断により、controllerがworkerと通信できない場合:
- controllerはhostをfaultyと判定し、HA failoverで別ホストにVMを再起動する
- 実際にはworkerは正常で、元のVMは稼働し続けている
- 同一VMが2台起動し、同一IP/MAC、同一ボリュームへの二重書き込みが発生（スプリットブレイン）

### 対策

faulty判定はHA failoverの**必要条件だが十分条件ではない**。failover実行前にフェンシング（元ホストの強制停止確認）を行う:

```
HA Failover前:
  1. HeartbeatMonitor: faulty遷移
  2. HostFaultyHandler: VM/ポートのステータス更新
  3. FencingAgent: 元ホストの強制シャットダウン確認
     - IPMI power-off（hook経由でAWXが実行）
     - 電源OFF確認後に proceed
     - フェンシング失敗時: failoverを中止し、Alert(critical)で管理者に通知
  4. Scheduler.Reschedule: 新ホスト選定
  5. VM再起動
```

**Phase 1では**: フェンシング未実装。faulty遷移 + Alert のみ。HA failoverはPhase 2（Sprint 13.5）でフェンシング込みで実装する。

**実装時期**: Sprint 13.5

## 対応アクション

検出されたドリフトに対する対応は3段階。

### Alert（通知のみ）

ログ出力 + 将来の通知連携ポイント。人間が判断する。

適用ケース:
- 未知リソースの検出（DB無・実体有）— 初回検出時
- ストレージのボリューム消失
- 原因不明のリソース使用量急変
- フェンシング失敗

### Auto-heal（自動修復）

自動的に状態を正しい方向に修復する。データ損失リスクが無く、冪等な操作のみ。

適用ケース:
- **VM消失（expected_missing）**: DBステータスを`error`に更新（`shutoff`ではない。ユーザ操作によるshutoffとの区別のため）
- **VMステータス不一致（state_mismatch）**: ユーザ起因の操作が無い場合のみ。DB上に直前のStopVM/StartVM操作記録が無く、かつlibvirtがshutoff → DBを`error`に更新（異常停止とみなす）。ユーザ操作記録がある場合はDBを実態に同期
- **フロー更新**: OVSフローが期待と異なる場合、HostNetworkStateを再配信
- **faulty遷移**: heartbeat途絶 → ホストをfaultyに遷移 + HostFaultyHandlerでカスケード処理

### Failover（切替）

別リソースへの切り替え。スケジューラと連携する。フェンシング必須。

適用ケース:
- ホスト障害（faulty遷移）→ フェンシング → HA failover（VMを別ホストで再起動）
- ストレージバックエンド障害 → ボリュームフェイルオーバー（レプリカがある場合）

### 未知リソースの段階的対応（unexpected_present）

「DB無・実体有」のリソースは単なるAlert止まりではなく、段階的に対応する:

1. **初回検出**: Alert（high）で通知。DriftEventを記録
2. **N回連続検出**（デフォルト3回）: 直近の失敗操作（作成/削除の途中失敗）との相関を調査
3. **相関あり**: orphanリソースと判定。操作ログに基づきクリーンアップ候補としてマーク
4. **相関なし**: セキュリティ/整合性異常としてAlert(critical)に昇格。管理者判断を待つ

VMについては、未知VMをそのまま放置するとリソースを消費し続けるため、将来的に`quarantine`ステータスを導入し、ネットワークを切断した状態で保全する。

## 並行性モデル

### 問題

- 複数reconcilerが同じリソースについてDriftEventを生成する可能性がある
- auto-healとユーザ操作（StopVM等）が同時に同じリソースのステータスを変更する可能性がある
- heartbeatは10秒間隔×ホスト数で到着し、各heartbeatがreconcile処理を含む

### 解決策

#### リソース単位のロック

auto-healアクションは対象リソースのIDでアドバイザリーロックを取得してから実行する。ユーザ操作も同じロックを使用する。

```go
// auto-heal実行前
if !tryLock(resourceID) {
    // 他の操作が進行中 → skip（次のheartbeatで再評価）
    return
}
defer unlock(resourceID)
```

#### 楽観的同時実行制御

VMの状態更新はDBの`updated_at`を条件に含めることで、他の操作が先に変更した場合はauto-healがno-opになる:

```sql
UPDATE vms SET status = 'error', updated_at = now()
WHERE id = $1 AND status = $2 AND updated_at = $3
```

#### DriftEventの重複排除

同一リソース・同一Typeの連続DriftEventは、最初の1件のみDriftHandlerに渡し、後続は抑制する。抑制は`resource_id + type`をキーにしたインメモリキャッシュ（TTL: reconcile-interval × 2）で実装。

#### バックプレッシャー

heartbeat受信時のreconcile処理は非同期キューに投入し、ワーカーgoroutineが処理する。キューが満杯の場合はdropして次のheartbeatで再評価。

## アーキテクチャ

```
Controller
  ├── HeartbeatMonitor           [Sprint 8.5]
  │     ├── heartbeat途絶 → faulty遷移
  │     └── HostFaultyHandler → カスケード状態更新
  │
  ├── HeartbeatReconciler        [Sprint 8.5]
  │     └── heartbeat内VMリスト vs DB → DriftEvent
  │         ※ 遷移中ステータス除外、楽観的ロック
  │
  ├── ReconcileLoop              [各レイヤSprintで段階実装]
  │     ├── NetworkReconciler     [Sprint 5 → 9で拡張]
  │     └── StorageReconciler    [Sprint 6]
  │     ※ 初期はログ出力のみ、Sprint 8.5のDriftEvent基盤完成後にDriftEvent発火に移行
  │
  ├── DriftHandler               [Sprint 8.5]
  │     ├── Deduplicator         重複排除（resource_id + typeベース）
  │     ├── Logger/AlertSink     ログ出力 + drift_eventsテーブル永続化
  │     ├── AutoHealer           VMステータス同期、OVSフロー更新等
  │     └── FailoverTrigger      [Sprint 13.5] フェンシング + HA failover
  │
  └── FencingAgent               [Sprint 13.5]
        └── hook経由でIPMI power-off → 確認
```

### DriftEvent

全ての検出結果は統一的な`DriftEvent`として扱う。

```go
type DriftEvent struct {
    ID          string    // イベントID（UUID）
    Layer       string    // "compute", "network", "storage", "host"
    Type        string    // "expected_missing", "unexpected_present", "state_mismatch",
                          // "heartbeat_timeout", "host_fault_cascade"
    Severity    string    // "critical", "high", "medium"
    Resource    string    // リソース種別 ("vm", "port", "volume", "flow", "policy_flow")
    ResourceID  string    // リソースID
    TenantID    string    // テナントID（影響範囲の特定用、ホスト系はempty）
    HostID      string    // 関連ホスト（あれば）
    Expected    string    // DBの状態
    Actual      string    // 実際の状態
    DetectedBy  string    // "heartbeat_reconciler", "network_reconciler", "storage_reconciler",
                          // "heartbeat_monitor", "host_faulty_handler"
    Action      string    // "alert", "auto_heal", "failover", "suppressed"
    Timestamp   time.Time
}
```

DriftEventは`drift_events`テーブルに永続化する。履歴分析、トレンド検出、監査に使用。

### 対応判定テーブル

| Layer | Type | Resource | Severity | Action | 条件 |
|-------|------|----------|----------|--------|------|
| compute | expected_missing | vm | critical | Auto-heal: DB→error | 遷移中ステータスでないこと |
| compute | unexpected_present | vm | high | Alert → 段階的対応 | |
| compute | state_mismatch | vm | medium | Auto-heal: DB→error（異常停止）or DB同期（ユーザ操作起因） | ユーザ操作記録の有無で分岐 |
| network | expected_missing | flow | critical | Alert | |
| network | unexpected_present | flow | high | Alert → 段階的対応 | |
| network | state_mismatch | flow | high | Auto-heal: HostNetworkState再配信 | |
| network | expected_missing | policy_flow | critical | Alert | |
| network | state_mismatch | policy_flow | high | Auto-heal: HostNetworkState再配信 | |
| storage | expected_missing | volume | critical | Alert | |
| storage | unexpected_present | volume | high | Alert → 段階的対応 | |
| host | heartbeat_timeout | host | critical | Auto-heal: faulty遷移 + HostFaultyHandler | |
| host | host_fault_cascade | vm | critical | Auto-heal: DB→error（カスケード） | HostFaultyHandler経由 |

## Worker起動時Reconcile

workerプロセスの再起動時に、ホスト上のlibvirt実態とcontroller DBの差分を一括検出する。

これはパッシブ検出の初回実行と同等であり、特別なreconcile RPCを追加せずheartbeatの初回送信で代替可能。worker起動 → RegisterHost → 最初のheartbeat送信 → HeartbeatReconcilerが差分検出。

## 設定

reconcileの動作パラメータ:

| パラメータ | デフォルト | 説明 |
|-----------|-----------|------|
| `heartbeat-timeout` | 30s | heartbeat途絶のタイムアウト |
| `heartbeat-fail-count` | 3 | faulty遷移までの連続タイムアウト回数 |
| `reconcile-interval` | 5m | アクティブ検出のデフォルトポーリング間隔 |
| `reconcile-network-interval` | 5m | Network reconcileの間隔（未指定時はreconcile-interval） |
| `reconcile-storage-interval` | 5m | Storage reconcileの間隔（未指定時はreconcile-interval） |
| `reconcile-enabled` | true | アクティブ検出の有効/無効 |
| `auto-heal-enabled` | true | auto-healアクションの有効/無効（falseならAlert止まり） |
| `unexpected-present-threshold` | 3 | 段階的対応の閾値（N回連続検出で調査開始） |
| `drift-event-retention-days` | 90 | drift_eventsテーブルの保持日数 |
