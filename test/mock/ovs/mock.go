// Package ovs provides a mock OVS client for Layer 2 testing.
// It records all commands without requiring a real OVS instance,
// enabling flow conversion logic tests in pure Go unit tests.
package ovs

import (
	"fmt"
	"sync"
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
type Client interface {
	AddFlow(table int, priority int, match string, actions string) error
	DeleteFlow(table int, match string) error
	AddPort(bridge string, port string) error
	DeletePort(bridge string, port string) error
	SetInterfaceExternalIDs(port string, externalIDs map[string]string) error
	GetFlows(table int) ([]Flow, error)
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

// MockClient is an in-memory mock implementation of Client.
type MockClient struct {
	mu       sync.RWMutex
	bridge   string
	flows    map[int][]Flow           // table -> flows
	ports    map[string]map[string]bool // bridge -> set of port names
	extIDs   map[string]map[string]string // port -> external_ids
	commands []OVSCommand
	errors   map[string]error // op -> forced error (for fault injection)
}

// New creates a new MockClient with the given default bridge name.
func New(bridge string) *MockClient {
	return &MockClient{
		bridge: bridge,
		flows:  make(map[int][]Flow),
		ports:  make(map[string]map[string]bool),
		extIDs: make(map[string]map[string]string),
		errors: make(map[string]error),
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
	return nil
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

// GetFlows returns all flow entries for the specified table.
func (m *MockClient) GetFlows(table int) ([]Flow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.checkError("get-flows"); err != nil {
		return nil, err
	}

	flows := make([]Flow, len(m.flows[table]))
	copy(flows, m.flows[table])
	return flows, nil
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

// Reset clears all state and recorded commands.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.flows = make(map[int][]Flow)
	m.ports = make(map[string]map[string]bool)
	m.extIDs = make(map[string]map[string]string)
	m.commands = nil
	m.errors = make(map[string]error)
}
