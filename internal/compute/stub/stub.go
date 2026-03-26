package stub

import (
	"context"
	"log/slog"
	"sync"

	"github.com/tjst-t/cirrus/internal/compute"
)

type Driver struct {
	mu  sync.Mutex
	vms map[string]string // vmID -> status
	log *slog.Logger
}

func New(log *slog.Logger) *Driver {
	return &Driver{
		vms: make(map[string]string),
		log: log,
	}
}

func (d *Driver) CreateVM(_ context.Context, spec compute.VMSpec) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.vms[spec.ID] = "running"
	d.log.Info("[stub/compute] CreateVM", "vm_id", spec.ID, "name", spec.Name,
		"vcpus", spec.VCPUs, "ram_mb", spec.RamMB, "disk", spec.Disk.Source)
	return nil
}

func (d *Driver) DeleteVM(_ context.Context, vmID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.vms, vmID)
	d.log.Info("[stub/compute] DeleteVM", "vm_id", vmID)
	return nil
}

func (d *Driver) StopVM(_ context.Context, vmID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.vms[vmID] = "shutoff"
	d.log.Info("[stub/compute] StopVM", "vm_id", vmID)
	return nil
}

func (d *Driver) StartVM(_ context.Context, vmID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.vms[vmID] = "running"
	d.log.Info("[stub/compute] StartVM", "vm_id", vmID)
	return nil
}

func (d *Driver) GetStatus(_ context.Context, vmID string) (compute.VMStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	status, ok := d.vms[vmID]
	if !ok {
		return compute.VMStatus{ID: vmID, Status: "not_found"}, nil
	}
	return compute.VMStatus{ID: vmID, Status: status}, nil
}

func (d *Driver) ListVMs(_ context.Context) ([]compute.VMStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var result []compute.VMStatus
	for id, status := range d.vms {
		result = append(result, compute.VMStatus{ID: id, Status: status})
	}
	return result, nil
}
