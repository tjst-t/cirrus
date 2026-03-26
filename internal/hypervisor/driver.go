package hypervisor

import "context"

// VMState represents the state of a virtual machine.
type VMState string

const (
	StateRunning  VMState = "running"
	StatePaused   VMState = "paused"
	StateShutoff  VMState = "shutoff"
	StateCrashed  VMState = "crashed"
	StateShutdown VMState = "shutdown"
)

// VMInfo holds summary information about a running VM.
type VMInfo struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	State  VMState `json:"state"`
	Vcpus  int32   `json:"vcpus"`
	RAMMb  int64   `json:"ram_mb"`
}

// HostInfo holds summary information about a hypervisor host.
type HostInfo struct {
	Hostname string `json:"hostname"`
	Vcpus    int32  `json:"vcpus"`
	MemoryMB int64  `json:"memory_mb"`
}

// Driver abstracts hypervisor operations for VM lifecycle management.
type Driver interface {
	// Connect establishes a connection to the hypervisor.
	Connect(ctx context.Context) error

	// Close releases the hypervisor connection.
	Close() error

	// GetHostInfo returns information about the hypervisor host.
	GetHostInfo(ctx context.Context) (*HostInfo, error)

	// ListVMs returns all VMs on the hypervisor.
	ListVMs(ctx context.Context) ([]VMInfo, error)
}
