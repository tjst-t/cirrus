package topology

import (
	"context"

	"github.com/google/uuid"
)

// Service defines topology management operations.
type Service interface {
	// Storage domains
	CreateStorageDomain(ctx context.Context, name string) (*StorageDomain, error)
	GetStorageDomain(ctx context.Context, id uuid.UUID) (*StorageDomain, error)
	ListStorageDomains(ctx context.Context) ([]StorageDomain, error)

	// Network domains
	CreateNetworkDomain(ctx context.Context, name, ovnNBConnection string) (*NetworkDomain, error)
	GetNetworkDomain(ctx context.Context, id uuid.UUID) (*NetworkDomain, error)
	ListNetworkDomains(ctx context.Context) ([]NetworkDomain, error)

	// Locations
	CreateLocation(ctx context.Context, parentID *uuid.UUID, name string, locType LocationType, faultAttrs []byte) (*Location, error)
	GetLocation(ctx context.Context, id uuid.UUID) (*Location, error)
	ListLocations(ctx context.Context) ([]Location, error)
	GetLocationPath(ctx context.Context, id uuid.UUID) ([]Location, error)
	GetLocationTree(ctx context.Context, id uuid.UUID) (*Location, error)

	// Host-domain associations
	AssociateHostStorageDomain(ctx context.Context, hostID, storageDomainID uuid.UUID) error
	DissociateHostStorageDomain(ctx context.Context, hostID, storageDomainID uuid.UUID) error
	SetHostNetworkDomain(ctx context.Context, hostID, networkDomainID uuid.UUID) error
	SetHostLocation(ctx context.Context, hostID, locationID uuid.UUID) error

	// Compute pool derivation
	GetComputePool(ctx context.Context, storageDomainID, networkDomainID uuid.UUID) (*ComputePool, error)

	// Reachability queries
	ListReachableHosts(ctx context.Context, storageDomainID uuid.UUID) ([]uuid.UUID, error)
	ListReachableBackends(ctx context.Context, hostID uuid.UUID) ([]uuid.UUID, error)
}
