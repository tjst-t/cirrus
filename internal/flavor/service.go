package flavor

import (
	"context"
	"time"

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
	// ListPage returns a page of flavors ordered by (created_at, id).
	// Pass zero values to start from the beginning.
	ListPage(ctx context.Context, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Flavor, error)
	// Delete removes a flavor (infra_admin).
	Delete(ctx context.Context, id uuid.UUID) error
}
