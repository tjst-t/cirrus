package compute

import (
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
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateVMSpec is the input for creating a VM.
type CreateVMSpec struct {
	TenantID     uuid.UUID
	Name         string
	FlavorID     uuid.UUID
	AZID         uuid.UUID
	NetworkID    uuid.UUID
	VolumeTypeID *uuid.UUID // optional; if nil, uses any available backend
}
