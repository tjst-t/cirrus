// Package quota provides hierarchical quota enforcement (org → tenant).
// The reservation pattern is: Check → Reserve → (Commit | Release).
//   - Check: verify limits are not exceeded (read-only, no lock).
//   - Reserve: atomically write a reservation row and verify limits.
//   - Commit: convert reservation to committed usage and delete the reservation row.
//   - Release: delete the reservation row without updating usage.
package quota

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ResourceDelta describes the resource amounts involved in one operation.
type ResourceDelta struct {
	Vcpus     int
	RAMMB     int
	VolumeGB  int
	VMs       int
	Volumes   int
	Snapshots int
	Networks  int
}

// Usage is the current committed usage for a tenant.
type Usage struct {
	TenantID      uuid.UUID
	VcpusUsed     int
	RAMMBUsed     int
	VolumeGBUsed  int
	VMsCount      int
	VolumesCount  int
	SnapshotsCount int
	NetworksCount int
}

// Limits holds the quota limits for a tenant or org.
type Limits struct {
	Vcpus     int
	RAMMB     int
	VolumeGB  int
	VMs       int
	Volumes   int
	Snapshots int
	Networks  int
}

// ErrQuotaExceeded is returned when a Check or Reserve operation would exceed a limit.
var ErrQuotaExceeded = errors.New("quota exceeded")

// ErrReservationNotFound is returned when Release/Commit targets a non-existent reservation.
var ErrReservationNotFound = errors.New("reservation not found")

// ErrNotFound is returned when GetTenantLimits/GetOrgLimits targets a non-existent entity.
var ErrNotFound = errors.New("not found")

// ResourceType identifies the kind of resource being reserved.
type ResourceType string

const (
	ResourceTypeVM       ResourceType = "vm"
	ResourceTypeVolume   ResourceType = "volume"
	ResourceTypeSnapshot ResourceType = "snapshot"
	ResourceTypeNetwork  ResourceType = "network"
)

// Service defines quota check, reservation, and usage tracking operations.
type Service interface {
	// Check verifies that adding delta to the tenant's current usage (including
	// in-flight reserves) would not exceed tenant or org limits. Returns
	// ErrQuotaExceeded if any limit would be breached. Read-only; no state change.
	Check(ctx context.Context, tenantID uuid.UUID, delta ResourceDelta) error

	// Reserve atomically creates a reservation for resourceID of resourceType,
	// then verifies limits. Returns ErrQuotaExceeded if the reservation would
	// breach limits (and does not persist in that case).
	Reserve(ctx context.Context, tenantID uuid.UUID, resourceType ResourceType, resourceID uuid.UUID, delta ResourceDelta) error

	// Commit moves a reservation into committed usage and deletes the reservation row.
	// Returns ErrReservationNotFound if no matching reservation exists.
	Commit(ctx context.Context, resourceType ResourceType, resourceID uuid.UUID) error

	// Release deletes a reservation without updating committed usage (used on failure paths).
	// Returns ErrReservationNotFound if no matching reservation exists.
	Release(ctx context.Context, resourceType ResourceType, resourceID uuid.UUID) error

	// Decommit decrements committed usage when a resource is destroyed (no prior reservation).
	// Used on the deletion path. A limit of 0 means the field is not decremented.
	Decommit(ctx context.Context, tenantID uuid.UUID, delta ResourceDelta) error

	// GetUsage returns the current committed usage for a tenant.
	GetUsage(ctx context.Context, tenantID uuid.UUID) (*Usage, error)

	// SetTenantLimits updates the quota limits for a tenant.
	SetTenantLimits(ctx context.Context, tenantID uuid.UUID, limits Limits) error

	// GetTenantLimits returns the quota limits for a tenant.
	GetTenantLimits(ctx context.Context, tenantID uuid.UUID) (*Limits, error)

	// SetOrgLimits updates the quota limits for an organization.
	SetOrgLimits(ctx context.Context, orgID uuid.UUID, limits Limits) error

	// GetOrgLimits returns the quota limits for an organization.
	GetOrgLimits(ctx context.Context, orgID uuid.UUID) (*Limits, error)
}
