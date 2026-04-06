package network

import (
	"time"

	"github.com/google/uuid"
)

// Egress and Ingress type constants.
const (
	EgressTypeNATGateway    = "nat_gateway"
	EgressTypeVPNIPsec      = "vpn_ipsec"
	EgressTypeVPNWireGuard  = "vpn_wireguard"
	EgressTypeDirectConnect = "direct_connect"
	IngressTypeDirectIP     = "direct_ip"
)

// NetworkStatus represents the lifecycle state of a network.
type NetworkStatus string

const (
	NetworkStatusCreating NetworkStatus = "creating"
	NetworkStatusActive   NetworkStatus = "active"
	NetworkStatusDeleting NetworkStatus = "deleting"
	NetworkStatusError    NetworkStatus = "error"
)

// PortStatus represents the lifecycle state of a port.
type PortStatus string

const (
	PortStatusCreating PortStatus = "creating"
	PortStatusDown     PortStatus = "down"
	PortStatusActive   PortStatus = "active"
	PortStatusDeleting PortStatus = "deleting"
	PortStatusError    PortStatus = "error"
)

// Network represents a tenant virtual network.
type Network struct {
	ID        uuid.UUID     `json:"id"`
	TenantID  uuid.UUID     `json:"tenant_id"`
	Name      string        `json:"name"`
	CIDR      string        `json:"cidr"`
	VNI       int           `json:"vni"`
	Status    NetworkStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Port represents a virtual network port.
type Port struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	NetworkID  uuid.UUID  `json:"network_id"`
	GroupID    *uuid.UUID `json:"group_id,omitempty"`
	VMID       *uuid.UUID `json:"vm_id,omitempty"`
	VMName     string     `json:"vm_name,omitempty"`
	MACAddress string     `json:"mac_address"`
	IPAddress  string     `json:"ip_address"`
	HostID     *uuid.UUID `json:"host_id,omitempty"`
	Role       string     `json:"role"`
	Status     PortStatus `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Group represents a collection of VMs within a network for policy targeting.
type Group struct {
	ID        uuid.UUID `json:"id"`
	NetworkID uuid.UUID `json:"network_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Policy represents a communication rule between groups within a network.
type Policy struct {
	ID         uuid.UUID `json:"id"`
	NetworkID  uuid.UUID `json:"network_id"`
	SrcGroupID uuid.UUID `json:"src_group_id"`
	DstGroupID uuid.UUID `json:"dst_group_id"`
	Protocol   string    `json:"protocol"`
	DstPort    *int      `json:"dst_port,omitempty"`
	Priority   int       `json:"priority"`
	Action     string    `json:"action"`
	CreatedAt  time.Time `json:"created_at"`
}

// NetworkSpec is the input for creating a new network.
type NetworkSpec struct {
	Name string `json:"name"`
	CIDR string `json:"cidr,omitempty"`
}

// GroupSpec is the input for creating a new group.
type GroupSpec struct {
	Name string `json:"name"`
}

// PortSpec is the input for creating a port internally (VM lifecycle / tests).
type PortSpec struct {
	TenantID  uuid.UUID  `json:"tenant_id"`
	NetworkID uuid.UUID  `json:"network_id"`
	GroupID   uuid.UUID  `json:"group_id"`
	HostID    uuid.UUID  `json:"host_id"`
	VMID      *uuid.UUID `json:"vm_id,omitempty"`
	VMName    string     `json:"vm_name"`
}

// PolicySpec is the input for creating a new policy.
type PolicySpec struct {
	SrcGroupID uuid.UUID `json:"src_group_id"`
	DstGroupID uuid.UUID `json:"dst_group_id"`
	Protocol   string    `json:"protocol"`
	DstPort    *int      `json:"dst_port,omitempty"`
	Priority   int       `json:"priority,omitempty"`
	Action     string    `json:"action,omitempty"`
}

// GatewayNode represents a host with gateway capability.
type GatewayNode struct {
	ID         uuid.UUID `json:"id"`
	HostID     uuid.UUID `json:"host_id"`
	ExternalIP string    `json:"external_ip"`            // Public-facing IP for SNAT/DNAT
	InternalIP string    `json:"internal_ip"`            // Fabric IP for Geneve tunnel
	UplinkPort string    `json:"uplink_port,omitempty"` // Physical uplink port for Direct Connect
	Status     string    `json:"status"`                 // "active", "draining", "retired"
	CreatedAt  time.Time `json:"created_at"`
}

// GatewayNodeSpec is the input for creating a gateway node.
type GatewayNodeSpec struct {
	HostID     uuid.UUID `json:"host_id"`
	ExternalIP string    `json:"external_ip"`
	InternalIP string    `json:"internal_ip"`
	UplinkPort string    `json:"uplink_port,omitempty"` // Physical uplink port for Direct Connect (GW-role hosts only)
}

// Egress represents a network egress rule (e.g. NAT gateway SNAT).
type Egress struct {
	ID        uuid.UUID    `json:"id"`
	NetworkID uuid.UUID    `json:"network_id"`
	Type      string       `json:"type"`   // "nat_gateway"
	Config    EgressConfig `json:"config"`
}

// EgressConfig holds type-specific egress configuration.
type EgressConfig struct {
	PublicIP      string               `json:"public_ip,omitempty"`      // For nat_gateway: the SNAT public IP
	VPNIPsec      *VPNIPsecConfig      `json:"vpn_ipsec,omitempty"`      // For vpn_ipsec type
	VPNWireGuard  *VPNWireGuardConfig  `json:"vpn_wireguard,omitempty"`  // For vpn_wireguard type
	DirectConnect *DirectConnectConfig `json:"direct_connect,omitempty"` // For direct_connect type
}

// DirectConnectConfig holds VLAN trunk configuration for direct Layer-2 connectivity.
type DirectConnectConfig struct {
	VLANID     int    `json:"vlan_id"`      // VLAN tag for this tenant segment (1-4094)
	UplinkPort string `json:"uplink_port"`  // Physical port name, inherited from GW node registration
}

// VPNIPsecConfig holds IKEv2 IPsec tunnel configuration.
type VPNIPsecConfig struct {
	PeerIP          string `json:"peer_ip"`                     // Remote IPsec peer address
	PreSharedKey    string `json:"pre_shared_key,omitempty"`    // IKEv2 pre-shared key (plaintext input only; cleared after encryption)
	PreSharedKeyEnc string `json:"pre_shared_key_enc,omitempty"` // AES-GCM encrypted PSK (base64); stored in DB
	LocalCIDR       string `json:"local_cidr"`                  // Tenant network CIDR
	RemoteCIDR      string `json:"remote_cidr"`                 // On-prem CIDR
}

// VPNWireGuardConfig holds WireGuard tunnel configuration.
type VPNWireGuardConfig struct {
	PrivateKeyEnc string   `json:"private_key_enc"` // AES-GCM encrypted private key (base64)
	PublicKey     string   `json:"public_key"`      // WireGuard public key (base64, for tenant to read)
	PeerPublicKey string   `json:"peer_public_key"` // Peer's WireGuard public key
	PeerEndpoint  string   `json:"peer_endpoint"`   // Peer host:port
	AllowedIPs    []string `json:"allowed_ips"`     // CIDRs routed through tunnel
	ListenPort    int      `json:"listen_port"`     // WireGuard listen port
}

// EgressSpec is the input for creating a new egress rule.
type EgressSpec struct {
	Type   string       `json:"type"`
	Config EgressConfig `json:"config"`
}

// IPPool represents a pool of public IP addresses managed by the admin.
type IPPool struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	CIDR        string    `json:"cidr"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// IPPoolSpec is the input for creating a new IP pool.
type IPPoolSpec struct {
	Name        string `json:"name"`
	CIDR        string `json:"cidr"`
	Description string `json:"description,omitempty"`
}

// Ingress represents an external traffic entry rule (e.g. Direct IP DNAT).
type Ingress struct {
	ID        uuid.UUID     `json:"id"`
	NetworkID uuid.UUID     `json:"network_id"`
	Type      string        `json:"type"`    // "direct_ip"
	PublicIP  string        `json:"public_ip"`
	IPPoolID  *uuid.UUID    `json:"ip_pool_id,omitempty"`
	Config    IngressConfig `json:"config"`
	CreatedAt time.Time     `json:"created_at"`
}

// IngressConfig holds type-specific ingress configuration.
type IngressConfig struct {
	TargetVMID string `json:"target_vm_id"` // UUID of the VM to DNAT to
	TargetIP   string `json:"target_ip"`    // Private IP of the VM (resolved at create time)
}

// IngressSpec is the input for creating a new ingress rule.
type IngressSpec struct {
	Type     string        `json:"type"`
	PublicIP string        `json:"public_ip"`  // Must be within an ip_pool CIDR
	IPPoolID uuid.UUID     `json:"ip_pool_id"`
	Config   IngressConfig `json:"config"`
}
