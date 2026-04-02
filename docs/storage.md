# ストレージ設計

## ストレージの種類と優先度

**ブロックストレージ（最優先）** — VMにディスクとして見える。VMのブートディスクに必須。Cirrusの中核。

**ファイルストレージ（後回し）** — NFS/CIFSの共有ボリューム。ブロックストレージと同じ抽象化＋ACL。需要が出てから対応。

**オブジェクトストレージ（後回し）** — S3互換API。MinIO等の外部サービスをCirrusの認証基盤と統合。テンプレートのバックストアとしても有用。

## ブロックストレージの抽象化

### ストレージバックエンド

容量とIOPSを提供するリソースの単位。物理装置の中の論理分割はCirrusのスコープ外で、分割された結果をバックエンドとして宣言的に登録する。

バックエンドのメタデータ:

- 容量、IOPS、帯域幅
- 到達可能ホスト群
- Capability（SSD/HDD、暗号化、レプリケーション、差分転送、QoS対応、ストレージライブマイグレーション対応等）

### ボリュームタイプ

バックエンドの特性をユーザ向けに抽象化。Flavorのストレージ版。QoSポリシーを紐づけてnoisy neighborを制御。

### ボリューム

ユーザ向けの論理ディスク。1ボリューム＝1VMのディスクが基本単位。バックエンドの種類（NFS、Ceph、iSCSI等）によらず、バックエンドドライバインターフェース（作成、削除、アタッチ、デタッチ、リサイズ、スナップショット、クローン）で統一。

### バックエンドドライバインターフェース

Driverは**Controller側**で動作し、ストレージバックエンドの管理APIを呼び出す。Workerホストには直接アクセスしない。

ホスト側のOSレベルアタッチ（iscsiadm、rbd map等）はWorker側の `blockdev.Manager` が担う（責務分担は下記参照）。

```go
// internal/storage/driver.go
package storage

type Driver interface {
    // ボリュームライフサイクル
    CreateVolume(ctx context.Context, spec VolumeSpec) (*Volume, error)
    DeleteVolume(ctx context.Context, volumeID string) error
    ResizeVolume(ctx context.Context, volumeID string, newSizeGB int) error

    // エクスポート（ストレージ側のアクセス許可設定）
    // HostInfo.Propertiesにプロトコル固有の接続情報（IQN等）が含まれる
    ExportVolume(ctx context.Context, volumeID string, host HostInfo) (*ExportInfo, error)
    UnexportVolume(ctx context.Context, volumeID string, host HostInfo) error

    // スナップショット
    CreateSnapshot(ctx context.Context, volumeID string, name string) (*Snapshot, error)
    DeleteSnapshot(ctx context.Context, snapshotID string) error

    // クローン
    CloneVolume(ctx context.Context, snapshotID string, spec VolumeSpec) (*Volume, error)

    // Capability照会
    Capabilities() DriverCapabilities
}

// HostInfo はExportVolume時にDriverに渡すホスト接続属性。
// Storage ServiceがDBのhostsレコードから組み立てる。
type HostInfo struct {
    ID         string
    DataIPs    []string
    Properties map[string]string // hosts.storage_properties から詰めたもの
                                 // 例: {"iscsi_iqn": "iqn.2024.com.example:host1"}
}

// ExportInfo はWorkerのblockdev.ManagerがOSレベルアタッチに使う接続情報。
type ExportInfo struct {
    Protocol string            // "rbd", "iscsi", "nfs", ...
    Params   map[string]string // プロトコル固有パラメータ
}

type DriverCapabilities struct {
    QoS                    bool
    Encryption             bool
    Replication            bool
    DifferentialTransfer   bool
    StorageLiveMigration   bool
    Snapshot               bool
    Clone                  bool
}
```

### DriverとBlockDevの責務分担

```
[Controller] Storage.ExportVolume(volumeID, hostID)
               → HostInfo組み立て（DB参照）
               → Driver.ExportVolume(volumeID, hostInfo)
                   → ストレージバックエンド管理API呼び出し
                       （iSCSI target作成+ACL設定、RBD keyring付与等）
               → ExportInfo を返す
               ↓ gRPC CreateVM に DiskSpec(ExportInfo) として含める
[Worker]     blockdev.Manager.Attach(ExportInfo)
               → OSレベルのデバイスとして接続
                   （iscsiadm login、rbd map等）
               → /dev/vda が現れる
```

### ホストのストレージ接続属性

iSCSIイニシエータIQNやCephキーリング等、プロトコル固有の接続情報は**ホストの属性**であり、ストレージ実装の詳細ではない。hostsテーブルの `storage_properties JSONB` カラムに格納し、管理者がホスト登録時に設定する。

```sql
-- hosts テーブル
storage_properties JSONB DEFAULT '{}'
-- 例: {"iscsi_iqn": "iqn.2024.com.example:host1", "ceph_client": "client.host1"}
```

プロトコル固有カラムをhostsテーブルに追加しないことで、新しいドライバを追加してもDBマイグレーションが不要になる。

## スナップショット・クローンの依存関係管理

**スナップショット** — 親ボリュームに依存する差分データ。読み取り専用。

**クローン** — スナップショットから作られた読み書き可能なボリューム。バックエンド内部ではcopy-on-writeで親に依存しているが、Cirrusの抽象化レベルでは論理的な依存関係として管理。

**依存関係グラフ** — ボリューム→スナップショット→クローンの親子関係をCirrusのメタデータとして保持。バックエンド内部のcopy-on-write実装は隠蔽。

```
Volume-A
├── Snapshot-1 (read-only)
│   ├── Clone-X (read-write, independent volume)
│   └── Clone-Y (read-write, independent volume)
└── Snapshot-2 (read-only)
    └── Clone-Z (read-write, independent volume)
```

**削除の制約** — 子が存在するスナップショットは削除不可。削除拒否するか、子をフラット化（フルコピーに変換して依存を切る）してから削除。フラット化は非同期操作。

**移行時の扱い** — バックエンド間ではcopy-on-write関係を維持できないため、フラット化して独立ボリュームとして移行。

## リージョン間レプリケーション

リージョン跨ぎではスナップショットの依存関係を維持しない（障害ドメイン分離の原則）。

**ローカルスナップショット** — 同一バックエンド内のcopy-on-write。高速・容量効率良い。依存関係あり。

**レプリカ** — スナップショットのデータを別バックエンド（別リージョン含む）にフルコピーしたもの。完全に独立。

**レプリケーションポリシー** — DR用の定期レプリケーション。対象、宛先、頻度、保持世代数を定義。バックエンドが差分転送対応ならcapabilityとして宣言し、Cirrusが差分転送を選択。非対応なら毎回フルコピー。

## テンプレート管理

**テンプレートサービス** — ボリュームとは別のサービス。メタデータとアクセス制御を担い、実データはストレージバックエンド上に保持。

**キャッシュコピー** — テンプレートの実データがないバックエンドでVM作成時、バックグラウンドで透過的にコピー。LRUで自動管理し、使われないキャッシュは自動削除。

**公開範囲** — public（Cirrus全体）、organization（組織内全テナント）、tenant（テナント内のみ）、shared（指定先に共有、将来）。

**レプリカとの共通基盤** — テンプレートキャッシュとレプリカは同じデータ転送の仕組みを共有。管理ポリシーの違い（自律的管理 vs ユーザ管理）をメタデータで区別。

## ストレージバックエンドのライフサイクル

```
登録 → 検証 → 稼働中 → 縮退 → ドレイン → 読み取り専用 → 退役
```

**縮退フェーズ** — 新規ボリューム配置の優先度を下げる。EOLの2年前から開始し、ドレイン期間を短縮。容量閾値による自動トリガーも設ける。

**ドレインフェーズ** — 新規ボリューム作成を完全停止。既存ボリュームを他バックエンドに順次移行。依存関係を考慮した移行順序、帯域制限、進捗可視化（残りボリューム数、データ量、推定完了時間）。

## ストレージライブマイグレーション

VMが稼働中のまま、ボリュームを別バックエンドに移動。バックエンドドライバのcapabilityとして扱う。

- **同種バックエンド間** — ネイティブ機能で効率的に実行
- **異種バックエンド間** — ホスト経由のブロックコピーで汎用的に実行。遅いが常に動作
- **移行非対応** — VMごとコンピュートライブマイグレーション（ブロックマイグレーション付き）で対応

## VMプレースメントとボリュームプレースメントの連動

統一トポロジモデルにより、ホストとバックエンドのペアを同時に評価できる。

**新規VM:** ボリュームタイプ要件を満たすバックエンド列挙 → 各バックエンドに到達可能なホスト列挙 → ホスト側（capability、空きリソース）とバックエンド側（空き容量、IOPS余裕）を同時評価 → （ホスト, バックエンド）ペアのスコアリング。

**既存ボリューム:** ボリュームのバックエンドに到達可能なホストに絞ってスケジューリング。

**複数ボリューム:** 全ボリュームのバックエンドに到達可能なホストに絞り込み。既存ボリュームが制約として先に効く。
