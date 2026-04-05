package agent

import (
	"sync"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// StateCache holds the current desired network state for this host.
// It provides thread-safe access and reverse lookups for DHCP, DNS, and metadata.
type StateCache struct {
	mu sync.RWMutex

	// Primary storage
	ports       map[string]*pb.PortState   // port_id -> PortState
	policies    map[string]*pb.PolicyRule   // policy_id -> PolicyRule
	remotePorts map[string]*pb.RemotePort   // "network_id:ip" -> RemotePort
	dnsRecords  map[string]*pb.DnsRecord    // "network_id:name" -> DnsRecord

	// Gateway state (GW-role hosts only)
	egressRules  []*pb.EgressRule
	ingressRules []*pb.IngressRule
	gatewayInfo  *pb.GatewayInfo

	// Reverse lookups
	ipToPort  map[string]*pb.PortState // VM IP -> PortState
	macToPort map[string]*pb.PortState // MAC -> PortState

	// Network-scoped lookups
	networkDNS    map[string][]*pb.DnsRecord  // network_id -> DNS records
	networkPorts  map[string][]*pb.PortState   // network_id -> local ports

	// Version tracking
	version uint64
}

// NewStateCache creates an empty StateCache.
func NewStateCache() *StateCache {
	return &StateCache{
		ports:        make(map[string]*pb.PortState),
		policies:     make(map[string]*pb.PolicyRule),
		remotePorts:  make(map[string]*pb.RemotePort),
		dnsRecords:   make(map[string]*pb.DnsRecord),
		ipToPort:     make(map[string]*pb.PortState),
		macToPort:    make(map[string]*pb.PortState),
		networkDNS:   make(map[string][]*pb.DnsRecord),
		networkPorts: make(map[string][]*pb.PortState),
	}
}

// ApplyFull replaces all state with a full snapshot.
func (s *StateCache) ApplyFull(update *pb.HostNetworkStateUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear everything
	s.ports = make(map[string]*pb.PortState)
	s.policies = make(map[string]*pb.PolicyRule)
	s.remotePorts = make(map[string]*pb.RemotePort)
	s.dnsRecords = make(map[string]*pb.DnsRecord)
	s.ipToPort = make(map[string]*pb.PortState)
	s.macToPort = make(map[string]*pb.PortState)
	s.networkDNS = make(map[string][]*pb.DnsRecord)
	s.networkPorts = make(map[string][]*pb.PortState)

	state := update.GetState()
	if state == nil {
		s.version = update.GetVersion()
		return
	}

	for _, p := range state.Ports {
		s.addPort(p)
	}
	for _, pol := range state.Policies {
		s.policies[pol.PolicyId] = pol
	}
	for _, rp := range state.RemotePorts {
		s.addRemotePort(rp)
	}
	for _, dns := range state.DnsRecords {
		s.addDnsRecord(dns)
	}
	s.egressRules = state.EgressRules
	s.ingressRules = state.IngressRules
	s.gatewayInfo = state.GatewayInfo

	s.version = update.GetVersion()
}

// ApplyDelta applies incremental additions and removals.
func (s *StateCache) ApplyDelta(update *pb.HostNetworkStateUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Process removals first
	for _, portID := range update.RemovedPortIds {
		s.removePort(portID)
	}
	for _, polID := range update.RemovedPolicyIds {
		delete(s.policies, polID)
	}
	for _, ip := range update.RemovedRemotePortIps {
		s.removeRemotePortByIP(ip)
	}
	for _, name := range update.RemovedDnsNames {
		s.removeDnsRecordByName(name)
	}

	// Process additions
	state := update.GetState()
	if state != nil {
		for _, p := range state.Ports {
			s.addPort(p)
		}
		for _, pol := range state.Policies {
			s.policies[pol.PolicyId] = pol
		}
		for _, rp := range state.RemotePorts {
			s.addRemotePort(rp)
		}
		for _, dns := range state.DnsRecords {
			s.addDnsRecord(dns)
		}
	}

	s.version = update.GetVersion()
}

// internal helpers (caller must hold lock)

func (s *StateCache) addPort(p *pb.PortState) {
	s.ports[p.PortId] = p
	s.ipToPort[p.IpAddress] = p
	s.macToPort[p.MacAddress] = p
	s.networkPorts[p.NetworkId] = append(s.networkPorts[p.NetworkId], p)
}

func (s *StateCache) removePort(portID string) {
	p, ok := s.ports[portID]
	if !ok {
		return
	}
	delete(s.ports, portID)
	delete(s.ipToPort, p.IpAddress)
	delete(s.macToPort, p.MacAddress)
	s.networkPorts[p.NetworkId] = removeFromSlice(s.networkPorts[p.NetworkId], func(pp *pb.PortState) bool {
		return pp.PortId == portID
	})
}

func remotePortKey(networkID, ip string) string {
	return networkID + ":" + ip
}

func (s *StateCache) addRemotePort(rp *pb.RemotePort) {
	s.remotePorts[remotePortKey(rp.NetworkId, rp.IpAddress)] = rp
}

func (s *StateCache) removeRemotePortByIP(ip string) {
	for key, rp := range s.remotePorts {
		if rp.IpAddress == ip {
			delete(s.remotePorts, key)
			return
		}
	}
}

func dnsKey(networkID, name string) string {
	return networkID + ":" + name
}

func (s *StateCache) addDnsRecord(dns *pb.DnsRecord) {
	key := dnsKey(dns.NetworkId, dns.Name)
	s.dnsRecords[key] = dns
	s.rebuildNetworkDNS(dns.NetworkId)
}

func (s *StateCache) removeDnsRecordByName(name string) {
	for key, dns := range s.dnsRecords {
		if dns.Name == name {
			netID := dns.NetworkId
			delete(s.dnsRecords, key)
			s.rebuildNetworkDNS(netID)
			return
		}
	}
}

func (s *StateCache) rebuildNetworkDNS(networkID string) {
	var records []*pb.DnsRecord
	for _, dns := range s.dnsRecords {
		if dns.NetworkId == networkID {
			records = append(records, dns)
		}
	}
	if len(records) == 0 {
		delete(s.networkDNS, networkID)
	} else {
		s.networkDNS[networkID] = records
	}
}

func removeFromSlice[T any](slice []T, match func(T) bool) []T {
	result := slice[:0]
	for _, item := range slice {
		if !match(item) {
			result = append(result, item)
		}
	}
	return result
}

// Read accessors (all thread-safe)

// GetPortByIP returns the PortState for the given VM IP address.
func (s *StateCache) GetPortByIP(ip string) *pb.PortState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ipToPort[ip]
}

// GetPortByMAC returns the PortState for the given MAC address.
func (s *StateCache) GetPortByMAC(mac string) *pb.PortState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.macToPort[mac]
}

// GetDNSRecordsForNetwork returns all DNS records for the given network.
func (s *StateCache) GetDNSRecordsForNetwork(networkID string) []*pb.DnsRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := s.networkDNS[networkID]
	result := make([]*pb.DnsRecord, len(records))
	copy(result, records)
	return result
}

// GetLocalPorts returns all local ports.
func (s *StateCache) GetLocalPorts() []*pb.PortState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*pb.PortState, 0, len(s.ports))
	for _, p := range s.ports {
		result = append(result, p)
	}
	return result
}

// GetPolicies returns all policy rules.
func (s *StateCache) GetPolicies() []*pb.PolicyRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*pb.PolicyRule, 0, len(s.policies))
	for _, p := range s.policies {
		result = append(result, p)
	}
	return result
}

// GetRemotePorts returns all remote ports.
func (s *StateCache) GetRemotePorts() []*pb.RemotePort {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*pb.RemotePort, 0, len(s.remotePorts))
	for _, rp := range s.remotePorts {
		result = append(result, rp)
	}
	return result
}

// GetAllDNSRecords returns all DNS records.
func (s *StateCache) GetAllDNSRecords() []*pb.DnsRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*pb.DnsRecord, 0, len(s.dnsRecords))
	for _, dns := range s.dnsRecords {
		result = append(result, dns)
	}
	return result
}

// GetVersion returns the current state version.
func (s *StateCache) GetVersion() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// Snapshot returns a copy of the full state for flow generation.
func (s *StateCache) Snapshot() *pb.HostNetworkState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := &pb.HostNetworkState{
		EgressRules:  s.egressRules,
		IngressRules: s.ingressRules,
		GatewayInfo:  s.gatewayInfo,
	}
	for _, p := range s.ports {
		state.Ports = append(state.Ports, p)
	}
	for _, pol := range s.policies {
		state.Policies = append(state.Policies, pol)
	}
	for _, rp := range s.remotePorts {
		state.RemotePorts = append(state.RemotePorts, rp)
	}
	for _, dns := range s.dnsRecords {
		state.DnsRecords = append(state.DnsRecords, dns)
	}
	return state
}
