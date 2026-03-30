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
	CIDR      string        `json:"cidr"`
	VNI       int           `json:"vni"`
	Status    NetworkStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Port represents a virtual network port.
type Port struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	NetworkID  uuid.UUID  `json:"network_id"`
	GroupID    *uuid.UUID `json:"group_id,omitempty"`
	VMID       *uuid.UUID `json:"vm_id,omitempty"`
	MACAddress string     `json:"mac_address"`
	IPAddress  string     `json:"ip_address"`
	HostID     *uuid.UUID `json:"host_id,omitempty"`
	Role       string     `json:"role"`
	Status     PortStatus `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Group represents a collection of VMs within a network for policy targeting.
type Group struct {
	ID        uuid.UUID `json:"id"`
	NetworkID uuid.UUID `json:"network_id"`
	Name      string    `json:"name"`
}

// Policy represents a communication rule between groups within a network.
type Policy struct {
	ID         uuid.UUID `json:"id"`
	NetworkID  uuid.UUID `json:"network_id"`
	SrcGroupID uuid.UUID `json:"src_group_id"`
	DstGroupID uuid.UUID `json:"dst_group_id"`
	Protocol   string    `json:"protocol"`
	DstPort    *int      `json:"dst_port,omitempty"`
	Priority   int       `json:"priority"`
	Action     string    `json:"action"`
}

// NetworkSpec is the input for creating a new network.
type NetworkSpec struct {
	Name string `json:"name"`
	CIDR string `json:"cidr,omitempty"`
}

// GroupSpec is the input for creating a new group.
type GroupSpec struct {
	Name string `json:"name"`
}

// PolicySpec is the input for creating a new policy.
type PolicySpec struct {
	SrcGroupID uuid.UUID `json:"src_group_id"`
	DstGroupID uuid.UUID `json:"dst_group_id"`
	Protocol   string    `json:"protocol"`
	DstPort    *int      `json:"dst_port,omitempty"`
	Priority   int       `json:"priority,omitempty"`
	Action     string    `json:"action,omitempty"`
}
