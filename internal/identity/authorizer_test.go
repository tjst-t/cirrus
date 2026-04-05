package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/identity"
)

// mockService implements identity.Service for testing.
type mockService struct {
	assignments []identity.RoleAssignment
}

func (m *mockService) CreateOrganization(ctx context.Context, name string) (*identity.Organization, error) {
	return nil, nil
}
func (m *mockService) GetOrganization(ctx context.Context, id uuid.UUID) (*identity.Organization, error) {
	return nil, nil
}
func (m *mockService) ListOrganizations(ctx context.Context) ([]identity.Organization, error) {
	return nil, nil
}
func (m *mockService) ListOrganizationsPage(_ context.Context, _ time.Time, _ uuid.UUID, _ int) ([]identity.Organization, error) {
	return nil, nil
}
func (m *mockService) CreateTenant(ctx context.Context, orgID uuid.UUID, name string) (*identity.Tenant, error) {
	return nil, nil
}
func (m *mockService) GetTenant(ctx context.Context, id uuid.UUID) (*identity.Tenant, error) {
	return nil, nil
}
func (m *mockService) ListTenants(ctx context.Context, orgID uuid.UUID) ([]identity.Tenant, error) {
	return nil, nil
}
func (m *mockService) ListTenantsPage(_ context.Context, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]identity.Tenant, error) {
	return nil, nil
}
func (m *mockService) CreateUser(ctx context.Context, externalID, name, email string) (*identity.User, error) {
	return nil, nil
}
func (m *mockService) GetUser(ctx context.Context, id uuid.UUID) (*identity.User, error) {
	return nil, nil
}
func (m *mockService) GetUserByExternalID(ctx context.Context, externalID string) (*identity.User, error) {
	return nil, nil
}
func (m *mockService) AssignRole(ctx context.Context, userID uuid.UUID, scopeType identity.ScopeType, scopeID *uuid.UUID, role identity.Role) (*identity.RoleAssignment, error) {
	return nil, nil
}
func (m *mockService) ListRoleAssignments(ctx context.Context, userID uuid.UUID) ([]identity.RoleAssignment, error) {
	return m.assignments, nil
}
func (m *mockService) ListRoleAssignmentsByScope(ctx context.Context, scopeType identity.ScopeType, scopeID uuid.UUID) ([]identity.RoleAssignment, error) {
	return nil, nil
}
func (m *mockService) DeleteRoleAssignment(ctx context.Context, id uuid.UUID) error {
	return nil
}

func TestRBACAuthorizer_InfraAdmin(t *testing.T) {
	userID := uuid.New()
	svc := &mockService{
		assignments: []identity.RoleAssignment{
			{UserID: userID, ScopeType: identity.ScopeGlobal, Role: identity.RoleInfraAdmin},
		},
	}
	authz := identity.NewRBACAuthorizer(svc)
	user := &identity.User{ID: userID}

	decision, err := authz.Authorize(context.Background(), user, identity.ActionCreateOrganization, identity.Resource{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != identity.Allow {
		t.Fatal("expected Allow for infra_admin")
	}
}

func TestRBACAuthorizer_OrgAdmin_CanCreateTenant(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	svc := &mockService{
		assignments: []identity.RoleAssignment{
			{UserID: userID, ScopeType: identity.ScopeOrganization, ScopeID: &orgID, Role: identity.RoleOrgAdmin},
		},
	}
	authz := identity.NewRBACAuthorizer(svc)
	user := &identity.User{ID: userID}

	decision, err := authz.Authorize(context.Background(), user, identity.ActionCreateTenant, identity.Resource{OrganizationID: &orgID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != identity.Allow {
		t.Fatal("expected Allow for org_admin creating tenant in their org")
	}
}

func TestRBACAuthorizer_OrgAdmin_DeniedOtherOrg(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	otherOrgID := uuid.New()
	svc := &mockService{
		assignments: []identity.RoleAssignment{
			{UserID: userID, ScopeType: identity.ScopeOrganization, ScopeID: &orgID, Role: identity.RoleOrgAdmin},
		},
	}
	authz := identity.NewRBACAuthorizer(svc)
	user := &identity.User{ID: userID}

	decision, err := authz.Authorize(context.Background(), user, identity.ActionCreateTenant, identity.Resource{OrganizationID: &otherOrgID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != identity.Deny {
		t.Fatal("expected Deny for org_admin in other org")
	}
}

func TestRBACAuthorizer_TenantMember_DeniedAssignRole(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	svc := &mockService{
		assignments: []identity.RoleAssignment{
			{UserID: userID, ScopeType: identity.ScopeTenant, ScopeID: &tenantID, Role: identity.RoleTenantMember},
		},
	}
	authz := identity.NewRBACAuthorizer(svc)
	user := &identity.User{ID: userID}

	decision, err := authz.Authorize(context.Background(), user, identity.ActionAssignRole, identity.Resource{TenantID: &tenantID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != identity.Deny {
		t.Fatal("expected Deny for tenant_member assigning roles")
	}
}

func TestRBACAuthorizer_NoRoles_Denied(t *testing.T) {
	svc := &mockService{assignments: nil}
	authz := identity.NewRBACAuthorizer(svc)
	user := &identity.User{ID: uuid.New()}

	decision, err := authz.Authorize(context.Background(), user, identity.ActionListOrganizations, identity.Resource{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != identity.Deny {
		t.Fatal("expected Deny for user with no roles")
	}
}
