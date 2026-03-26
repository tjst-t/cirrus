package identity

import (
	"time"

	"github.com/google/uuid"
)

// Organization is the top-level billing/contract entity.
type Organization struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	QuotaVcpus   int       `json:"quota_vcpus"`
	QuotaRAMMB   int       `json:"quota_ram_mb"`
	QuotaVolumeGB int      `json:"quota_volume_gb"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Tenant is a resource-owning unit within an organization.
type Tenant struct {
	ID              uuid.UUID `json:"id"`
	OrganizationID  uuid.UUID `json:"organization_id"`
	Name            string    `json:"name"`
	QuotaVcpus      int       `json:"quota_vcpus"`
	QuotaRAMMB      int       `json:"quota_ram_mb"`
	QuotaVolumeGB   int       `json:"quota_volume_gb"`
	QuotaVMs        int       `json:"quota_vms"`
	QuotaVolumes    int       `json:"quota_volumes"`
	QuotaSnapshots  int       `json:"quota_snapshots"`
	QuotaNetworks   int       `json:"quota_networks"`
	QuotaFloatingIPs int      `json:"quota_floating_ips"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// User is an authenticated identity.
type User struct {
	ID         uuid.UUID `json:"id"`
	ExternalID string    `json:"external_id"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Role represents a fixed RBAC role.
type Role string

const (
	RoleInfraAdmin   Role = "infra_admin"
	RoleOrgAdmin     Role = "org_admin"
	RoleTenantAdmin  Role = "tenant_admin"
	RoleTenantMember Role = "tenant_member"
)

// ScopeType defines the level at which a role is assigned.
type ScopeType string

const (
	ScopeGlobal       ScopeType = "global"
	ScopeOrganization ScopeType = "organization"
	ScopeTenant       ScopeType = "tenant"
)

// RoleAssignment binds a user to a role within a scope.
type RoleAssignment struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	ScopeType ScopeType  `json:"scope_type"`
	ScopeID   *uuid.UUID `json:"scope_id,omitempty"`
	Role      Role       `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
}
