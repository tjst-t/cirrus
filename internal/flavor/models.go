package flavor

import (
	"time"

	"github.com/google/uuid"
)

// Flavor defines a VM size template (vCPUs, RAM, disk).
type Flavor struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	VCPUs     int       `json:"vcpus"`
	RAMMB     int64     `json:"ram_mb"`
	DiskGB    int64     `json:"disk_gb"`
	IsPublic  bool      `json:"is_public"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateFlavorSpec is the input for creating a flavor.
type CreateFlavorSpec struct {
	Name     string
	VCPUs    int
	RAMMB    int64
	DiskGB   int64
	IsPublic bool
}
