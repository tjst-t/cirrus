package identity

import (
	"context"

	"github.com/google/uuid"
)

// Action represents an API action to authorize.
type Action string

const (
	ActionCreateOrganization Action = "create_organization"
	ActionListOrganizations  Action = "list_organizations"
	ActionGetOrganization    Action = "get_organization"

	ActionCreateTenant Action = "create_tenant"
	ActionListTenants  Action = "list_tenants"
	ActionGetTenant    Action = "get_tenant"

	ActionAssignRole      Action = "assign_role"
	ActionListRoles       Action = "list_roles"
	ActionDeleteRole      Action = "delete_role"

	ActionCreateHost      Action = "create_host"
	ActionListHosts       Action = "list_hosts"
	ActionGetHost         Action = "get_host"
	ActionUpdateHost      Action = "update_host"
	ActionHostAction      Action = "host_action"

	ActionCreateStorageDomain Action = "create_storage_domain"
	ActionListStorageDomains  Action = "list_storage_domains"
	ActionGetStorageDomain    Action = "get_storage_domain"

	ActionCreateNetworkDomain Action = "create_network_domain"
	ActionListNetworkDomains  Action = "list_network_domains"
	ActionGetNetworkDomain    Action = "get_network_domain"

	ActionCreateLocation Action = "create_location"
	ActionListLocations  Action = "list_locations"
	ActionGetLocation    Action = "get_location"

	ActionGetComputePool      Action = "get_compute_pool"
	ActionManageHostTopology  Action = "manage_host_topology"

	// Network actions (tenant-scoped)
	ActionCreateNetwork Action = "create_network"
	ActionListNetworks  Action = "list_networks"
	ActionGetNetwork    Action = "get_network"
	ActionDeleteNetwork Action = "delete_network"

	ActionCreatePort Action = "create_port"
	ActionListPorts  Action = "list_ports"
	ActionGetPort    Action = "get_port"
	ActionDeletePort Action = "delete_port"
)

// Resource represents the target resource of an authorization check.
type Resource struct {
	OrganizationID *uuid.UUID
	TenantID       *uuid.UUID
}

// Decision is the result of an authorization check.
type Decision int

const (
	Allow Decision = iota
	Deny
)

// Authorizer determines whether a user can perform an action on a resource.
type Authorizer interface {
	Authorize(ctx context.Context, user *User, action Action, resource Resource) (Decision, error)
}

// RBACAuthorizer implements fixed role-based access control.
type RBACAuthorizer struct {
	service Service
}

// NewRBACAuthorizer creates a new RBAC authorizer.
func NewRBACAuthorizer(svc Service) *RBACAuthorizer {
	return &RBACAuthorizer{service: svc}
}

func (a *RBACAuthorizer) Authorize(ctx context.Context, user *User, action Action, resource Resource) (Decision, error) {
	assignments, err := a.service.ListRoleAssignments(ctx, user.ID)
	if err != nil {
		return Deny, err
	}

	for _, ra := range assignments {
		if a.checkPermission(ra, action, resource) {
			return Allow, nil
		}
	}
	return Deny, nil
}

func (a *RBACAuthorizer) checkPermission(ra RoleAssignment, action Action, resource Resource) bool {
	switch ra.Role {
	case RoleInfraAdmin:
		// infra_admin can do everything (global scope)
		return true

	case RoleOrgAdmin:
		// org_admin can manage their organization and its tenants
		if ra.ScopeType != ScopeOrganization || ra.ScopeID == nil {
			return false
		}
		switch action {
		case ActionGetOrganization:
			return resource.OrganizationID != nil && *resource.OrganizationID == *ra.ScopeID
		case ActionCreateTenant, ActionListTenants:
			return resource.OrganizationID != nil && *resource.OrganizationID == *ra.ScopeID
		case ActionGetTenant:
			// org_admin can view tenants in their org (caller must verify tenant belongs to org)
			return resource.OrganizationID != nil && *resource.OrganizationID == *ra.ScopeID
		case ActionAssignRole, ActionListRoles, ActionDeleteRole:
			// org_admin can manage roles within their org's tenants
			return resource.OrganizationID != nil && *resource.OrganizationID == *ra.ScopeID
		}

	case RoleTenantAdmin:
		// tenant_admin can manage their tenant
		if ra.ScopeType != ScopeTenant || ra.ScopeID == nil {
			return false
		}
		switch action {
		case ActionGetTenant:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		case ActionAssignRole, ActionListRoles, ActionDeleteRole:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		case ActionCreateNetwork, ActionListNetworks, ActionGetNetwork, ActionDeleteNetwork,
			ActionCreatePort, ActionListPorts, ActionGetPort, ActionDeletePort:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		}

	case RoleTenantMember:
		// tenant_member can read their tenant and use network resources (read-only)
		if ra.ScopeType != ScopeTenant || ra.ScopeID == nil {
			return false
		}
		switch action {
		case ActionGetTenant, ActionListRoles:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		case ActionListNetworks, ActionGetNetwork,
			ActionCreatePort, ActionListPorts, ActionGetPort:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		}
	}
	return false
}
