package network

import (
	"context"

	"github.com/google/uuid"
)

// Service defines network management operations.
type Service interface {
	// Networks
	CreateNetwork(ctx context.Context, tenantID uuid.UUID, spec NetworkSpec) (*Network, error)
	GetNetwork(ctx context.Context, id uuid.UUID) (*Network, error)
	ListNetworks(ctx context.Context, tenantID uuid.UUID) ([]Network, error)
	DeleteNetwork(ctx context.Context, id uuid.UUID) error

	// Ports (read-only for tenants; creation is internal via VM lifecycle)
	GetPort(ctx context.Context, id uuid.UUID) (*Port, error)
	ListPorts(ctx context.Context, networkID uuid.UUID) ([]Port, error)
}
