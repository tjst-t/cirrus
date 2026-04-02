package storage

import (
	"context"

	"github.com/google/uuid"
)

// Service provides volume and backend management operations.
type Service interface {
	// Backend management (infra admin)
	RegisterBackend(ctx context.Context, spec RegisterBackendSpec) (*Backend, error)
	GetBackend(ctx context.Context, id uuid.UUID) (*Backend, error)
	ListBackends(ctx context.Context) ([]Backend, error)
	DrainBackend(ctx context.Context, id uuid.UUID) error

	// Volume type management (infra admin: create; tenant: list)
	CreateVolumeType(ctx context.Context, name, description string, requiredCaps []string, qosPolicy map[string]any, isPublic bool) (*VolumeType, error)
	GetVolumeType(ctx context.Context, id uuid.UUID) (*VolumeType, error)
	ListVolumeTypes(ctx context.Context) ([]VolumeType, error)

	// Volume lifecycle (tenant)
	CreateVolume(ctx context.Context, spec CreateVolumeSpec) (*Volume, error)
	GetVolume(ctx context.Context, tenantID, volumeID uuid.UUID) (*Volume, error)
	ListVolumes(ctx context.Context, tenantID uuid.UUID) ([]Volume, error)
	DeleteVolume(ctx context.Context, tenantID, volumeID uuid.UUID) error
	ResizeVolume(ctx context.Context, tenantID, volumeID uuid.UUID, newSizeGB int64) (*Volume, error)

	// Export/unexport (called by Compute orchestrator during VM create/delete)
	ExportVolume(ctx context.Context, volumeID, hostID uuid.UUID) (*ExportInfo, error)
	UnexportVolume(ctx context.Context, volumeID uuid.UUID) error

	// ListVolumesOnBackend returns all volumes on a backend (for Reconciler)
	ListVolumesOnBackend(ctx context.Context, backendID uuid.UUID) ([]Volume, error)

	// ListBackendsForAZ returns active backends in the given AZ (for Scheduler)
	ListBackendsForAZ(ctx context.Context, azID uuid.UUID) ([]Backend, error)

	// ListBackendsReachableFromHost returns active backends reachable from a host (for Scheduler)
	ListBackendsReachableFromHost(ctx context.Context, hostID uuid.UUID) ([]Backend, error)
}
