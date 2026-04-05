// Package agent implements the worker-side network agent that manages
// OVS flows, DHCP, DNS, and metadata services based on HostNetworkState
// received from the controller.
package agent

// FlowEntry represents a single OpenFlow flow rule.
type FlowEntry struct {
	Table    int
	Priority int
	Match    string
	Actions  string
}

// OVSClient defines the interface for OVS operations used by the network agent.
// Implementations include the real OpenFlow client (ovs_openflow.go) and the
// mock client (test/mock/ovs/) for testing.
type OVSClient interface {
	// AddFlow adds a single flow entry.
	AddFlow(table int, priority int, match string, actions string) error

	// DeleteFlow removes flow entries matching the given table and match criteria.
	DeleteFlow(table int, match string) error

	// AddFlowBundle atomically adds multiple flow entries.
	// Either all flows are applied or none are (transactional).
	AddFlowBundle(flows []FlowEntry) error

	// DeleteFlowBundle atomically removes multiple flow entries.
	DeleteFlowBundle(flows []FlowEntry) error

	// AddPort adds a port to the bridge.
	AddPort(bridge string, port string) error

	// DeletePort removes a port from the bridge.
	DeletePort(bridge string, port string) error

	// AddTunnelPort creates a Geneve tunnel port on the bridge.
	AddTunnelPort(bridge string, port string, remoteIP string, key int) error

	// GetOfPort returns the OpenFlow port number for the named port.
	GetOfPort(port string) (int, error)

	// FindPortByExternalID returns the OVS port name whose external_ids:iface-id
	// matches the given portID. Returns ("", nil) if not found.
	FindPortByExternalID(portID string) (string, error)

	// GetFlows returns all flow entries for the specified table.
	GetFlows(table int) ([]FlowEntry, error)

	// SetInterfaceExternalIDs sets external_ids on an interface.
	SetInterfaceExternalIDs(port string, externalIDs map[string]string) error
}
