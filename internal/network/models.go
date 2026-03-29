package network

import (
	"time"

	"github.com/google/uuid"
)

// NetworkStatus represents the lifecycle state of a network.
type NetworkStatus string

const (
	NetworkStatusCreating NetworkStatus = "creating"
	NetworkStatusActive   NetworkStatus = "active"
	NetworkStatusDeleting NetworkStatus = "deleting"
	NetworkStatusError    NetworkStatus = "error"
)

// PortStatus represents the lifecycle state of a port.
type PortStatus string

const (
	PortStatusCreating PortStatus = "creating"
	PortStatusDown     PortStatus = "down"
	PortStatusActive   PortStatus = "active"
	PortStatusDeleting PortStatus = "deleting"
	PortStatusError    PortStatus = "error"
)

// Network represents a tenant virtual network.
type Network struct {
	ID        uuid.UUID     `json:"id"`
	TenantID  uuid.UUID     `json:"tenant_id"`
	Name      string        `json:"name"`
	Status    NetworkStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Port represents a virtual network port.
type Port struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	NetworkID  uuid.UUID  `json:"network_id"`
	VMID       *uuid.UUID `json:"vm_id,omitempty"`
	MACAddress string     `json:"mac_address"`
	IPAddress  string     `json:"ip_address"`
	Status     PortStatus `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

// NetworkSpec is the input for creating a new network.
type NetworkSpec struct {
	Name string `json:"name"`
}
