package network

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service defines network management operations.
type Service interface {
	// Networks
	CreateNetwork(ctx context.Context, tenantID uuid.UUID, spec NetworkSpec) (*Network, error)
	GetNetwork(ctx context.Context, id uuid.UUID) (*Network, error)
	ListNetworks(ctx context.Context, tenantID uuid.UUID) ([]Network, error)
	ListNetworksPage(ctx context.Context, tenantID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Network, error)
	DeleteNetwork(ctx context.Context, id uuid.UUID) error

	// Groups
	CreateGroup(ctx context.Context, networkID uuid.UUID, spec GroupSpec) (*Group, error)
	GetGroup(ctx context.Context, id uuid.UUID) (*Group, error)
	ListGroups(ctx context.Context, networkID uuid.UUID) ([]Group, error)
	ListGroupsPage(ctx context.Context, networkID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Group, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) error

	// Policies
	CreatePolicy(ctx context.Context, networkID uuid.UUID, spec PolicySpec) (*Policy, error)
	GetPolicy(ctx context.Context, id uuid.UUID) (*Policy, error)
	ListPolicies(ctx context.Context, networkID uuid.UUID) ([]Policy, error)
	ListPoliciesPage(ctx context.Context, networkID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Policy, error)
	DeletePolicy(ctx context.Context, id uuid.UUID) error

	// Ports (read-only for tenants; creation is internal via VM lifecycle)
	CreatePort(ctx context.Context, spec PortSpec) (*Port, error)
	GetPort(ctx context.Context, id uuid.UUID) (*Port, error)
	GetPortByVMID(ctx context.Context, vmID uuid.UUID) (*Port, error)
	ListPorts(ctx context.Context, networkID uuid.UUID) ([]Port, error)
	DeletePort(ctx context.Context, id uuid.UUID) error

	// Gateway Nodes (admin operations)
	CreateGatewayNode(ctx context.Context, spec GatewayNodeSpec) (*GatewayNode, error)
	GetGatewayNode(ctx context.Context, id uuid.UUID) (*GatewayNode, error)
	ListGatewayNodes(ctx context.Context) ([]GatewayNode, error)
	DeleteGatewayNode(ctx context.Context, id uuid.UUID) error
	AssignGatewayNode(ctx context.Context, networkID uuid.UUID, gatewayNodeID uuid.UUID) error
	GetNetworkGatewayNode(ctx context.Context, networkID uuid.UUID) (*GatewayNode, error)

	// Egress (tenant operations)
	CreateEgress(ctx context.Context, networkID uuid.UUID, spec EgressSpec) (*Egress, error)
	GetEgress(ctx context.Context, id uuid.UUID) (*Egress, error)
	ListEgresses(ctx context.Context, networkID uuid.UUID) ([]Egress, error)
	UpdateEgressConfig(ctx context.Context, egressID uuid.UUID, config EgressConfig) (*Egress, error)
	DeleteEgress(ctx context.Context, id uuid.UUID) error

	// IP Pools (admin)
	CreateIPPool(ctx context.Context, spec IPPoolSpec) (*IPPool, error)
	GetIPPool(ctx context.Context, id uuid.UUID) (*IPPool, error)
	ListIPPools(ctx context.Context) ([]IPPool, error)
	DeleteIPPool(ctx context.Context, id uuid.UUID) error

	// Ingress (tenant operations)
	CreateIngress(ctx context.Context, networkID uuid.UUID, spec IngressSpec) (*Ingress, error)
	GetIngress(ctx context.Context, id uuid.UUID) (*Ingress, error)
	ListIngresses(ctx context.Context, networkID uuid.UUID) ([]Ingress, error)
	UpdateIngressConfig(ctx context.Context, ingressID uuid.UUID, config IngressConfig) (*Ingress, error)
	DeleteIngress(ctx context.Context, id uuid.UUID) error

	// UpdatePortHost updates the host_id of a port and sets its status to active.
	// Used during failover to rebind a port to the new host after VM relocation.
	UpdatePortHost(ctx context.Context, portID, hostID uuid.UUID) error

	// UpdateBackendHealth updates the healthy state of a backend VM in an l4_lb ingress.
	UpdateBackendHealth(ctx context.Context, ingressID uuid.UUID, vmID uuid.UUID, healthy bool) error

	// Internal Load Balancers (tenant operations)
	CreateLoadBalancer(ctx context.Context, tenantID, networkID uuid.UUID, spec LoadBalancerSpec) (*LoadBalancer, error)
	GetLoadBalancer(ctx context.Context, id uuid.UUID) (*LoadBalancer, error)
	ListLoadBalancers(ctx context.Context, networkID uuid.UUID) ([]LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, id uuid.UUID) error
	UpdateLBBackendHealth(ctx context.Context, lbID uuid.UUID, vmID uuid.UUID, healthy bool) error
}
