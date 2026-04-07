// Package ovs provides a mock OVS client for Layer 2 testing.
// It records all commands without requiring a real OVS instance,
// enabling flow conversion logic tests in pure Go unit tests.
package ovs

import (
	"fmt"
	"strings"
	"sync"

	netagent "github.com/tjst-t/cirrus/internal/network/agent"
)

// OVSCommand represents a recorded OVS operation.
type OVSCommand struct {
	Op      string // "add-flow", "delete-flow", "add-port", "delete-port", "set-interface"
	Bridge  string
	Table   int
	Priority int
	Match   string
	Actions string
	Port    string
	Options map[string]string
}

// Client defines the interface for OVS operations.
// MockClient implements both this interface and netagent.OVSClient.
type Client interface {
	netagent.OVSClient
	GetPorts(bridge string) ([]string, error)
	GetRecordedCommands() []OVSCommand
	Reset()
}

// Flow represents an OpenFlow flow entry.
type Flow struct {
	Table    int
	Priority int
	Match    string
	Actions  string
}

// TunnelPort holds metadata about a Geneve tunnel port.
type TunnelPort struct {
	Bridge   string
	Port     string
	RemoteIP string
	Key      int
}

// MockClient is an in-memory mock implementation of Client.
type MockClient struct {
	mu          sync.RWMutex
	bridge      string
	flows       map[int][]Flow              // table -> flows
	ports       map[string]map[string]bool   // bridge -> set of port names
	extIDs      map[string]map[string]string // port -> external_ids
	tunnelPorts map[string]TunnelPort        // port name -> tunnel info
	ofPortSeq   int                          // next OpenFlow port number
	ofPorts     map[string]int               // port name -> ofport number
	groups      map[uint32]string            // group_id -> spec
	commands    []OVSCommand
	errors      map[string]error // op -> forced error (for fault injection)
}

// New creates a new MockClient with the given default bridge name.
func New(bridge string) *MockClient {
	return &MockClient{
		bridge:      bridge,
		flows:       make(map[int][]Flow),
		ports:       make(map[string]map[string]bool),
		extIDs:      make(map[string]map[string]string),
		tunnelPorts: make(map[string]TunnelPort),
		ofPortSeq:   1,
		ofPorts:     make(map[string]int),
		groups:      make(map[uint32]string),
		errors:      make(map[string]error),
	}
}

// InjectError forces the specified operation to return an error.
func (m *MockClient) InjectError(op string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		delete(m.errors, op)
	} else {
		m.errors[op] = err
	}
}

func (m *MockClient) checkError(op string) error {
	if err, ok := m.errors[op]; ok {
		return err
	}
	return nil
}

func (m *MockClient) record(cmd OVSCommand) {
	m.commands = append(m.commands, cmd)
}

// AddFlow adds a flow entry to the specified table.
func (m *MockClient) AddFlow(table int, priority int, match string, actions string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("add-flow"); err != nil {
		return err
	}

	m.record(OVSCommand{
		Op:       "add-flow",
		Bridge:   m.bridge,
		Table:    table,
		Priority: priority,
		Match:    match,
		Actions:  actions,
	})

	m.flows[table] = append(m.flows[table], Flow{
		Table:    table,
		Priority: priority,
		Match:    match,
		Actions:  actions,
	})
	return nil
}

// DeleteFlow removes matching flow entries from the specified table.
func (m *MockClient) DeleteFlow(table int, match string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("delete-flow"); err != nil {
		return err
	}

	m.record(OVSCommand{
		Op:     "delete-flow",
		Bridge: m.bridge,
		Table:  table,
		Match:  match,
	})

	flows := m.flows[table]
	filtered := flows[:0]
	for _, f := range flows {
		if f.Match != match {
			filtered = append(filtered, f)
		}
	}
	m.flows[table] = filtered
	return nil
}

// AddPort adds a port to the specified bridge.
func (m *MockClient) AddPort(bridge string, port string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("add-port"); err != nil {
		return err
	}

	m.record(OVSCommand{
		Op:     "add-port",
		Bridge: bridge,
		Port:   port,
	})

	if m.ports[bridge] == nil {
		m.ports[bridge] = make(map[string]bool)
	}
	if m.ports[bridge][port] {
		return fmt.Errorf("port %s already exists on bridge %s", port, bridge)
	}
	m.ports[bridge][port] = true
	m.ofPorts[port] = m.ofPortSeq
	m.ofPortSeq++
	return nil
}

// DeletePort removes a port from the specified bridge.
func (m *MockClient) DeletePort(bridge string, port string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("delete-port"); err != nil {
		return err
	}

	m.record(OVSCommand{
		Op:     "delete-port",
		Bridge: bridge,
		Port:   port,
	})

	if m.ports[bridge] == nil || !m.ports[bridge][port] {
		return fmt.Errorf("port %s not found on bridge %s", port, bridge)
	}
	delete(m.ports[bridge], port)
	delete(m.ofPorts, port)
	delete(m.tunnelPorts, port)
	return nil
}

// AddFlowBundle atomically adds multiple flow entries.
func (m *MockClient) AddFlowBundle(flows []netagent.FlowEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("add-flow-bundle"); err != nil {
		return err
	}

	for _, f := range flows {
		m.record(OVSCommand{
			Op:       "add-flow-bundle",
			Bridge:   m.bridge,
			Table:    f.Table,
			Priority: f.Priority,
			Match:    f.Match,
			Actions:  f.Actions,
		})
		m.flows[f.Table] = append(m.flows[f.Table], Flow{
			Table:    f.Table,
			Priority: f.Priority,
			Match:    f.Match,
			Actions:  f.Actions,
		})
	}
	return nil
}

// DeleteFlowBundle atomically removes multiple flow entries.
func (m *MockClient) DeleteFlowBundle(flows []netagent.FlowEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("delete-flow-bundle"); err != nil {
		return err
	}

	for _, f := range flows {
		m.record(OVSCommand{
			Op:       "delete-flow-bundle",
			Bridge:   m.bridge,
			Table:    f.Table,
			Priority: f.Priority,
			Match:    f.Match,
		})
		tableFlows := m.flows[f.Table]
		filtered := tableFlows[:0]
		for _, existing := range tableFlows {
			if !(existing.Priority == f.Priority && existing.Match == f.Match) {
				filtered = append(filtered, existing)
			}
		}
		m.flows[f.Table] = filtered
	}
	return nil
}

// AddTunnelPort creates a Geneve tunnel port on the bridge.
func (m *MockClient) AddTunnelPort(bridge string, port string, remoteIP string, key int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("add-tunnel-port"); err != nil {
		return err
	}

	m.record(OVSCommand{
		Op:     "add-tunnel-port",
		Bridge: bridge,
		Port:   port,
		Options: map[string]string{
			"remote_ip": remoteIP,
			"key":       fmt.Sprintf("%d", key),
			"type":      "geneve",
		},
	})

	if m.ports[bridge] == nil {
		m.ports[bridge] = make(map[string]bool)
	}
	if m.ports[bridge][port] {
		return fmt.Errorf("port %s already exists on bridge %s", port, bridge)
	}
	m.ports[bridge][port] = true
	m.tunnelPorts[port] = TunnelPort{
		Bridge:   bridge,
		Port:     port,
		RemoteIP: remoteIP,
		Key:      key,
	}
	m.ofPorts[port] = m.ofPortSeq
	m.ofPortSeq++
	return nil
}

// GetOfPort returns the OpenFlow port number for the named port.
func (m *MockClient) GetOfPort(port string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.checkError("get-ofport"); err != nil {
		return 0, err
	}

	ofPort, ok := m.ofPorts[port]
	if !ok {
		return 0, fmt.Errorf("port %s not found", port)
	}
	return ofPort, nil
}

// GetTunnelPorts returns all tunnel port metadata.
func (m *MockClient) GetTunnelPorts() map[string]TunnelPort {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]TunnelPort, len(m.tunnelPorts))
	for k, v := range m.tunnelPorts {
		result[k] = v
	}
	return result
}

// SetInterfaceExternalIDs sets external_ids on an interface.
func (m *MockClient) SetInterfaceExternalIDs(port string, externalIDs map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkError("set-interface"); err != nil {
		return err
	}

	m.record(OVSCommand{
		Op:      "set-interface",
		Port:    port,
		Options: externalIDs,
	})

	if m.extIDs[port] == nil {
		m.extIDs[port] = make(map[string]string)
	}
	for k, v := range externalIDs {
		m.extIDs[port][k] = v
	}
	return nil
}

// FindPortByExternalID returns the port name whose external_ids:iface-id matches portID.
func (m *MockClient) FindPortByExternalID(portID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for portName, ids := range m.extIDs {
		if ids["iface-id"] == portID {
			return portName, nil
		}
	}
	return "", nil
}

// GetFlows returns all flow entries for the specified table.
func (m *MockClient) GetFlows(table int) ([]netagent.FlowEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.checkError("get-flows"); err != nil {
		return nil, err
	}

	result := make([]netagent.FlowEntry, len(m.flows[table]))
	for i, f := range m.flows[table] {
		result[i] = netagent.FlowEntry{Table: f.Table, Priority: f.Priority, Match: f.Match, Actions: f.Actions}
	}
	return result, nil
}

// GetPorts returns all port names on the specified bridge.
func (m *MockClient) GetPorts(bridge string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.checkError("get-ports"); err != nil {
		return nil, err
	}

	var ports []string
	for p := range m.ports[bridge] {
		ports = append(ports, p)
	}
	return ports, nil
}

// GetInterfaceExternalIDs returns external_ids for the specified port.
func (m *MockClient) GetInterfaceExternalIDs(port string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range m.extIDs[port] {
		result[k] = v
	}
	return result
}

// GetRecordedCommands returns all recorded OVS commands.
func (m *MockClient) GetRecordedCommands() []OVSCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cmds := make([]OVSCommand, len(m.commands))
	copy(cmds, m.commands)
	return cmds
}

// AddGroup records an add-group command.
func (m *MockClient) AddGroup(groupSpec string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkError("add-group"); err != nil {
		return err
	}
	m.record(OVSCommand{Op: "add-group", Bridge: m.bridge, Actions: groupSpec})
	// Parse group_id from spec for in-memory tracking.
	id := parseGroupID(groupSpec)
	m.groups[id] = groupSpec
	return nil
}

// ModifyGroup records a mod-group command.
func (m *MockClient) ModifyGroup(groupSpec string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkError("mod-group"); err != nil {
		return err
	}
	m.record(OVSCommand{Op: "mod-group", Bridge: m.bridge, Actions: groupSpec})
	id := parseGroupID(groupSpec)
	m.groups[id] = groupSpec
	return nil
}

// DeleteGroup records a del-groups command.
func (m *MockClient) DeleteGroup(groupID uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkError("del-group"); err != nil {
		return err
	}
	m.record(OVSCommand{Op: "del-group", Bridge: m.bridge, Table: int(groupID)})
	delete(m.groups, groupID)
	return nil
}

// GetGroups returns the current in-memory groups map (group_id → spec).
func (m *MockClient) GetGroups() map[uint32]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[uint32]string, len(m.groups))
	for k, v := range m.groups {
		result[k] = v
	}
	return result
}

// parseGroupID extracts the group_id integer from a group spec string like "group_id=42,type=select,...".
func parseGroupID(spec string) uint32 {
	for _, part := range strings.Split(spec, ",") {
		if strings.HasPrefix(part, "group_id=") {
			var id uint32
			fmt.Sscanf(strings.TrimPrefix(part, "group_id="), "%d", &id)
			return id
		}
	}
	return 0
}

// Reset clears all state and recorded commands.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.flows = make(map[int][]Flow)
	m.ports = make(map[string]map[string]bool)
	m.extIDs = make(map[string]map[string]string)
	m.tunnelPorts = make(map[string]TunnelPort)
	m.ofPortSeq = 1
	m.ofPorts = make(map[string]int)
	m.groups = make(map[uint32]string)
	m.commands = nil
	m.errors = make(map[string]error)
}
