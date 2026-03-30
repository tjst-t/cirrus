package agent

import (
	"strings"
	"testing"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

const (
	testGatewayMAC = "02:00:00:00:00:01"
)

func makeTestContext() *FlowContext {
	groupHash := groupIDHash("11111111-1111-1111-1111-111111111111")
	return &FlowContext{
		LocalPorts: []PortInfo{
			{
				PortID:    "port-1",
				OfPort:    1,
				MAC:       "aa:bb:cc:dd:ee:01",
				IP:        "100.64.0.1",
				GatewayIP: "100.64.0.2",
				VNI:       100,
				GroupHash: groupHash,
				NetworkID: "net-1",
			},
		},
		RemotePorts: []*pb.RemotePort{
			{
				NetworkId: "net-1",
				GroupId:   "22222222-2222-2222-2222-222222222222",
				IpAddress: "100.64.0.5",
				HostIp:    "10.0.0.2",
				Vni:       100,
			},
		},
		Policies: []*pb.PolicyRule{
			{
				PolicyId:   "pol-1",
				NetworkId:  "net-1",
				SrcGroupId: "11111111-1111-1111-1111-111111111111",
				DstGroupId: "22222222-2222-2222-2222-222222222222",
				Protocol:   "tcp",
				DstPort:    80,
				Priority:   1000,
				Action:     "allow",
			},
		},
		TunnelOfPort: 10,
		GatewayMAC:   testGatewayMAC,
	}
}

func TestGenerateFlows_ProducesAllTables(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	tableFound := make(map[int]bool)
	for _, f := range flows {
		tableFound[f.Table] = true
	}

	for _, table := range []int{0, 1, 2, 3, 4, 5, 6, 7} {
		if !tableFound[table] {
			t.Errorf("no flows generated for table %d", table)
		}
	}
}

func TestGenerateFlows_PortSecurity(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	// Check that Table 0 has port security flow for the local port
	found := false
	for _, f := range flows {
		if f.Table == TableInputClassification && f.Priority == 100 &&
			strings.Contains(f.Match, "in_port=1") &&
			strings.Contains(f.Match, "dl_src=aa:bb:cc:dd:ee:01") &&
			strings.Contains(f.Match, "nw_src=100.64.0.1") {
			found = true
			if !strings.Contains(f.Actions, "NXM_NX_REG1") {
				t.Error("port security flow should load src group into REG1")
			}
			break
		}
	}
	if !found {
		t.Error("port security flow not found in Table 0")
	}
}

func TestGenerateFlows_DHCPRedirect(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TableInputClassification && f.Priority == 200 &&
			strings.Contains(f.Match, "tp_dst=67") {
			if f.Actions != "LOCAL" {
				t.Errorf("DHCP redirect should punt to LOCAL, got %s", f.Actions)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("DHCP redirect flow not found")
	}
}

func TestGenerateFlows_DNSRedirect(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TableInputClassification && f.Priority == 200 &&
			strings.Contains(f.Match, "tp_dst=53") &&
			strings.Contains(f.Match, "nw_dst=100.64.0.2") {
			found = true
			break
		}
	}
	if !found {
		t.Error("DNS redirect flow not found")
	}
}

func TestGenerateFlows_MetadataRedirect(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TableInputClassification && f.Priority == 200 &&
			strings.Contains(f.Match, "169.254.169.254") {
			found = true
			break
		}
	}
	if !found {
		t.Error("metadata redirect flow not found")
	}
}

func TestGenerateFlows_ARPReply(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TableInputClassification && f.Priority == 200 &&
			strings.Contains(f.Match, "arp") &&
			strings.Contains(f.Match, "arp_tpa=100.64.0.2") {
			if !strings.Contains(f.Actions, "NXM_OF_ARP_OP") {
				t.Error("ARP reply flow should set ARP op")
			}
			if !strings.Contains(f.Actions, "IN_PORT") {
				t.Error("ARP reply should send back to IN_PORT")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("ARP reply flow not found")
	}
}

func TestGenerateFlows_Conntrack(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	var ctFlows []FlowEntry
	for _, f := range flows {
		if f.Table == TableConntrack {
			ctFlows = append(ctFlows, f)
		}
	}

	// Should have: established, related, invalid, untracked, new + table-miss
	if len(ctFlows) < 5 {
		t.Errorf("expected at least 5 conntrack flows, got %d", len(ctFlows))
	}
}

func TestGenerateFlows_PolicyAllow(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TablePolicyEvaluation && f.Priority == 1000 &&
			strings.Contains(f.Match, "tcp") &&
			strings.Contains(f.Match, "tp_dst=80") {
			if !strings.Contains(f.Actions, "ct(commit)") {
				t.Error("allow policy should commit to conntrack")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("policy allow flow not found")
	}
}

func TestGenerateFlows_DstHostLocal(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TableDstHostResolution && strings.Contains(f.Match, "100.64.0.1") {
			if !strings.Contains(f.Actions, "load:1->NXM_NX_REG2") {
				t.Errorf("local dst resolution should set REG2 to ofport, got %s", f.Actions)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("local dst host resolution flow not found")
	}
}

func TestGenerateFlows_DstHostRemote(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TableDstHostResolution && strings.Contains(f.Match, "100.64.0.5") {
			if !strings.Contains(f.Actions, "tun_dst") || !strings.Contains(f.Actions, "10.0.0.2") {
				t.Errorf("remote dst resolution should set tun_dst, got %s", f.Actions)
			}
			if !strings.Contains(f.Actions, "tun_id") {
				t.Error("remote dst resolution should set tun_id (VNI)")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("remote dst host resolution flow not found")
	}
}

func TestGenerateFlows_TunnelInput(t *testing.T) {
	ctx := makeTestContext()
	flows := GenerateFlows(ctx)

	found := false
	for _, f := range flows {
		if f.Table == TableInputClassification && strings.Contains(f.Match, "in_port=10") {
			found = true
			break
		}
	}
	if !found {
		t.Error("tunnel input flow not found")
	}
}

func TestGroupIDHash_Deterministic(t *testing.T) {
	id := "11111111-1111-1111-1111-111111111111"
	h1 := groupIDHash(id)
	h2 := groupIDHash(id)
	if h1 != h2 {
		t.Error("groupIDHash should be deterministic")
	}
	if h1 == 0 {
		t.Error("groupIDHash should not be 0 for valid UUID")
	}
}

func TestMacToOFHex(t *testing.T) {
	got := macToOFHex("aa:bb:cc:dd:ee:ff")
	want := "0xaabbccddeeff"
	if got != want {
		t.Errorf("macToOFHex = %s, want %s", got, want)
	}
}

func TestIpToHex(t *testing.T) {
	got := ipToHex("100.64.0.2")
	want := "0x64400002"
	if got != want {
		t.Errorf("ipToHex = %s, want %s", got, want)
	}
}
