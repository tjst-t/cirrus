package agent

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strings"
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

// internalLBGroupIDBase is added to internal LB group IDs to avoid collision with
// external L4LB group IDs (which use FNV-1a of the ingress UUID directly).
// FNV-1a 32-bit hashes have a maximum of ~4 billion values; by XOR-ing with
// this constant we shift the internal LB IDs into a disjoint range in practice.
// Both ranges can still theoretically collide for unlucky UUIDs, but the
// probability is negligible for the number of LBs expected in a deployment.
const internalLBGroupIDBase = uint32(0x80000000)

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

	// Installed OVS group IDs for l4_lb ingresses: ingress_id → group_id
	lbGroups map[string]uint32

	// Installed direct_ip DNAT flows: ingress_id → public_ip (for deletion)
	directIPFlows map[string]string

	// Installed l4lb steering flows: ingress_id → "match|actions" fingerprint
	lbFlows map[string]string

	// Installed OVS group IDs for internal LBs: lb_id → group_id
	internalLBGroups map[string]uint32

	// Installed internal LB steering flows: lb_id → "match|actions" fingerprint
	internalLBFlows map[string]string
}

// NewPipeline creates a new Pipeline manager.
func NewPipeline(client OVSClient, logger *slog.Logger) *Pipeline {
	return &Pipeline{
		client:           client,
		logger:           logger,
		tunnelPorts:      make(map[string]string),
		lbGroups:         make(map[string]uint32),
		directIPFlows:    make(map[string]string),
		lbFlows:          make(map[string]string),
		internalLBGroups: make(map[string]uint32),
		internalLBFlows:  make(map[string]string),
	}
}

// Apply computes the desired OVS flows from the state, diffs against
// current, and applies the changes atomically.
func (p *Pipeline) Apply(state *pb.HostNetworkState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. Ensure tunnel ports exist for all remote hosts and fallback route destinations
	if err := p.ensureTunnelPorts(state.RemotePorts, state.FallbackRoutes); err != nil {
		return fmt.Errorf("pipeline: ensure tunnels: %w", err)
	}

	// 2. Resolve OpenFlow port numbers for local ports
	localPorts, err := p.resolveLocalPorts(state.Ports)
	if err != nil {
		return fmt.Errorf("pipeline: resolve ports: %w", err)
	}

	// 3. Generate desired flows
	ctx := &FlowContext{
		LocalPorts:     localPorts,
		RemotePorts:    state.RemotePorts,
		Policies:       state.Policies,
		TunnelOfPort:   p.tunnelOfPort,
		GatewayMAC:     DefaultGatewayMAC,
		FallbackRoutes: state.FallbackRoutes,
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

	// 6. Apply ingress (DNAT) rules for GW-role hosts.
	// Always called when GatewayInfo is present so stale flows are cleaned up
	// even when all ingress rules have been removed.
	if state.GatewayInfo != nil {
		if err := p.applyIngressRules(state.IngressRules, state.GatewayInfo); err != nil {
			return fmt.Errorf("pipeline: apply ingress rules: %w", err)
		}
	}

	// 7. Apply internal LB rules on ALL hosts (no GW check).
	if err := p.applyInternalLBRules(state.InternalLbRules); err != nil {
		return fmt.Errorf("pipeline: apply internal lb rules: %w", err)
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
		"internal_lb_rules", len(state.InternalLbRules),
		"fallback_routes", len(state.FallbackRoutes),
	)

	return nil
}

// applyIngressRules installs DNAT OVS flows for Direct IP and L4 LB ingress rules on a GW-role host.
func (p *Pipeline) applyIngressRules(ingressRules []*pb.IngressRule, gatewayInfo *pb.GatewayInfo) error {
	// Track which ingress IDs we've seen this cycle to detect removed ones.
	seenIngresses := make(map[string]bool)

	for _, rule := range ingressRules {
		seenIngresses[rule.IngressId] = true

		switch rule.Type {
		case network.IngressTypeDirectIP:
			match := fmt.Sprintf("ip,nw_dst=%s", rule.PublicIp)
			actions := fmt.Sprintf("ct(commit,nat(dst=%s)),resubmit(,%d)", rule.TargetIp, TableDstHostResolution)
			fingerprint := match + "|" + actions
			if p.directIPFlows[rule.IngressId] != fingerprint {
				if err := p.client.AddFlow(TableInputClassification, 300, match, actions); err != nil {
					return fmt.Errorf("add dnat flow for ingress %s: %w", rule.IngressId, err)
				}
				p.directIPFlows[rule.IngressId] = fingerprint
				p.logger.Info("installed DNAT flow",
					"ingress_id", rule.IngressId,
					"public_ip", rule.PublicIp,
					"target_ip", rule.TargetIp,
					"gateway_external_ip", gatewayInfo.ExternalIp,
				)
			}

		case network.IngressTypeL4LB:
			if err := p.applyL4LBRule(rule, gatewayInfo); err != nil {
				return fmt.Errorf("apply l4lb rule for ingress %s: %w", rule.IngressId, err)
			}
		}
	}

	// Clean up OVS groups and steering flows for l4_lb ingresses that are no longer present.
	for ingressID, groupID := range p.lbGroups {
		if !seenIngresses[ingressID] {
			if err := p.client.DeleteGroup(groupID); err != nil {
				p.logger.Warn("failed to delete stale lb group",
					"ingress_id", ingressID, "group_id", groupID, "error", err)
			}
			// Also remove the steering flow fingerprint so it gets re-added if the ingress returns.
			delete(p.lbFlows, ingressID)
			delete(p.lbGroups, ingressID)
		}
	}

	// Clean up DNAT flows for direct_ip ingresses that are no longer present.
	// directIPFlows stores "match|actions" fingerprint; the match part is before "|".
	for ingressID, fingerprint := range p.directIPFlows {
		if !seenIngresses[ingressID] {
			match := fingerprint
			if i := strings.IndexByte(fingerprint, '|'); i >= 0 {
				match = fingerprint[:i]
			}
			if err := p.client.DeleteFlow(TableInputClassification, match); err != nil {
				p.logger.Warn("failed to delete stale dnat flow",
					"ingress_id", ingressID, "error", err)
			}
			delete(p.directIPFlows, ingressID)
		}
	}

	return nil
}

// applyL4LBRule installs an OpenFlow select group and matching flow for an L4 LB ingress.
func (p *Pipeline) applyL4LBRule(rule *pb.IngressRule, gatewayInfo *pb.GatewayInfo) error {
	if len(rule.Backends) == 0 {
		p.logger.Warn("l4lb rule has no healthy backends, skipping", "ingress_id", rule.IngressId)
		return nil
	}

	groupID := l4lbGroupID(rule.IngressId)

	// Build the group spec string.
	groupSpec := p.buildL4LBGroupSpec(groupID, rule)

	// Add or modify the group depending on whether it exists.
	if _, exists := p.lbGroups[rule.IngressId]; exists {
		if err := p.client.ModifyGroup(groupSpec); err != nil {
			return fmt.Errorf("mod-group: %w", err)
		}
	} else {
		if err := p.client.AddGroup(groupSpec); err != nil {
			return fmt.Errorf("add-group: %w", err)
		}
		p.lbGroups[rule.IngressId] = groupID
	}

	// Install the matching flow: ip,tcp,nw_dst=<public_ip>,tp_dst=<listener_port> → group:<id>
	match := fmt.Sprintf("ip,tcp,nw_dst=%s,tp_dst=%d", rule.PublicIp, rule.ListenerPort)
	actions := fmt.Sprintf("group:%d", groupID)
	fingerprint := match + "|" + actions
	if p.lbFlows[rule.IngressId] != fingerprint {
		if err := p.client.AddFlow(TableInputClassification, 310, match, actions); err != nil {
			return fmt.Errorf("add l4lb flow: %w", err)
		}
		p.lbFlows[rule.IngressId] = fingerprint
	}

	p.logger.Info("installed L4 LB flow",
		"ingress_id", rule.IngressId,
		"public_ip", rule.PublicIp,
		"listener_port", rule.ListenerPort,
		"backends", len(rule.Backends),
		"session_affinity", rule.SessionAffinity,
		"group_id", groupID,
		"gateway_external_ip", gatewayInfo.ExternalIp,
	)

	return nil
}

// buildGroupSpec constructs the ovs-ofctl group spec string for an L4LB rule.
// For session_affinity=source_ip: uses selection_method=ip_src (hash on src IP only).
// For session_affinity=none (default): uses default 5-tuple hash.
// OVS requires selection_method to appear before the bucket list.
func buildGroupSpec(groupID uint32, sessionAffinity string, backends []*pb.L4LBBackend) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "group_id=%d,type=select", groupID)

	if sessionAffinity == "source_ip" {
		fmt.Fprintf(&sb, ",selection_method=ip_src")
	}

	for _, b := range backends {
		weight := b.Weight
		if weight <= 0 {
			weight = 1
		}
		fmt.Fprintf(&sb, ",bucket=weight:%d,actions=ct(commit,nat(dst=%s:%d)),resubmit(,%d)",
			weight, b.Ip, b.Port, TableDstHostResolution)
	}

	return sb.String()
}

func (p *Pipeline) buildL4LBGroupSpec(groupID uint32, rule *pb.IngressRule) string {
	return buildGroupSpec(groupID, rule.SessionAffinity, rule.Backends)
}

// l4lbGroupID derives a stable uint32 OpenFlow group ID from an ingress UUID string.
// Uses FNV-1a to produce a deterministic non-zero group ID.
func l4lbGroupID(ingressID string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(ingressID))
	// Avoid group_id=0 (reserved in OpenFlow).
	id := h.Sum32()
	if id == 0 {
		id = 1
	}
	return id
}

// applyInternalLBRules installs OVS select groups and steering flows for internal
// LBs. Unlike external L4LB, these flows are installed on EVERY host in the network.
func (p *Pipeline) applyInternalLBRules(rules []*pb.InternalLBRule) error {
	seenLBs := make(map[string]bool)

	for _, rule := range rules {
		if len(rule.Backends) == 0 {
			p.logger.Warn("internal lb rule has no healthy backends, skipping", "lb_id", rule.LbId)
			continue
		}
		seenLBs[rule.LbId] = true

		groupID := internalLBGroupID(rule.LbId)

		// Build the group spec (same structure as external L4LB).
		groupSpec := p.buildInternalLBGroupSpec(groupID, rule)

		if _, exists := p.internalLBGroups[rule.LbId]; exists {
			if err := p.client.ModifyGroup(groupSpec); err != nil {
				return fmt.Errorf("internal lb mod-group: %w", err)
			}
		} else {
			if err := p.client.AddGroup(groupSpec); err != nil {
				return fmt.Errorf("internal lb add-group: %w", err)
			}
			p.internalLBGroups[rule.LbId] = groupID
		}

		// Steering flow: ip,tcp,nw_dst=<vip>,tp_dst=<port> → group:<id>
		match := fmt.Sprintf("ip,tcp,nw_dst=%s,tp_dst=%d", rule.Vip, rule.ListenerPort)
		actions := fmt.Sprintf("group:%d", groupID)
		fingerprint := match + "|" + actions
		if p.internalLBFlows[rule.LbId] != fingerprint {
			if err := p.client.AddFlow(TableInputClassification, 310, match, actions); err != nil {
				return fmt.Errorf("internal lb add flow: %w", err)
			}
			p.internalLBFlows[rule.LbId] = fingerprint
		}

		p.logger.Info("installed internal LB flow",
			"lb_id", rule.LbId,
			"vip", rule.Vip,
			"listener_port", rule.ListenerPort,
			"backends", len(rule.Backends),
			"session_affinity", rule.SessionAffinity,
			"group_id", groupID,
		)
	}

	// Clean up stale groups and flows for deleted internal LBs.
	for lbID, groupID := range p.internalLBGroups {
		if !seenLBs[lbID] {
			if err := p.client.DeleteGroup(groupID); err != nil {
				p.logger.Warn("failed to delete stale internal lb group",
					"lb_id", lbID, "group_id", groupID, "error", err)
			}
			if fingerprint, ok := p.internalLBFlows[lbID]; ok {
				match := fingerprint
				if i := strings.IndexByte(fingerprint, '|'); i >= 0 {
					match = fingerprint[:i]
				}
				if err := p.client.DeleteFlow(TableInputClassification, match); err != nil {
					p.logger.Warn("failed to delete stale internal lb flow",
						"lb_id", lbID, "error", err)
				}
				delete(p.internalLBFlows, lbID)
			}
			delete(p.internalLBGroups, lbID)
		}
	}

	return nil
}

func (p *Pipeline) buildInternalLBGroupSpec(groupID uint32, rule *pb.InternalLBRule) string {
	return buildGroupSpec(groupID, rule.SessionAffinity, rule.Backends)
}

// internalLBGroupID derives a stable uint32 OpenFlow group ID from a load balancer UUID.
// XOR with internalLBGroupIDBase to separate from external L4LB group IDs.
func internalLBGroupID(lbID string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(lbID))
	id := h.Sum32() ^ internalLBGroupIDBase
	if id == 0 {
		id = internalLBGroupIDBase
	}
	return id
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
		// Port name in OVS follows the convention: first 14 chars of port ID.
		// Fall back to looking up by external_ids:iface-id for sim environments
		// where ports are named by VM UUID rather than port UUID.
		portName := ovsPortName(port.PortId)
		ofPort, err := p.client.GetOfPort(portName)
		if err != nil {
			// Try finding the port by external_ids:iface-id
			if altName, ferr := p.client.FindPortByExternalID(port.PortId); ferr == nil && altName != "" {
				if altPort, aerr := p.client.GetOfPort(altName); aerr == nil {
					ofPort = altPort
					portName = altName
					err = nil
				}
			}
			if err != nil {
				p.logger.Warn("port not found in OVS, skipping",
					"port_id", port.PortId,
					"ovs_port", portName,
					"error", err,
				)
				continue
			}
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
// It considers both remote VM ports and fallback route destinations so that
// tunnel ports for migration targets are cleaned up when no longer required.
func (p *Pipeline) ensureTunnelPorts(remotePorts []*pb.RemotePort, fallbackRoutes []*pb.FallbackRoute) error {
	// Collect unique remote host IPs from both remote VM ports and fallback routes.
	neededHosts := make(map[string]bool)
	for _, rp := range remotePorts {
		neededHosts[rp.HostIp] = true
	}
	for _, fr := range fallbackRoutes {
		if fr.DestHostIp != "" {
			neededHosts[fr.DestHostIp] = true
		}
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

