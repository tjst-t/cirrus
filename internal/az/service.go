package az

import (
	"context"

	"github.com/google/uuid"
)

// Service defines availability zone management operations.
type Service interface {
	Create(ctx context.Context, name, description string, locationID, networkDomainID uuid.UUID) (*AvailabilityZone, error)
	Get(ctx context.Context, id uuid.UUID) (*AvailabilityZone, error)
	List(ctx context.Context) ([]AvailabilityZone, error)
	ListEnabled(ctx context.Context) ([]AvailabilityZone, error)
	Update(ctx context.Context, id uuid.UUID, name, description string, enabled *bool) (*AvailabilityZone, error)
	Delete(ctx context.Context, id uuid.UUID) error

	// Storage domain associations
	AddStorageDomain(ctx context.Context, azID, storageDomainID uuid.UUID) error
	RemoveStorageDomain(ctx context.Context, azID, storageDomainID uuid.UUID) error
	ListStorageDomains(ctx context.Context, azID uuid.UUID) ([]uuid.UUID, error)

	// Resolve: find AZ by network_domain_id (used by network service)
	GetByNetworkDomain(ctx context.Context, networkDomainID uuid.UUID) (*AvailabilityZone, error)

	// GetDefault returns the single default AZ (Phase 1: only one AZ exists).
	// Returns ErrNotFound if no enabled AZ exists.
	GetDefault(ctx context.Context) (*AvailabilityZone, error)
}
