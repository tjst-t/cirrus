package compute

import "context"

// Driver abstracts VM lifecycle management.
type Driver interface {
	CreateVM(ctx context.Context, spec VMSpec) error
	DeleteVM(ctx context.Context, vmID string) error
	StopVM(ctx context.Context, vmID string) error
	StartVM(ctx context.Context, vmID string) error
	GetStatus(ctx context.Context, vmID string) (VMStatus, error)
	ListVMs(ctx context.Context) ([]VMStatus, error)
}

type VMSpec struct {
	ID        string
	Name      string
	VCPUs     int
	RamMB     int
	Disk      DiskSpec
	Ports     []PortSpec
	CloudInit []byte
}

type DiskSpec struct {
	Type   string // "file", "network"
	Source string // path or rbd/iscsi uri
	Format string // qcow2, raw
}

type PortSpec struct {
	ID  string
	MAC string
}

type VMStatus struct {
	ID     string
	Status string // "running", "shutoff", "not_found"
}
