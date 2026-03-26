package stub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/tjst-t/cirrus/internal/storage"
)

type Backend struct {
	mu    sync.Mutex
	disks map[string]bool
	log   *slog.Logger
}

func New(log *slog.Logger) *Backend {
	return &Backend{
		disks: make(map[string]bool),
		log:   log,
	}
}

func (b *Backend) CreateDisk(_ context.Context, vmID string, baseImage string, sizeGB int) (*storage.DiskResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.disks[vmID] = true
	b.log.Info("[stub/storage] CreateDisk", "vm_id", vmID, "base", baseImage, "size_gb", sizeGB)
	return &storage.DiskResult{DriverData: nil}, nil
}

func (b *Backend) DeleteDisk(_ context.Context, vmID string, _ json.RawMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.disks, vmID)
	b.log.Info("[stub/storage] DeleteDisk", "vm_id", vmID)
	return nil
}

func (b *Backend) DiskSpec(vmID string, _ json.RawMessage) storage.LibvirtDiskSpec {
	return storage.LibvirtDiskSpec{
		Type:   "file",
		Source: fmt.Sprintf("/var/lib/cirrus/disks/%s.qcow2", vmID),
		Format: "qcow2",
	}
}
