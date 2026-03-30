package agent

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/google/uuid"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// OpenFlow table numbers
const (
	TableInputClassification = 0
	TableConntrack           = 1
	TableDstGroupResolution  = 2
	TablePolicyEvaluation    = 3
	TableDstHostResolution   = 4
	TableGeneveEncap         = 5
	TableLocalOutput         = 6
	TableEgress              = 7
)

// PortInfo holds per-port metadata needed for flow generation.
type PortInfo struct {
	PortID    string
	OfPort    int // OpenFlow port number
	MAC       string
	IP        string
	GatewayIP string
	VNI       int32
	GroupHash uint32
	NetworkID string
}

// FlowContext provides the runtime context needed to generate flows.
type FlowContext struct {
	// Local ports with their OpenFlow port numbers
	LocalPorts []PortInfo

	// Remote ports (VMs on other hosts)
	RemotePorts []*pb.RemotePort

	// Policy rules
	Policies []*pb.PolicyRule

	// Geneve tunnel port number (for remote traffic)
	TunnelOfPort int

	// Gateway MAC used for ARP replies (constant per bridge)
	GatewayMAC string
}

// GenerateFlows produces all OpenFlow flow entries for the given context.
func GenerateFlows(ctx *FlowContext) []FlowEntry {
	var flows []FlowEntry

	flows = append(flows, generateTableMissFlows()...)
	flows = append(flows, generateConntrackFlows(ctx)...)

	for _, p := range ctx.LocalPorts {
		flows = append(flows, generateInputClassificationFlows(p, ctx)...)
		flows = append(flows, generateDstGroupFlows(p)...)
		flows = append(flows, generateDstHostLocalFlows(p)...)
	}

	for _, rp := range ctx.RemotePorts {
		flows = append(flows, generateDstGroupRemoteFlows(rp)...)
		flows = append(flows, generateDstHostRemoteFlows(rp, ctx.TunnelOfPort)...)
	}

	flows = append(flows, generatePolicyFlows(ctx.Policies)...)
	flows = append(flows, generateTunnelInputFlows(ctx.TunnelOfPort)...)
	flows = append(flows, generateEncapOutputFlows()...)
	flows = append(flows, generateLocalOutputFlows()...)

	return flows
}

// --- Table-miss flows (default drop on all tables) ---

func generateTableMissFlows() []FlowEntry {
	var flows []FlowEntry
	for _, table := range []int{
		TableInputClassification, TableConntrack,
		TableDstGroupResolution, TablePolicyEvaluation,
		TableDstHostResolution, TableGeneveEncap,
		TableLocalOutput, TableEgress,
	} {
		flows = append(flows, FlowEntry{
			Table:    table,
			Priority: 0,
			Match:    "",
			Actions:  "drop",
		})
	}
	return flows
}

// --- Table 0: Input Classification + Port Security ---

func generateInputClassificationFlows(p PortInfo, ctx *FlowContext) []FlowEntry {
	srcGroupHex := fmt.Sprintf("0x%x", p.GroupHash)
	gwMAC := ctx.GatewayMAC
	gwMACHex := macToHex(p.GatewayIP) // not MAC, we need IP as hex for ARP load
	gwIPHex := ipToHex(p.GatewayIP)

	var flows []FlowEntry

	// Port security: allow IP traffic with correct MAC+IP, tag source group
	flows = append(flows, FlowEntry{
		Table:    TableInputClassification,
		Priority: 100,
		Match:    fmt.Sprintf("in_port=%d,ip,dl_src=%s,nw_src=%s", p.OfPort, p.MAC, p.IP),
		Actions:  fmt.Sprintf("load:%s->NXM_NX_REG1[],resubmit(,%d)", srcGroupHex, TableConntrack),
	})

	// DHCP: broadcast DHCP requests → LOCAL (agent DHCP server)
	flows = append(flows, FlowEntry{
		Table:    TableInputClassification,
		Priority: 200,
		Match:    fmt.Sprintf("in_port=%d,udp,tp_src=68,tp_dst=67", p.OfPort),
		Actions:  "LOCAL",
	})

	// DNS: queries to gateway → LOCAL (agent DNS server)
	flows = append(flows, FlowEntry{
		Table:    TableInputClassification,
		Priority: 200,
		Match:    fmt.Sprintf("in_port=%d,udp,nw_dst=%s,tp_dst=53", p.OfPort, p.GatewayIP),
		Actions:  "LOCAL",
	})

	// Metadata: 169.254.169.254 → LOCAL (agent metadata server)
	flows = append(flows, FlowEntry{
		Table:    TableInputClassification,
		Priority: 200,
		Match:    fmt.Sprintf("in_port=%d,ip,nw_dst=169.254.169.254", p.OfPort),
		Actions:  "LOCAL",
	})

	// ARP: request for gateway IP → generate inline ARP reply
	flows = append(flows, FlowEntry{
		Table:    TableInputClassification,
		Priority: 200,
		Match:    fmt.Sprintf("in_port=%d,arp,arp_tpa=%s,arp_op=1", p.OfPort, p.GatewayIP),
		Actions: fmt.Sprintf(
			"move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],"+
				"load:%s->NXM_OF_ETH_SRC[],"+
				"load:0x2->NXM_OF_ARP_OP[],"+
				"move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[],"+
				"move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[],"+
				"load:%s->NXM_NX_ARP_SHA[],"+
				"load:%s->NXM_OF_ARP_SPA[],"+
				"IN_PORT",
			macToOFHex(gwMAC), macToOFHex(gwMAC), gwIPHex,
		),
	})

	// ARP: allow ARP with correct source MAC (pass through)
	flows = append(flows, FlowEntry{
		Table:    TableInputClassification,
		Priority: 100,
		Match:    fmt.Sprintf("in_port=%d,arp,dl_src=%s", p.OfPort, p.MAC),
		Actions:  fmt.Sprintf("load:%s->NXM_NX_REG1[],resubmit(,%d)", srcGroupHex, TableConntrack),
	})

	_ = gwMACHex // unused helper reference

	return flows
}

// --- Table 1: Conntrack ---

func generateConntrackFlows(ctx *FlowContext) []FlowEntry {
	return []FlowEntry{
		// Established → skip to local output
		{Table: TableConntrack, Priority: 100, Match: "ct_state=+est+trk", Actions: fmt.Sprintf("resubmit(,%d)", TableLocalOutput)},
		// Related → skip to local output
		{Table: TableConntrack, Priority: 100, Match: "ct_state=+rel+trk", Actions: fmt.Sprintf("resubmit(,%d)", TableLocalOutput)},
		// Invalid → drop
		{Table: TableConntrack, Priority: 100, Match: "ct_state=+inv+trk", Actions: "drop"},
		// Untracked → send to conntrack zone, then re-enter this table
		{Table: TableConntrack, Priority: 50, Match: "ct_state=-trk", Actions: fmt.Sprintf("ct(table=%d)", TableConntrack)},
		// New connection → policy pipeline
		{Table: TableConntrack, Priority: 10, Match: "ct_state=+new+trk", Actions: fmt.Sprintf("resubmit(,%d)", TableDstGroupResolution)},
	}
}

// --- Table 2: Destination Group Resolution ---

func generateDstGroupFlows(p PortInfo) []FlowEntry {
	return []FlowEntry{
		{
			Table:    TableDstGroupResolution,
			Priority: 100,
			Match:    fmt.Sprintf("ip,nw_dst=%s", p.IP),
			Actions:  fmt.Sprintf("load:0x%x->NXM_NX_REG0[],resubmit(,%d)", p.GroupHash, TablePolicyEvaluation),
		},
	}
}

func generateDstGroupRemoteFlows(rp *pb.RemotePort) []FlowEntry {
	groupHash := groupIDHash(rp.GroupId)
	return []FlowEntry{
		{
			Table:    TableDstGroupResolution,
			Priority: 100,
			Match:    fmt.Sprintf("ip,nw_dst=%s", rp.IpAddress),
			Actions:  fmt.Sprintf("load:0x%x->NXM_NX_REG0[],resubmit(,%d)", groupHash, TablePolicyEvaluation),
		},
	}
}

// --- Table 3: Policy Evaluation ---

func generatePolicyFlows(policies []*pb.PolicyRule) []FlowEntry {
	var flows []FlowEntry

	for _, pol := range policies {
		srcHash := groupIDHash(pol.SrcGroupId)
		dstHash := groupIDHash(pol.DstGroupId)

		match := fmt.Sprintf("ip,reg0=0x%x,reg1=0x%x", dstHash, srcHash)

		// Add protocol match
		switch pol.Protocol {
		case "tcp":
			match += ",tcp"
			if pol.DstPort > 0 {
				match += fmt.Sprintf(",tp_dst=%d", pol.DstPort)
			}
		case "udp":
			match += ",udp"
			if pol.DstPort > 0 {
				match += fmt.Sprintf(",tp_dst=%d", pol.DstPort)
			}
		case "icmp":
			match += ",icmp"
		case "any":
			// no additional protocol match
		}

		var actions string
		if pol.Action == "allow" {
			actions = fmt.Sprintf("ct(commit),resubmit(,%d)", TableDstHostResolution)
		} else {
			actions = "drop"
		}

		flows = append(flows, FlowEntry{
			Table:    TablePolicyEvaluation,
			Priority: int(pol.Priority),
			Match:    match,
			Actions:  actions,
		})
	}

	return flows
}

// --- Table 4: Destination Host Resolution ---

func generateDstHostLocalFlows(p PortInfo) []FlowEntry {
	return []FlowEntry{
		{
			Table:    TableDstHostResolution,
			Priority: 100,
			Match:    fmt.Sprintf("ip,nw_dst=%s", p.IP),
			Actions:  fmt.Sprintf("load:%d->NXM_NX_REG2[],resubmit(,%d)", p.OfPort, TableLocalOutput),
		},
	}
}

func generateDstHostRemoteFlows(rp *pb.RemotePort, tunnelOfPort int) []FlowEntry {
	return []FlowEntry{
		{
			Table:    TableDstHostResolution,
			Priority: 100,
			Match:    fmt.Sprintf("ip,nw_dst=%s", rp.IpAddress),
			Actions: fmt.Sprintf(
				"load:%d->NXM_NX_REG2[],set_field:%s->tun_dst,set_field:%d->tun_id,resubmit(,%d)",
				tunnelOfPort, rp.HostIp, rp.Vni, TableGeneveEncap,
			),
		},
	}
}

// --- Table 5: Geneve Encapsulation ---

func generateEncapOutputFlows() []FlowEntry {
	return []FlowEntry{
		{Table: TableGeneveEncap, Priority: 100, Match: "", Actions: "output:NXM_NX_REG2[]"},
	}
}

// generateLocalOutputFlows creates the output flow for Table 6.
func generateLocalOutputFlows() []FlowEntry {
	return []FlowEntry{
		{Table: TableLocalOutput, Priority: 100, Match: "", Actions: "output:NXM_NX_REG2[]"},
	}
}

// --- Tunnel input (Table 0) ---

func generateTunnelInputFlows(tunnelOfPort int) []FlowEntry {
	if tunnelOfPort == 0 {
		return nil
	}
	// Traffic from tunnel: already policy-evaluated at source host → deliver locally
	return []FlowEntry{
		{
			Table:    TableInputClassification,
			Priority: 50,
			Match:    fmt.Sprintf("in_port=%d", tunnelOfPort),
			Actions:  fmt.Sprintf("resubmit(,%d)", TableDstHostResolution),
		},
	}
}

// --- Utility functions ---

// groupIDHash produces a 32-bit hash from a UUID string for NXM register use.
func groupIDHash(uuidStr string) uint32 {
	id, err := uuid.Parse(uuidStr)
	if err != nil {
		return 0
	}
	// Use first 4 bytes of UUID as hash
	return binary.BigEndian.Uint32(id[:4])
}

// macToOFHex converts "aa:bb:cc:dd:ee:ff" to "0xaabbccddeeff" for OpenFlow load actions.
func macToOFHex(mac string) string {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return "0x000000000000"
	}
	return fmt.Sprintf("0x%02x%02x%02x%02x%02x%02x", hw[0], hw[1], hw[2], hw[3], hw[4], hw[5])
}

// ipToHex converts "100.64.0.2" to "0x64400002" for OpenFlow load actions.
func ipToHex(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "0x00000000"
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "0x00000000"
	}
	return fmt.Sprintf("0x%02x%02x%02x%02x", ip4[0], ip4[1], ip4[2], ip4[3])
}

// macToHex is an alias kept for clarity (unused, but available).
func macToHex(ipStr string) string {
	return ipToHex(ipStr)
}
