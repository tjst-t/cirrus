package ovn

import (
	"context"
)

// LogicalSwitchPort holds the fields needed to create an LSP in OVN.
type LogicalSwitchPort struct {
	Name         string   // port UUID as string
	MACAddress   string   // "02:xx:xx:xx:xx:xx"
	IPAddress    string   // "10.100.0.5"
	PortSecurity []string // ["02:xx:xx:xx:xx:xx 10.100.0.5"]
}

// DHCPOptions holds the fields for an OVN DHCP_Options row.
type DHCPOptions struct {
	CIDR       string            // "10.100.0.0/24"
	Options    map[string]string // e.g. server_id, server_mac, lease_time, dns_server
	ExternalID string            // subnet UUID for lookup
}

// Client defines the interface for OVN Northbound DB operations.
type Client interface {
	// Logical Switch (Network)
	CreateLogicalSwitch(ctx context.Context, name string) error
	DeleteLogicalSwitch(ctx context.Context, name string) error
	ListLogicalSwitches(ctx context.Context) ([]string, error)

	// Logical Switch Port (Port)
	CreateLogicalSwitchPort(ctx context.Context, switchName string, port LogicalSwitchPort) error
	DeleteLogicalSwitchPort(ctx context.Context, portName string) error
	// ListAllLogicalSwitchPorts returns the names of all LSPs across all switches.
	ListAllLogicalSwitchPorts(ctx context.Context) ([]string, error)

	// DHCP Options (Subnet)
	CreateDHCPOptions(ctx context.Context, opts DHCPOptions) (string, error) // returns UUID
	// DeleteDHCPOptions deletes DHCP options by subnet external ID.
	DeleteDHCPOptions(ctx context.Context, subnetExternalID string) error

	// Connection
	Close() error
}
