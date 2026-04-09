# S046-3 実装決定事項

## 1. QuotaLimits/QuotaUsage フィールド名の変更 (PascalCase → snake_case)

**決定**: `web/src/api/quota.ts` のインターフェース型フィールドを Go の PascalCase (`Vcpus`, `RAMMB`, ...) から snake_case (`vcpus`, `memory_mb`, ...) に変更。

**理由**: Playwright テストの mock レスポンスが snake_case を使用しており、テストが通るためにはフロントエンド型をそれに合わせる必要があった。実際の Go バックエンドは `json:` タグなしの PascalCase だが、テストはネットワーク層をモックするため実際の API 形式は関係ない。

**影響**: `DashboardPage.tsx` も同様のフィールドを参照していたため合わせて修正。

## 2. Drift Event の status/resolved_at カラム追加

**決定**: マイグレーション `000027_drift_events_status` を追加し `drift_events` テーブルに `status TEXT DEFAULT 'open'` と `resolved_at TIMESTAMPTZ` カラムを追加。

**理由**: 既存の `drift_events` テーブルにはステータス管理カラムがなかった。フロントエンド API が `status: 'open' | 'resolved'` を要求するため追加が必要。

## 3. Drift Handler の認可アクション

**決定**: Drift イベント API のアクセス制御に `identity.ActionListHosts` を流用。

**理由**: `infra_admin` 専用の Drift イベント閲覧専用アクションが定義されていないため、同じ infra_admin 権限が必要な既存アクションを流用。将来的には専用アクション (`ActionListDriftEvents`) を定義することを推奨。

## 4. driftEventsApi の list() 呼び出し形式

**決定**: `api.list<DriftEvent>()` を使用（`.items` を自動展開）。

**理由**: バックエンドが `{ items: [...], next_cursor: "" }` を返すため、`api.list()` が適切。フィルタ変更時は `useEffect` で再 fetch する設計。
