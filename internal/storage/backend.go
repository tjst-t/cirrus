package storage

import (
	"context"
	"encoding/json"
)

// Backend abstracts disk storage operations.
type Backend interface {
	CreateDisk(ctx context.Context, vmID string, baseImage string, sizeGB int) (*DiskResult, error)
	DeleteDisk(ctx context.Context, vmID string, stored json.RawMessage) error
	DiskSpec(vmID string, stored json.RawMessage) LibvirtDiskSpec
}

type DiskResult struct {
	DriverData json.RawMessage
}

type LibvirtDiskSpec struct {
	Type   string // "file", "network"
	Source string // path or rbd/iscsi uri
	Format string // qcow2, raw
}
