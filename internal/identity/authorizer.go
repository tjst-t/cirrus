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

	ActionCreateLocation Action = "create_location"
	ActionListLocations  Action = "list_locations"
	ActionGetLocation    Action = "get_location"

	ActionGetComputePool      Action = "get_compute_pool"
	ActionManageHostTopology  Action = "manage_host_topology"

	// Availability Zone actions (infra_admin)
	ActionCreateAZ Action = "create_az"
	ActionUpdateAZ Action = "update_az"
	ActionDeleteAZ Action = "delete_az"
	ActionListAZs  Action = "list_azs"

	// Network actions (tenant-scoped)
	ActionCreateNetwork Action = "create_network"
	ActionListNetworks  Action = "list_networks"
	ActionGetNetwork    Action = "get_network"
	ActionDeleteNetwork Action = "delete_network"

	ActionListPorts Action = "list_ports"
	ActionGetPort   Action = "get_port"

	// Group actions (tenant-scoped)
	ActionCreateGroup Action = "create_group"
	ActionListGroups  Action = "list_groups"
	ActionGetGroup    Action = "get_group"
	ActionDeleteGroup Action = "delete_group"

	// Policy actions (tenant-scoped)
	ActionCreatePolicy  Action = "create_policy"
	ActionListPolicies  Action = "list_policies"
	ActionGetPolicy     Action = "get_policy"
	ActionDeletePolicy  Action = "delete_policy"

	// Storage backend actions (infra_admin)
	ActionCreateStorageBackend Action = "create_storage_backend"
	ActionListStorageBackends  Action = "list_storage_backends"
	ActionGetStorageBackend    Action = "get_storage_backend"
	ActionDrainStorageBackend  Action = "drain_storage_backend"

	// Volume type actions (infra_admin: create; tenant: list)
	ActionCreateVolumeType Action = "create_volume_type"
	ActionListVolumeTypes  Action = "list_volume_types"
	ActionGetVolumeType    Action = "get_volume_type"

	// Volume actions (tenant-scoped)
	ActionCreateVolume Action = "create_volume"
	ActionListVolumes  Action = "list_volumes"
	ActionGetVolume    Action = "get_volume"
	ActionDeleteVolume Action = "delete_volume"
	ActionResizeVolume Action = "resize_volume"

	// Flavor actions (infra_admin: create/delete; all authenticated: list/get)
	ActionCreateFlavor Action = "create_flavor"
	ActionListFlavors  Action = "list_flavors"
	ActionGetFlavor    Action = "get_flavor"
	ActionDeleteFlavor Action = "delete_flavor"

	// VM actions (tenant-scoped)
	ActionCreateVM Action = "create_vm"
	ActionListVMs  Action = "list_vms"
	ActionGetVM    Action = "get_vm"
	ActionDeleteVM Action = "delete_vm"
	ActionStartVM  Action = "start_vm"
	ActionStopVM   Action = "stop_vm"
	ActionRebootVM Action = "reboot_vm"
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
			ActionListPorts, ActionGetPort,
			ActionCreateGroup, ActionListGroups, ActionGetGroup, ActionDeleteGroup,
			ActionCreatePolicy, ActionListPolicies, ActionGetPolicy, ActionDeletePolicy,
			ActionListVolumeTypes, ActionGetVolumeType,
			ActionCreateVolume, ActionListVolumes, ActionGetVolume, ActionDeleteVolume, ActionResizeVolume,
			ActionListFlavors, ActionGetFlavor,
			ActionCreateVM, ActionListVMs, ActionGetVM, ActionDeleteVM, ActionStartVM, ActionStopVM, ActionRebootVM:
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
			ActionListPorts, ActionGetPort,
			ActionListGroups, ActionGetGroup,
			ActionListPolicies, ActionGetPolicy,
			ActionListVolumeTypes, ActionGetVolumeType,
			ActionListVolumes, ActionGetVolume,
			ActionListFlavors, ActionGetFlavor,
			ActionListVMs, ActionGetVM:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		}
	}
	return false
}
