package topology

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StorageDomain represents a group of storage backends accessible by a set of hosts.
type StorageDomain struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LocationType represents the hierarchy level of a location.
type LocationType string

const (
	LocationTypeSite  LocationType = "site"
	LocationTypeFloor LocationType = "floor"
	LocationTypeRow   LocationType = "row"
	LocationTypeRack  LocationType = "rack"
	LocationTypeUnit  LocationType = "unit"
)

// IsValidLocationType returns true if the given type is known.
func IsValidLocationType(t LocationType) bool {
	switch t {
	case LocationTypeSite, LocationTypeFloor, LocationTypeRow, LocationTypeRack, LocationTypeUnit:
		return true
	}
	return false
}

// Location represents a node in the failure topology tree.
type Location struct {
	ID              uuid.UUID        `json:"id"`
	ParentID        *uuid.UUID       `json:"parent_id,omitempty"`
	Name            string           `json:"name"`
	Type            LocationType     `json:"type"`
	FaultAttributes json.RawMessage  `json:"fault_attributes,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	Children        []*Location      `json:"children,omitempty"`
}

// FaultDomain represents a group of hosts under a location at a specified hierarchy level.
// Used for operational purposes: rollout planning, failure analysis, canary deployments.
type FaultDomain struct {
	LocationID   uuid.UUID    `json:"location_id"`
	LocationName string       `json:"location_name"`
	Level        LocationType `json:"level"`
	HostIDs      []uuid.UUID  `json:"host_ids"`
	Count        int          `json:"count"`
}

// ComputePool is the set of hosts associated with a storage domain.
type ComputePool struct {
	StorageDomainID   uuid.UUID   `json:"storage_domain_id"`
	StorageDomainName string      `json:"storage_domain_name"`
	HostIDs           []uuid.UUID `json:"host_ids"`
	Count             int         `json:"count"`
}
