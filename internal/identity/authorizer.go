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
	ActionCreateVM    Action = "create_vm"
	ActionListVMs     Action = "list_vms"
	ActionGetVM       Action = "get_vm"
	ActionDeleteVM    Action = "delete_vm"
	ActionStartVM     Action = "start_vm"
	ActionStopVM      Action = "stop_vm"
	ActionForceStopVM Action = "force_stop_vm"
	ActionRebootVM    Action = "reboot_vm"
	ActionMigrateVM   Action = "migrate_vm"
	ActionRepairVM    Action = "repair_vm" // admin only

	ActionGetQuota Action = "get_quota"
	ActionSetQuota Action = "set_quota"

	// Gateway node actions (infra_admin)
	ActionCreateGatewayNode       Action = "create_gateway_node"
	ActionListGatewayNodes        Action = "list_gateway_nodes"
	ActionGetGatewayNode          Action = "get_gateway_node"
	ActionDeleteGatewayNode       Action = "delete_gateway_node"
	ActionAssignGatewayNode       Action = "assign_gateway_node"
	ActionGetNetworkGatewayNode   Action = "get_network_gateway_node"

	// Egress actions (tenant-scoped)
	ActionCreateEgress Action = "create_egress"
	ActionListEgresses Action = "list_egresses"
	ActionGetEgress    Action = "get_egress"
	ActionDeleteEgress Action = "delete_egress"

	// IP Pool actions (infra_admin)
	ActionCreateIPPool Action = "create_ip_pool"
	ActionListIPPools  Action = "list_ip_pools"
	ActionGetIPPool    Action = "get_ip_pool"
	ActionDeleteIPPool Action = "delete_ip_pool"

	// Ingress actions (tenant-scoped)
	ActionCreateIngress Action = "create_ingress"
	ActionListIngresses Action = "list_ingresses"
	ActionGetIngress    Action = "get_ingress"
	ActionDeleteIngress Action = "delete_ingress"

	// Internal Load Balancer actions (tenant-scoped)
	ActionCreateLoadBalancer Action = "create_load_balancer"
	ActionListLoadBalancers  Action = "list_load_balancers"
	ActionGetLoadBalancer    Action = "get_load_balancer"
	ActionDeleteLoadBalancer Action = "delete_load_balancer"

	// Drift event actions (infra_admin)
	ActionListDriftEvents   Action = "list_drift_events"
	ActionResolveDriftEvent Action = "resolve_drift_event"

	// DRS actions (infra_admin)
	ActionDRSRun    Action = "drs_run"
	ActionDRSStatus Action = "drs_status"
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
		case ActionGetQuota, ActionSetQuota:
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
			ActionCreateEgress, ActionListEgresses, ActionGetEgress, ActionDeleteEgress,
			ActionCreateIngress, ActionListIngresses, ActionGetIngress, ActionDeleteIngress,
			ActionCreateLoadBalancer, ActionListLoadBalancers, ActionGetLoadBalancer, ActionDeleteLoadBalancer,
			ActionListVolumeTypes, ActionGetVolumeType,
			ActionCreateVolume, ActionListVolumes, ActionGetVolume, ActionDeleteVolume, ActionResizeVolume,
			ActionListFlavors, ActionGetFlavor,
			ActionCreateVM, ActionListVMs, ActionGetVM, ActionDeleteVM,
			ActionStartVM, ActionStopVM, ActionForceStopVM, ActionRebootVM, ActionMigrateVM,
			ActionGetQuota:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		case ActionSetQuota:
			// only org_admin or infra_admin may set quotas on a tenant
			return false
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
			ActionListEgresses, ActionGetEgress,
			ActionListIngresses, ActionGetIngress,
			ActionListLoadBalancers, ActionGetLoadBalancer,
			ActionListVolumeTypes, ActionGetVolumeType,
			ActionListVolumes, ActionGetVolume,
			ActionListFlavors, ActionGetFlavor,
			ActionListVMs, ActionGetVM:
			return resource.TenantID != nil && *resource.TenantID == *ra.ScopeID
		}
	}
	return false
}
