package host

import (
	"context"

	"github.com/google/uuid"
)

// Service defines host management operations.
type Service interface {
	// Registration
	// id is optional — if nil, a new UUID is generated.
	Register(ctx context.Context, id *uuid.UUID, name, address string) (*Host, error)
	GetHost(ctx context.Context, id uuid.UUID) (*Host, error)
	ListHosts(ctx context.Context) ([]Host, error)

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
