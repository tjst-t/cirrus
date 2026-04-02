package host

import (
	"context"

	"github.com/google/uuid"
)

// Service defines host management operations.
type Service interface {
	// RegisterOrGet performs idempotent registration: if a host with the same name
	// already exists, returns the existing host (created=false); otherwise creates
	// a new one (created=true).
	RegisterOrGet(ctx context.Context, name, address, workerGRPCAddr, fabricIP, capability string) (h *Host, created bool, err error)
	GetHost(ctx context.Context, id uuid.UUID) (*Host, error)
	ListHosts(ctx context.Context) ([]Host, error)
	ListHostsByState(ctx context.Context, state OperationalState) ([]Host, error)
	DeleteHost(ctx context.Context, id uuid.UUID) error

	// Capability and resource management
	UpdateCapability(ctx context.Context, hostID uuid.UUID, capability []byte) error
	UpdateResourcePhysical(ctx context.Context, hostID uuid.UUID, resources []byte) error
	UpdateOvercommitRatios(ctx context.Context, hostID uuid.UUID, ratios []byte) error

	// Operational state management
	SetOperationalState(ctx context.Context, hostID uuid.UUID, state OperationalState) error

	// Heartbeat from worker
	Heartbeat(ctx context.Context, hostID string, report ResourceReport) error

	// Resource availability (used by scheduler)
	GetAllocatable(ctx context.Context, hostID uuid.UUID) (*AllocatableResources, error)
}
