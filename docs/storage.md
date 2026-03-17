# ストレージ設計

## interface

```go
// internal/storage/backend.go
package storage

type Backend interface {
    CreateDisk(ctx context.Context, vmID string, baseImage string, sizeGB int) (*DiskResult, error)
    DeleteDisk(ctx context.Context, vmID string, stored json.RawMessage) error
    DiskSpec(stored json.RawMessage) LibvirtDiskSpec
}

type DiskResult struct {
    DriverData json.RawMessage  // controller DBに保存される（nilも可）
}

type LibvirtDiskSpec struct {
    Type   string // "file", "network"
    Source string // path or rbd/iscsi uri
    Format string // qcow2, raw
}
```

## バックエンド実装

### local（Phase 1）

qcow2のCoW cloneでベースイメージからVMディスクを作成。

```go
func (l *Local) CreateDisk(ctx context.Context, vmID, base string, size int) (*storage.DiskResult, error) {
    path := filepath.Join(l.diskDir, vmID+".qcow2")
    // qemu-img create -b base -F qcow2 -f qcow2 path sizeG
    return &storage.DiskResult{DriverData: nil}, nil  // 規約から導出可能
}

func (l *Local) DiskSpec(stored json.RawMessage) storage.LibvirtDiskSpec {
    // storedは無視（常にnil）
    return storage.LibvirtDiskSpec{
        Type:   "file",
        Source: filepath.Join(l.diskDir, vmID+".qcow2"),
        Format: "qcow2",
    }
}
```

### iSCSI（将来）

SAN APIでボリューム作成後、SANが割り当てたIQN/LUN番号を `DriverData` に保存。

```go
type ISCSIMeta struct {
    IQN string `json:"iqn"`
    LUN int    `json:"lun"`
}

func (i *ISCSI) CreateDisk(ctx context.Context, vmID, base string, size int) (*storage.DiskResult, error) {
    vol, _ := i.sanClient.CreateVolume(size)
    meta := ISCSIMeta{IQN: vol.IQN, LUN: vol.LUN}
    return &storage.DiskResult{DriverData: mustMarshal(meta)}, nil
}

func (i *ISCSI) DiskSpec(stored json.RawMessage) storage.LibvirtDiskSpec {
    var meta ISCSIMeta
    json.Unmarshal(stored, &meta)
    return storage.LibvirtDiskSpec{
        Type:   "network",
        Source: fmt.Sprintf("iscsi://%s/%s/%d", i.portal, meta.IQN, meta.LUN),
        Format: "raw",
    }
}
```

### Ceph RBD（将来）

規約ベースで導出可能なため `DriverData` は不要（nilのまま）。

```go
func (c *Ceph) DiskSpec(stored json.RawMessage) storage.LibvirtDiskSpec {
    return storage.LibvirtDiskSpec{
        Type:   "network",
        Source: fmt.Sprintf("rbd:%s/vm-%s", c.pool, vmID),
        Format: "raw",
    }
}
```

## イメージ管理

```go
// internal/image/store.go
package image

type Store interface {
    Upload(ctx context.Context, id string, reader io.Reader) error
    GetPath(ctx context.Context, id string) (string, error)
    Delete(ctx context.Context, id string) error
    EnsureLocal(ctx context.Context, id string) (string, error)  // workerにイメージを配布
}
```

## 設計判断

### DriverData (JSONB) の使い分け

| バックエンド | DriverData | 理由 |
|---|---|---|
| local | NULL | パスはVM IDから導出可能 |
| Ceph RBD | NULL | pool名 + VM IDから導出可能 |
| iSCSI | 使用 | SANが割り当てるIQN/LUNが予測不能 |
| FC | 使用 | SANが割り当てるWWN/LUNが予測不能 |

### バックエンド個別のDBについて

Phase 1では持たない。外部システム（libvirt、OVS、Ceph）自体が状態を保持しており、二重管理は不要。本当に必要になった場合、バックエンド実装内にbbolt等を足す。

### 設定ファイルでのバックエンド選択

```yaml
storage:
  driver: local
  local:
    disk_dir: /var/lib/cirrus/disks
    image_dir: /var/lib/cirrus/images
  # iscsi:
  #   portal: 192.168.100.10:3260
  #   auth:
  #     username: cirrus
  #     password_env: CIRRUS_ISCSI_PASSWORD
  # ceph:
  #   pool: cirrus-vms
  #   monitors: [192.168.100.20:6789]
```
