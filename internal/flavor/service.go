package flavor

import (
	"context"

	"github.com/google/uuid"
)

// Service provides flavor management operations.
type Service interface {
	// Create creates a new flavor (infra_admin).
	Create(ctx context.Context, spec CreateFlavorSpec) (*Flavor, error)
	// Get returns a flavor by ID.
	Get(ctx context.Context, id uuid.UUID) (*Flavor, error)
	// List returns all public flavors (or all flavors for admin).
	List(ctx context.Context) ([]Flavor, error)
	// Delete removes a flavor (infra_admin).
	Delete(ctx context.Context, id uuid.UUID) error
}
