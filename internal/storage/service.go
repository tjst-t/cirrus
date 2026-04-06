package storage

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/jobqueue"
)

// CreateVolumeResponse holds the result of an async volume creation request.
type CreateVolumeResponse struct {
	JobID uuid.UUID
}

// DeleteVolumeResponse holds the result of an async volume deletion request.
type DeleteVolumeResponse struct {
	JobID uuid.UUID
}

// Service provides volume and backend management operations.
type Service interface {
	// RegisterHandlers registers the storage job handlers with the given dispatcher.
	// Must be called before the dispatcher is started.
	RegisterHandlers(d *jobqueue.Dispatcher)

	// Backend management (infra admin)
	RegisterBackend(ctx context.Context, spec RegisterBackendSpec) (*Backend, error)
	GetBackend(ctx context.Context, id uuid.UUID) (*Backend, error)
	ListBackends(ctx context.Context) ([]Backend, error)
	DrainBackend(ctx context.Context, id uuid.UUID) error

	// Volume type management (infra admin: create; tenant: list)
	CreateVolumeType(ctx context.Context, name, description string, requiredCaps []string, qosPolicy map[string]any, isPublic bool) (*VolumeType, error)
	GetVolumeType(ctx context.Context, id uuid.UUID) (*VolumeType, error)
	ListVolumeTypes(ctx context.Context) ([]VolumeType, error)

	// CreateVolume enqueues an async volume creation job. Returns job info for polling.
	// Use SyncCreateVolume for internal synchronous creation (e.g., inside VM build jobs).
	CreateVolume(ctx context.Context, spec CreateVolumeSpec, createdBy string) (*CreateVolumeResponse, error)

	// SyncCreateVolume creates a volume synchronously. Used internally by the compute
	// orchestrator when building VMs.
	SyncCreateVolume(ctx context.Context, spec CreateVolumeSpec) (*Volume, error)

	GetVolume(ctx context.Context, tenantID, volumeID uuid.UUID) (*Volume, error)
	ListVolumesPage(ctx context.Context, tenantID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Volume, error)

	// DeleteVolume enqueues an async volume deletion job. Returns job info for polling.
	// Use SyncDeleteVolume for internal synchronous deletion (e.g., inside VM teardown jobs).
	DeleteVolume(ctx context.Context, tenantID, volumeID uuid.UUID, createdBy string) (*DeleteVolumeResponse, error)

	// SyncDeleteVolume deletes a volume synchronously. Used internally by the compute
	// orchestrator when tearing down VMs.
	SyncDeleteVolume(ctx context.Context, tenantID, volumeID uuid.UUID) error

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
