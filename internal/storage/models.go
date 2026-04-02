package storage

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BackendState represents the lifecycle state of a storage backend.
type BackendState string

const (
	BackendStateActive   BackendState = "active"
	BackendStateDraining BackendState = "draining"
	BackendStateRetired  BackendState = "retired"
)

// Backend represents a registered storage backend.
type Backend struct {
	ID              uuid.UUID       `json:"id"`
	StorageDomainID uuid.UUID       `json:"storage_domain_id"`
	Name            string          `json:"name"`
	Driver          string          `json:"driver"` // "sim", "iscsi", "rbd", ...
	Endpoint        string          `json:"endpoint"`
	TotalCapacityGB int64           `json:"total_capacity_gb"`
	TotalIOPS       int64           `json:"total_iops"`
	Capabilities    json.RawMessage `json:"capabilities"`  // []string
	DriverConfig    json.RawMessage `json:"driver_config"` // driver-specific
	State           BackendState    `json:"state"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// VolumeType is the user-facing abstraction of backend characteristics.
type VolumeType struct {
	ID                   uuid.UUID       `json:"id"`
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	RequiredCapabilities json.RawMessage `json:"required_capabilities"` // []string
	QoSPolicy            json.RawMessage `json:"qos_policy,omitempty"`
	IsPublic             bool            `json:"is_public"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// VolumeState represents the lifecycle state of a volume.
type VolumeState string

const (
	VolumeStateCreating  VolumeState = "creating"
	VolumeStateAvailable VolumeState = "available"
	VolumeStateInUse     VolumeState = "in_use"
	VolumeStateDeleting  VolumeState = "deleting"
	VolumeStateError     VolumeState = "error"
)

// Volume is the user-facing logical disk.
type Volume struct {
	ID             uuid.UUID       `json:"id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	Name           string          `json:"name"`
	VolumeTypeID   *uuid.UUID      `json:"volume_type_id,omitempty"`
	BackendID      *uuid.UUID      `json:"backend_id,omitempty"`
	SizeGB         int64           `json:"size_gb"`
	State          VolumeState     `json:"state"`
	ExportedHostID *uuid.UUID      `json:"exported_host_id,omitempty"`
	ExportInfo     json.RawMessage `json:"export_info,omitempty"`
	AZID           *uuid.UUID      `json:"az_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// RegisterBackendSpec is the input for registering a new backend.
type RegisterBackendSpec struct {
	StorageDomainID uuid.UUID
	Name            string
	Driver          string
	Endpoint        string
	TotalCapacityGB int64
	TotalIOPS       int64
	Capabilities    []string
	DriverConfig    map[string]any
}

// CreateVolumeSpec is the input for creating a volume.
type CreateVolumeSpec struct {
	TenantID     uuid.UUID
	Name         string
	VolumeTypeID *uuid.UUID
	SizeGB       int64
	AZID         *uuid.UUID
}
