package agent

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/tjst-t/cirrus/internal/network"
	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

const (
	// DefaultGatewayMAC is the fixed MAC used by OVS for gateway ARP replies.
	DefaultGatewayMAC = "02:00:00:00:00:01"

	// BridgeName is the default OVS bridge.
	BridgeName = "br-int"

	// TunnelPortName is the Geneve tunnel port name.
	TunnelPortName = "geneve0"
)

// Pipeline manages OVS flows for the network data plane.
type Pipeline struct {
	mu     sync.Mutex
	client OVSClient
	logger *slog.Logger

	// Current flows installed in OVS (our view)
	currentFlows []FlowEntry

	// Tunnel ports: remote host IP → port name
	tunnelPorts map[string]string

	// Tunnel port ofport
	tunnelOfPort int
}

// NewPipeline creates a new Pipeline manager.
func NewPipeline(client OVSClient, logger *slog.Logger) *Pipeline {
	return &Pipeline{
		client:      client,
		logger:      logger,
		tunnelPorts: make(map[string]string),
	}
}

// Apply computes the desired OVS flows from the state, diffs against
// current, and applies the changes atomically.
func (p *Pipeline) Apply(state *pb.HostNetworkState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. Ensure tunnel ports exist for all remote hosts
	if err := p.ensureTunnelPorts(state.RemotePorts); err != nil {
		return fmt.Errorf("pipeline: ensure tunnels: %w", err)
	}

	// 2. Resolve OpenFlow port numbers for local ports
	localPorts, err := p.resolveLocalPorts(state.Ports)
	if err != nil {
		return fmt.Errorf("pipeline: resolve ports: %w", err)
	}

	// 3. Generate desired flows
	ctx := &FlowContext{
		LocalPorts:   localPorts,
		RemotePorts:  state.RemotePorts,
		Policies:     state.Policies,
		TunnelOfPort: p.tunnelOfPort,
		GatewayMAC:   DefaultGatewayMAC,
	}
	desired := GenerateFlows(ctx)

	// 4. Diff and apply
	add, del := DiffFlows(p.currentFlows, desired)

	if len(del) > 0 {
		if err := p.client.DeleteFlowBundle(del); err != nil {
			return fmt.Errorf("pipeline: delete flows: %w", err)
		}
	}

	if len(add) > 0 {
		if err := p.client.AddFlowBundle(add); err != nil {
			return fmt.Errorf("pipeline: add flows: %w", err)
		}
	}

	p.currentFlows = desired

	// 5. Apply egress (SNAT) rules for GW-role hosts
	if state.GatewayInfo != nil && len(state.EgressRules) > 0 {
		if err := p.applyEgressRules(state.EgressRules, state.GatewayInfo); err != nil {
			return fmt.Errorf("pipeline: apply egress rules: %w", err)
		}
	}

	// 6. Apply ingress (DNAT) rules for GW-role hosts
	if state.GatewayInfo != nil && len(state.IngressRules) > 0 {
		if err := p.applyIngressRules(state.IngressRules, state.GatewayInfo); err != nil {
			return fmt.Errorf("pipeline: apply ingress rules: %w", err)
		}
	}

	p.logger.Info("pipeline applied",
		"added", len(add),
		"deleted", len(del),
		"total", len(desired),
		"local_ports", len(localPorts),
		"remote_ports", len(state.RemotePorts),
		"policies", len(state.Policies),
		"egress_rules", len(state.EgressRules),
		"ingress_rules", len(state.IngressRules),
	)

	return nil
}

// applyIngressRules installs DNAT OVS flows for Direct IP ingress rules on a GW-role host.
func (p *Pipeline) applyIngressRules(ingressRules []*pb.IngressRule, gatewayInfo *pb.GatewayInfo) error {
	for _, rule := range ingressRules {
		if rule.Type != network.IngressTypeDirectIP {
			continue
		}
		flow := FlowEntry{
			Table:    TableInputClassification,
			Priority: 300,
			Match:    fmt.Sprintf("ip,nw_dst=%s", rule.PublicIp),
			Actions:  fmt.Sprintf("ct(commit,nat(dst=%s)),resubmit(,%d)", rule.TargetIp, TableDstHostResolution),
		}
		if err := p.client.AddFlow(flow.Table, flow.Priority, flow.Match, flow.Actions); err != nil {
			return fmt.Errorf("add dnat flow for ingress %s: %w", rule.IngressId, err)
		}
		p.logger.Info("installed DNAT flow",
			"ingress_id", rule.IngressId,
			"public_ip", rule.PublicIp,
			"target_ip", rule.TargetIp,
			"gateway_external_ip", gatewayInfo.ExternalIp,
		)
	}
	return nil
}

// applyEgressRules installs SNAT flows for each EgressRule on a GW-role host.
func (p *Pipeline) applyEgressRules(egressRules []*pb.EgressRule, gatewayInfo *pb.GatewayInfo) error {
	for _, rule := range egressRules {
		if rule.Type != network.EgressTypeNATGateway {
			continue
		}
		flow := FlowEntry{
			Table:    TableEgress,
			Priority: 100,
			Match:    fmt.Sprintf("ip,nw_src=%s", rule.NetworkCidr),
			Actions:  fmt.Sprintf("ct(commit,nat(src=%s)),output:NORMAL", rule.PublicIp),
		}
		if err := p.client.AddFlow(flow.Table, flow.Priority, flow.Match, flow.Actions); err != nil {
			return fmt.Errorf("add snat flow for egress %s: %w", rule.EgressId, err)
		}
		p.logger.Info("installed SNAT flow",
			"egress_id", rule.EgressId,
			"network_cidr", rule.NetworkCidr,
			"public_ip", rule.PublicIp,
			"gateway_external_ip", gatewayInfo.ExternalIp,
		)
	}
	return nil
}

// resolveLocalPorts maps PortState to PortInfo with OpenFlow port numbers.
func (p *Pipeline) resolveLocalPorts(ports []*pb.PortState) ([]PortInfo, error) {
	var result []PortInfo
	for _, port := range ports {
		// Port name in OVS follows the convention: first 14 chars of port ID
		portName := ovsPortName(port.PortId)
		ofPort, err := p.client.GetOfPort(portName)
		if err != nil {
			p.logger.Warn("port not found in OVS, skipping",
				"port_id", port.PortId,
				"ovs_port", portName,
				"error", err,
			)
			continue
		}

		result = append(result, PortInfo{
			PortID:    port.PortId,
			OfPort:    ofPort,
			MAC:       port.MacAddress,
			IP:        port.IpAddress,
			GatewayIP: port.GatewayIp,
			VNI:       port.Vni,
			GroupHash: groupIDHash(port.GroupId),
			NetworkID: port.NetworkId,
		})
	}
	return result, nil
}

// ensureTunnelPorts creates/removes Geneve tunnel ports as needed.
func (p *Pipeline) ensureTunnelPorts(remotePorts []*pb.RemotePort) error {
	// Collect unique remote host IPs
	neededHosts := make(map[string]bool)
	for _, rp := range remotePorts {
		neededHosts[rp.HostIp] = true
	}

	// Remove tunnels to hosts no longer needed
	for hostIP, portName := range p.tunnelPorts {
		if !neededHosts[hostIP] {
			if err := p.client.DeletePort(BridgeName, portName); err != nil {
				p.logger.Warn("failed to delete tunnel port", "port", portName, "error", err)
			}
			delete(p.tunnelPorts, hostIP)
		}
	}

	// Create tunnels to new hosts
	for hostIP := range neededHosts {
		if _, exists := p.tunnelPorts[hostIP]; exists {
			continue
		}
		portName := tunnelPortName(hostIP)
		if err := p.client.AddTunnelPort(BridgeName, portName, hostIP, 0); err != nil {
			return fmt.Errorf("add tunnel to %s: %w", hostIP, err)
		}
		p.tunnelPorts[hostIP] = portName
	}

	// If we have any tunnels, get the ofport for the first one.
	// In practice all Geneve traffic goes through a single tunnel port
	// with flow-based key (VNI set per-flow), but for simplicity we use
	// one port per remote host. Pick any tunnel port for the ofport.
	p.tunnelOfPort = 0
	if len(p.tunnelPorts) > 0 {
		// Use a deterministic choice (sorted)
		var names []string
		for _, name := range p.tunnelPorts {
			names = append(names, name)
		}
		sort.Strings(names)
		ofPort, err := p.client.GetOfPort(names[0])
		if err == nil {
			p.tunnelOfPort = ofPort
		}
	}

	return nil
}

// ovsPortName derives the OVS port name from a port UUID.
// OVS port names are limited to 15 characters.
func ovsPortName(portID string) string {
	if len(portID) > 14 {
		return portID[:14]
	}
	return portID
}

// tunnelPortName generates a unique tunnel port name from the remote host IP.
func tunnelPortName(hostIP string) string {
	// Replace dots with underscores, prefix with "gn_"
	name := "gn_"
	for _, c := range hostIP {
		if c == '.' {
			name += "_"
		} else {
			name += string(c)
		}
	}
	// Truncate to 15 chars (OVS limit)
	if len(name) > 15 {
		name = name[:15]
	}
	return name
}

