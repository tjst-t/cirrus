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
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	State VMState `json:"state"`
	Vcpus int32   `json:"vcpus"`
	RAMMb int64   `json:"ram_mb"`
}

// HostInfo holds summary information about a hypervisor host.
type HostInfo struct {
	Hostname string `json:"hostname"`
	Vcpus    int32  `json:"vcpus"`
	MemoryMB int64  `json:"memory_mb"`
}

// DiskSpec describes a block device to attach to a VM.
type DiskSpec struct {
	// DevicePath is the host-side block device (e.g. /dev/sdb, /dev/null for sim).
	DevicePath string
	// TargetDev is the guest device name (e.g. "vda").
	TargetDev string
}

// InterfaceSpec describes a network interface to attach to a VM.
type InterfaceSpec struct {
	// PortID is the OVS port / interface-id for the NIC.
	PortID string
	// MACAddress for the guest interface.
	MACAddress string
	// BridgeName is the OVS bridge to attach to.
	BridgeName string
}

// CloudInitSpec contains the user-data / meta-data / network-config for cloud-init.
type CloudInitSpec struct {
	Hostname   string
	UserData   string // #cloud-config YAML
	MetaData   string // YAML key-value pairs
	NetworkConfig string // network-config YAML (optional)
}

// VMSpec is the full specification for defining a VM.
type VMSpec struct {
	Name        string
	VCPUs       int32
	RAMMB       int64
	Disks       []DiskSpec
	Interfaces  []InterfaceSpec
	CloudInit   *CloudInitSpec
	// CloudInitISOPath is set by DefineVM after ISO generation; callers can also
	// supply it directly to bypass built-in generation.
	CloudInitISOPath string
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

	// DefineVM creates a new VM definition in shutoff state.
	DefineVM(ctx context.Context, spec VMSpec) (*VMInfo, error)

	// StartVM starts a shutoff VM.
	StartVM(ctx context.Context, name string) error

	// StopVM requests a graceful shutdown of a running VM.
	StopVM(ctx context.Context, name string) error

	// DestroyVM forcefully powers off a running VM.
	DestroyVM(ctx context.Context, name string) error

	// UndefineVM removes a VM definition (must be shutoff first).
	UndefineVM(ctx context.Context, name string) error
}
