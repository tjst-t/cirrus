package compute

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// VMStatus represents the lifecycle state of a VM.
type VMStatus string

const (
	VMStatusPending  VMStatus = "pending"
	VMStatusBuilding VMStatus = "building"
	VMStatusRunning  VMStatus = "running"
	VMStatusStopped  VMStatus = "stopped"
	VMStatusError    VMStatus = "error"
	VMStatusDeleting VMStatus = "deleting"
)

// VM represents a virtual machine managed by Cirrus.
type VM struct {
	ID           uuid.UUID  `json:"id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	Name         string     `json:"name"`
	FlavorID     *uuid.UUID `json:"flavor_id,omitempty"`
	AZID         *uuid.UUID `json:"az_id,omitempty"`
	NetworkID    *uuid.UUID `json:"network_id,omitempty"`
	HostID       *uuid.UUID `json:"host_id,omitempty"`
	Status       VMStatus   `json:"status"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// transitionalStatuses are states where no operations are allowed.
var transitionalStatuses = map[VMStatus]bool{
	VMStatusBuilding: true,
	VMStatusDeleting: true,
	VMStatusPending:  true,
}

// ErrConflict is returned when an operation is not allowed in the current state.
var ErrConflict = errors.New("compute: operation not allowed in current vm state")

// CanStart reports whether the VM can be started.
func (v *VM) CanStart() bool { return v.Status == VMStatusStopped }

// CanStop reports whether the VM can be stopped (graceful or force).
func (v *VM) CanStop() bool { return v.Status == VMStatusRunning }

// CanReboot reports whether the VM can be rebooted.
func (v *VM) CanReboot() bool { return v.Status == VMStatusRunning }

// CanDelete reports whether the VM can be deleted.
func (v *VM) CanDelete() bool {
	return v.Status == VMStatusStopped || v.Status == VMStatusError
}

// IsTransitional reports whether the VM is in a transitional state.
func (v *VM) IsTransitional() bool { return transitionalStatuses[v.Status] }

// CreateVMSpec is the input for creating a VM.
type CreateVMSpec struct {
	TenantID     uuid.UUID
	Name         string
	FlavorID     uuid.UUID
	AZID         uuid.UUID
	NetworkID    uuid.UUID
	VolumeTypeID *uuid.UUID // optional; if nil, uses any available backend
}
