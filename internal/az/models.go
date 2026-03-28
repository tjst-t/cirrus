package az

import (
	"time"

	"github.com/google/uuid"
)

// AvailabilityZone represents a tenant-facing placement abstraction.
// Each AZ maps 1:1 to a Network Domain (OVN cluster) and references a Location.
type AvailabilityZone struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	LocationID      uuid.UUID `json:"location_id"`
	NetworkDomainID uuid.UUID `json:"network_domain_id"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
