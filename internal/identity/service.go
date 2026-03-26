package identity

import (
	"context"

	"github.com/google/uuid"
)

// Service defines the identity management operations.
type Service interface {
	// Organizations
	CreateOrganization(ctx context.Context, name string) (*Organization, error)
	GetOrganization(ctx context.Context, id uuid.UUID) (*Organization, error)
	ListOrganizations(ctx context.Context) ([]Organization, error)

	// Tenants
	CreateTenant(ctx context.Context, orgID uuid.UUID, name string) (*Tenant, error)
	GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error)
	ListTenants(ctx context.Context, orgID uuid.UUID) ([]Tenant, error)

	// Users
	CreateUser(ctx context.Context, externalID, name, email string) (*User, error)
	GetUser(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByExternalID(ctx context.Context, externalID string) (*User, error)

	// Role assignments
	AssignRole(ctx context.Context, userID uuid.UUID, scopeType ScopeType, scopeID *uuid.UUID, role Role) (*RoleAssignment, error)
	ListRoleAssignments(ctx context.Context, userID uuid.UUID) ([]RoleAssignment, error)
	ListRoleAssignmentsByScope(ctx context.Context, scopeType ScopeType, scopeID uuid.UUID) ([]RoleAssignment, error)
	DeleteRoleAssignment(ctx context.Context, id uuid.UUID) error
}
