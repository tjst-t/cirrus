package agent

import (
	"testing"

	"github.com/tjst-t/cirrus/internal/network"
	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

func makePort(id, vmID, vmName, netID, netName, groupID, groupName, mac, ip, gw string, vni int32) *pb.PortState {
	return &pb.PortState{
		PortId:      id,
		VmId:        vmID,
		VmName:      vmName,
		NetworkId:   netID,
		NetworkName: netName,
		GroupId:     groupID,
		GroupName:   groupName,
		MacAddress:  mac,
		IpAddress:   ip,
		GatewayIp:   gw,
		Vni:         vni,
	}
}

func makePolicy(id, netID, srcGroup, dstGroup, proto string, dstPort, priority int32, action string) *pb.PolicyRule {
	return &pb.PolicyRule{
		PolicyId:   id,
		NetworkId:  netID,
		SrcGroupId: srcGroup,
		DstGroupId: dstGroup,
		Protocol:   proto,
		DstPort:    dstPort,
		Priority:   priority,
		Action:     action,
	}
}

func makeRemotePort(netID, groupID, ip, hostIP string, vni int32) *pb.RemotePort {
	return &pb.RemotePort{
		NetworkId: netID,
		GroupId:   groupID,
		IpAddress: ip,
		HostIp:    hostIP,
		Vni:       vni,
	}
}

func makeDNS(name, ip, netID string) *pb.DnsRecord {
	return &pb.DnsRecord{
		Name:      name,
		Ip:        ip,
		NetworkId: netID,
	}
}

func TestStateCache_ApplyFull(t *testing.T) {
	sc := NewStateCache()

	port1 := makePort("p1", "vm1", "web-1", "net1", "prod", "g1", "web", "aa:bb:cc:dd:ee:01", "100.64.0.1", "100.64.0.2", 100)
	pol1 := makePolicy("pol1", "net1", "g1", "g2", "tcp", 80, 1000, "allow")
	rp1 := makeRemotePort("net1", "g2", "100.64.0.5", "10.0.0.2", 100)
	dns1 := makeDNS("web-1.web.prod.internal", "100.64.0.1", "net1")

	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full:    true,
		Version: 1,
		State: &pb.HostNetworkState{
			Ports:       []*pb.PortState{port1},
			Policies:    []*pb.PolicyRule{pol1},
			RemotePorts: []*pb.RemotePort{rp1},
			DnsRecords:  []*pb.DnsRecord{dns1},
		},
	})

	// Verify version
	if sc.GetVersion() != 1 {
		t.Errorf("expected version 1, got %d", sc.GetVersion())
	}

	// Verify port lookups
	if p := sc.GetPortByIP("100.64.0.1"); p == nil || p.PortId != "p1" {
		t.Error("GetPortByIP failed")
	}
	if p := sc.GetPortByMAC("aa:bb:cc:dd:ee:01"); p == nil || p.PortId != "p1" {
		t.Error("GetPortByMAC failed")
	}

	// Verify policies
	pols := sc.GetPolicies()
	if len(pols) != 1 || pols[0].PolicyId != "pol1" {
		t.Error("GetPolicies failed")
	}

	// Verify remote ports
	rps := sc.GetRemotePorts()
	if len(rps) != 1 || rps[0].IpAddress != "100.64.0.5" {
		t.Error("GetRemotePorts failed")
	}

	// Verify DNS
	records := sc.GetDNSRecordsForNetwork("net1")
	if len(records) != 1 || records[0].Name != "web-1.web.prod.internal" {
		t.Error("GetDNSRecordsForNetwork failed")
	}

	// Apply full again (should replace)
	port2 := makePort("p2", "vm2", "api-1", "net1", "prod", "g2", "api", "aa:bb:cc:dd:ee:02", "100.64.0.5", "100.64.0.6", 100)
	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full:    true,
		Version: 2,
		State: &pb.HostNetworkState{
			Ports: []*pb.PortState{port2},
		},
	})

	if sc.GetVersion() != 2 {
		t.Errorf("expected version 2, got %d", sc.GetVersion())
	}
	if p := sc.GetPortByIP("100.64.0.1"); p != nil {
		t.Error("old port should be gone after full apply")
	}
	if p := sc.GetPortByIP("100.64.0.5"); p == nil || p.PortId != "p2" {
		t.Error("new port should exist after full apply")
	}
	if len(sc.GetPolicies()) != 0 {
		t.Error("policies should be empty after full apply with no policies")
	}
}

func TestStateCache_ApplyDelta(t *testing.T) {
	sc := NewStateCache()

	// Initial full state
	port1 := makePort("p1", "vm1", "web-1", "net1", "prod", "g1", "web", "aa:bb:cc:dd:ee:01", "100.64.0.1", "100.64.0.2", 100)
	pol1 := makePolicy("pol1", "net1", "g1", "g2", "tcp", 80, 1000, "allow")
	dns1 := makeDNS("web-1.web.prod.internal", "100.64.0.1", "net1")

	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full:    true,
		Version: 1,
		State: &pb.HostNetworkState{
			Ports:      []*pb.PortState{port1},
			Policies:   []*pb.PolicyRule{pol1},
			DnsRecords: []*pb.DnsRecord{dns1},
		},
	})

	// Delta: add port2, remove pol1, add dns2
	port2 := makePort("p2", "vm2", "api-1", "net1", "prod", "g2", "api", "aa:bb:cc:dd:ee:02", "100.64.0.5", "100.64.0.6", 100)
	dns2 := makeDNS("api-1.api.prod.internal", "100.64.0.5", "net1")

	sc.ApplyDelta(&pb.HostNetworkStateUpdate{
		Full:             false,
		Version:          2,
		RemovedPolicyIds: []string{"pol1"},
		State: &pb.HostNetworkState{
			Ports:      []*pb.PortState{port2},
			DnsRecords: []*pb.DnsRecord{dns2},
		},
	})

	if sc.GetVersion() != 2 {
		t.Errorf("expected version 2, got %d", sc.GetVersion())
	}

	// port1 should still exist
	if p := sc.GetPortByIP("100.64.0.1"); p == nil {
		t.Error("port1 should still exist")
	}
	// port2 should be added
	if p := sc.GetPortByIP("100.64.0.5"); p == nil || p.PortId != "p2" {
		t.Error("port2 should be added")
	}
	// pol1 should be removed
	if len(sc.GetPolicies()) != 0 {
		t.Error("pol1 should be removed")
	}
	// Both DNS records should exist
	records := sc.GetDNSRecordsForNetwork("net1")
	if len(records) != 2 {
		t.Errorf("expected 2 DNS records, got %d", len(records))
	}
}

func TestStateCache_DeltaRemovePort(t *testing.T) {
	sc := NewStateCache()

	port1 := makePort("p1", "vm1", "web-1", "net1", "prod", "g1", "web", "aa:bb:cc:dd:ee:01", "100.64.0.1", "100.64.0.2", 100)

	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full:    true,
		Version: 1,
		State: &pb.HostNetworkState{
			Ports: []*pb.PortState{port1},
		},
	})

	// Remove port
	sc.ApplyDelta(&pb.HostNetworkStateUpdate{
		Full:           false,
		Version:        2,
		RemovedPortIds: []string{"p1"},
	})

	if p := sc.GetPortByIP("100.64.0.1"); p != nil {
		t.Error("port should be removed")
	}
	if p := sc.GetPortByMAC("aa:bb:cc:dd:ee:01"); p != nil {
		t.Error("MAC lookup should be cleared")
	}
	if len(sc.GetLocalPorts()) != 0 {
		t.Error("no local ports should remain")
	}
}

func TestStateCache_Snapshot(t *testing.T) {
	sc := NewStateCache()

	port1 := makePort("p1", "vm1", "web-1", "net1", "prod", "g1", "web", "aa:bb:cc:dd:ee:01", "100.64.0.1", "100.64.0.2", 100)
	pol1 := makePolicy("pol1", "net1", "g1", "g2", "tcp", 80, 1000, "allow")
	rp1 := makeRemotePort("net1", "g2", "100.64.0.5", "10.0.0.2", 100)
	dns1 := makeDNS("web-1.web.prod.internal", "100.64.0.1", "net1")

	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full:    true,
		Version: 1,
		State: &pb.HostNetworkState{
			Ports:       []*pb.PortState{port1},
			Policies:    []*pb.PolicyRule{pol1},
			RemotePorts: []*pb.RemotePort{rp1},
			DnsRecords:  []*pb.DnsRecord{dns1},
		},
	})

	snap := sc.Snapshot()
	if len(snap.Ports) != 1 {
		t.Errorf("expected 1 port in snapshot, got %d", len(snap.Ports))
	}
	if len(snap.Policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(snap.Policies))
	}
	if len(snap.RemotePorts) != 1 {
		t.Errorf("expected 1 remote port, got %d", len(snap.RemotePorts))
	}
	if len(snap.DnsRecords) != 1 {
		t.Errorf("expected 1 DNS record, got %d", len(snap.DnsRecords))
	}
}

// TestApplyDelta_RemovedEgressIngressIDs verifies that the bug fix for
// removed_egress_ids and removed_ingress_ids handling works correctly.
func TestApplyDelta_RemovedEgressIngressIDs(t *testing.T) {
	sc := NewStateCache()

	// First apply a full state with egress and ingress rules.
	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full: true,
		State: &pb.HostNetworkState{
			EgressRules: []*pb.EgressRule{
				{EgressId: "egress-1", NetworkId: "net-1", Type: "nat_gateway", PublicIp: "1.2.3.4"},
				{EgressId: "egress-2", NetworkId: "net-1", Type: "vpn_ipsec"},
				{EgressId: "egress-3", NetworkId: "net-1", Type: "vpn_wireguard"},
			},
			IngressRules: []*pb.IngressRule{
				{IngressId: "ingress-1", NetworkId: "net-1", Type: "direct_ip"},
				{IngressId: "ingress-2", NetworkId: "net-1", Type: "direct_ip"},
			},
		},
		Version: 1,
	})

	snap := sc.Snapshot()
	if len(snap.EgressRules) != 3 {
		t.Fatalf("expected 3 egress rules after full, got %d", len(snap.EgressRules))
	}
	if len(snap.IngressRules) != 2 {
		t.Fatalf("expected 2 ingress rules after full, got %d", len(snap.IngressRules))
	}

	// Now apply a delta that removes one egress and one ingress rule.
	sc.ApplyDelta(&pb.HostNetworkStateUpdate{
		Full:              false,
		RemovedEgressIds:  []string{"egress-2"},
		RemovedIngressIds: []string{"ingress-1"},
		Version:           2,
	})

	snap = sc.Snapshot()
	if len(snap.EgressRules) != 2 {
		t.Errorf("expected 2 egress rules after delta remove, got %d", len(snap.EgressRules))
	}
	for _, r := range snap.EgressRules {
		if r.EgressId == "egress-2" {
			t.Error("egress-2 should have been removed")
		}
	}
	if len(snap.IngressRules) != 1 {
		t.Errorf("expected 1 ingress rule after delta remove, got %d", len(snap.IngressRules))
	}
	if snap.IngressRules[0].IngressId != "ingress-2" {
		t.Errorf("expected ingress-2 to remain, got %q", snap.IngressRules[0].IngressId)
	}
}

// TestApplyDelta_AddEgressRule verifies that delta updates can add new egress rules.
func TestApplyDelta_AddEgressRule(t *testing.T) {
	sc := NewStateCache()

	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full: true,
		State: &pb.HostNetworkState{
			EgressRules: []*pb.EgressRule{
				{EgressId: "egress-1", Type: "nat_gateway"},
			},
		},
		Version: 1,
	})

	sc.ApplyDelta(&pb.HostNetworkStateUpdate{
		Full: false,
		State: &pb.HostNetworkState{
			EgressRules: []*pb.EgressRule{
				{EgressId: "egress-2", Type: "vpn_wireguard"},
			},
		},
		Version: 2,
	})

	snap := sc.Snapshot()
	if len(snap.EgressRules) != 2 {
		t.Errorf("expected 2 egress rules after delta add, got %d", len(snap.EgressRules))
	}
}

// TestEgressTypeForID verifies EgressTypeForID returns the correct type for cached rules.
func TestEgressTypeForID(t *testing.T) {
	sc := NewStateCache()

	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full: true,
		State: &pb.HostNetworkState{
			EgressRules: []*pb.EgressRule{
				{EgressId: "egress-nat", Type: "nat_gateway"},
				{EgressId: "egress-ipsec", Type: "vpn_ipsec"},
				{EgressId: "egress-wg", Type: "vpn_wireguard"},
				{EgressId: "egress-dc", Type: "direct_connect"},
			},
		},
		Version: 1,
	})

	cases := []struct {
		id       string
		wantType string
	}{
		{"egress-nat", "nat_gateway"},
		{"egress-ipsec", "vpn_ipsec"},
		{"egress-wg", "vpn_wireguard"},
		{"egress-dc", "direct_connect"},
		{"egress-missing", ""},
	}

	for _, tc := range cases {
		got := sc.EgressTypeForID(tc.id)
		if got != tc.wantType {
			t.Errorf("EgressTypeForID(%q) = %q, want %q", tc.id, got, tc.wantType)
		}
	}
}

// TestApplyDelta_DirectConnectRemoval verifies that direct_connect egress rules are
// removed correctly via delta updates and that EgressTypeForID reflects the removal.
func TestApplyDelta_DirectConnectRemoval(t *testing.T) {
	sc := NewStateCache()

	sc.ApplyFull(&pb.HostNetworkStateUpdate{
		Full: true,
		State: &pb.HostNetworkState{
			EgressRules: []*pb.EgressRule{
				{EgressId: "egress-dc", Type: "direct_connect", NetworkId: "net-1"},
				{EgressId: "egress-nat", Type: "nat_gateway"},
			},
		},
		Version: 1,
	})

	// Verify both rules are present before removal.
	if sc.EgressTypeForID("egress-dc") != network.EgressTypeDirectConnect {
		t.Error("expected direct_connect type before removal")
	}

	// Apply delta that removes the direct_connect rule.
	sc.ApplyDelta(&pb.HostNetworkStateUpdate{
		Full:             false,
		RemovedEgressIds: []string{"egress-dc"},
		Version:          2,
	})

	// After removal, the type lookup should return empty string.
	if got := sc.EgressTypeForID("egress-dc"); got != "" {
		t.Errorf("expected empty type after removal, got %q", got)
	}
	// nat_gateway should still be present.
	if sc.EgressTypeForID("egress-nat") != "nat_gateway" {
		t.Error("nat_gateway egress rule should not be affected by direct_connect removal")
	}

	snap := sc.Snapshot()
	if len(snap.EgressRules) != 1 {
		t.Errorf("expected 1 egress rule after removal, got %d", len(snap.EgressRules))
	}
}
