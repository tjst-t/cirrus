package host

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OperationalState represents the administrative state of a host.
type OperationalState string

const (
	StateRegistering OperationalState = "registering"
	StateActive      OperationalState = "active"
	StateMaintenance OperationalState = "maintenance"
	StateDraining    OperationalState = "draining"
	StateFaulty      OperationalState = "faulty"
	StateRetiring    OperationalState = "retiring"
)

// IsValidOperationalState returns true if the given state is a known operational state.
func IsValidOperationalState(s OperationalState) bool {
	switch s {
	case StateRegistering, StateActive, StateMaintenance, StateDraining, StateFaulty, StateRetiring:
		return true
	}
	return false
}

// Host represents a physical compute host managed by Cirrus.
type Host struct {
	ID               uuid.UUID        `json:"id"`
	Name             string           `json:"name"`
	Address          string           `json:"address"`
	FabricIP         string           `json:"fabric_ip,omitempty"`
	OperationalState OperationalState `json:"operational_state"`
	Capability       json.RawMessage  `json:"capability"`
	ResourcePhysical json.RawMessage  `json:"resource_physical"`
	OvercommitRatios json.RawMessage  `json:"overcommit_ratios"`
	ResourceUsed     json.RawMessage  `json:"resource_used"`
	LastHeartbeat    *time.Time       `json:"last_heartbeat,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// ResourceReport is resource usage data sent from workers via heartbeat.
type ResourceReport struct {
	UsedVcpus int32 `json:"used_vcpus"`
	UsedRAMMB int64 `json:"used_ram_mb"`
}

// AllocatableResources represents the remaining allocatable resources on a host.
type AllocatableResources struct {
	Vcpus    float64 `json:"vcpus"`
	MemoryMB float64 `json:"memory_mb"`
}

// PhysicalResources holds the raw physical resource counts for a host.
type PhysicalResources struct {
	Vcpus    int   `json:"vcpus"`
	MemoryMB int64 `json:"memory_mb"`
}

// OvercommitRatios holds per-resource overcommit rates.
type OvercommitRatios struct {
	Vcpus    float64 `json:"vcpus"`
	MemoryMB float64 `json:"memory_mb"`
}
