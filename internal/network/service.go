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

	// Subnets
	CreateSubnet(ctx context.Context, networkID uuid.UUID, spec SubnetSpec) (*Subnet, error)
	GetSubnet(ctx context.Context, id uuid.UUID) (*Subnet, error)
	ListSubnets(ctx context.Context, networkID uuid.UUID) ([]Subnet, error)
	DeleteSubnet(ctx context.Context, id uuid.UUID) error

	// Ports
	CreatePort(ctx context.Context, tenantID uuid.UUID, spec PortSpec) (*Port, error)
	GetPort(ctx context.Context, id uuid.UUID) (*Port, error)
	ListPorts(ctx context.Context, networkID uuid.UUID) ([]Port, error)
	DeletePort(ctx context.Context, id uuid.UUID) error
}
