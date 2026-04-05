package agent_test

import (
	"log/slog"
	"os"
	"testing"

	netagent "github.com/tjst-t/cirrus/internal/network/agent"
	pb "github.com/tjst-t/cirrus/proto/networkpb"
	mockovs "github.com/tjst-t/cirrus/test/mock/ovs"
)

func newTestPipeline(t *testing.T) (*netagent.Pipeline, *mockovs.MockClient) {
	t.Helper()
	mock := mockovs.New("br-int")

	// Pre-add the local port so GetOfPort works
	if err := mock.AddPort("br-int", "port-1-uuid-ab"); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	pipeline := netagent.NewPipeline(mock, logger)

	return pipeline, mock
}

func TestPipeline_Apply_SingleLocalPort(t *testing.T) {
	pipeline, mock := newTestPipeline(t)

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:      "port-1-uuid-abcdef",
				VmId:        "vm-1",
				VmName:      "web-1",
				NetworkId:   "net-1",
				NetworkName: "prod",
				GroupId:     "11111111-1111-1111-1111-111111111111",
				GroupName:   "web",
				MacAddress:  "aa:bb:cc:dd:ee:01",
				IpAddress:   "100.64.0.1",
				GatewayIp:   "100.64.0.2",
				Vni:         100,
			},
		},
		Policies: []*pb.PolicyRule{
			{
				PolicyId:   "pol-1",
				NetworkId:  "net-1",
				SrcGroupId: "11111111-1111-1111-1111-111111111111",
				DstGroupId: "11111111-1111-1111-1111-111111111111",
				Protocol:   "any",
				Priority:   1000,
				Action:     "allow",
			},
		},
	}

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify flows were installed in all tables
	for table := 0; table <= 7; table++ {
		flows, err := mock.GetFlows(table)
		if err != nil {
			t.Fatalf("GetFlows(%d): %v", table, err)
		}
		if len(flows) == 0 {
			t.Errorf("no flows in table %d", table)
		}
	}

	// Verify recorded commands include flow bundles
	cmds := mock.GetRecordedCommands()
	hasBundleAdd := false
	for _, cmd := range cmds {
		if cmd.Op == "add-flow-bundle" {
			hasBundleAdd = true
			break
		}
	}
	if !hasBundleAdd {
		t.Error("expected add-flow-bundle commands")
	}
}

func TestPipeline_Apply_WithRemotePort(t *testing.T) {
	pipeline, mock := newTestPipeline(t)

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:      "port-1-uuid-abcdef",
				VmId:        "vm-1",
				VmName:      "web-1",
				NetworkId:   "net-1",
				NetworkName: "prod",
				GroupId:     "11111111-1111-1111-1111-111111111111",
				GroupName:   "web",
				MacAddress:  "aa:bb:cc:dd:ee:01",
				IpAddress:   "100.64.0.1",
				GatewayIp:   "100.64.0.2",
				Vni:         100,
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
	}

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify tunnel port was created
	tunnels := mock.GetTunnelPorts()
	if len(tunnels) == 0 {
		t.Error("expected tunnel port to be created for remote host")
	}

	// Verify Table 4 has remote dst resolution
	flows, _ := mock.GetFlows(4)
	hasRemoteDst := false
	for _, f := range flows {
		if f.Match != "" && containsSubstr(f.Actions, "tun_dst") {
			hasRemoteDst = true
			break
		}
	}
	if !hasRemoteDst {
		t.Error("expected remote destination flow in Table 4")
	}
}

func TestPipeline_Apply_Idempotent(t *testing.T) {
	pipeline, mock := newTestPipeline(t)

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:      "port-1-uuid-abcdef",
				VmId:        "vm-1",
				VmName:      "web-1",
				NetworkId:   "net-1",
				NetworkName: "prod",
				GroupId:     "11111111-1111-1111-1111-111111111111",
				GroupName:   "web",
				MacAddress:  "aa:bb:cc:dd:ee:01",
				IpAddress:   "100.64.0.1",
				GatewayIp:   "100.64.0.2",
				Vni:         100,
			},
		},
	}

	// First apply
	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("first Apply failed: %v", err)
	}
	cmds1 := len(mock.GetRecordedCommands())

	// Second apply (same state) should produce no additional bundle commands
	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("second Apply failed: %v", err)
	}
	cmds2 := len(mock.GetRecordedCommands())

	if cmds2 != cmds1 {
		t.Errorf("idempotent apply should not produce new commands: before=%d after=%d", cmds1, cmds2)
	}
}

func TestPipeline_Apply_EgressRules_GWHost(t *testing.T) {
	pipeline, mock := newTestPipeline(t)

	// A GW-role host receives EgressRules and GatewayInfo in the state.
	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:      "port-1-uuid-abcdef",
				VmId:        "vm-1",
				VmName:      "web-1",
				NetworkId:   "net-1",
				NetworkName: "prod",
				GroupId:     "11111111-1111-1111-1111-111111111111",
				GroupName:   "web",
				MacAddress:  "aa:bb:cc:dd:ee:01",
				IpAddress:   "100.64.0.1",
				GatewayIp:   "100.64.0.2",
				Vni:         100,
			},
		},
		GatewayInfo: &pb.GatewayInfo{
			ExternalIp: "203.0.113.1",
			InternalIp: "10.0.0.1",
		},
		EgressRules: []*pb.EgressRule{
			{
				EgressId:    "egress-1-uuid",
				NetworkId:   "net-1",
				Type:        "nat_gateway",
				PublicIp:    "203.0.113.1",
				NetworkCidr: "100.64.0.0/24",
			},
		},
	}

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify a SNAT flow was installed in Table 7 (Egress) via AddFlow
	cmds := mock.GetRecordedCommands()
	hasSNATFlow := false
	for _, cmd := range cmds {
		if cmd.Op == "add-flow" {
			if containsSubstr(cmd.Match, "nw_src=100.64.0.0/24") || containsSubstr(cmd.Actions, "nat(src=203.0.113.1)") {
				hasSNATFlow = true
				break
			}
		}
	}
	if !hasSNATFlow {
		t.Error("expected SNAT flow to be installed for egress rule")
	}
}

func TestPipeline_Apply_NoEgressRules_NonGWHost(t *testing.T) {
	pipeline, mock := newTestPipeline(t)

	// A non-GW host has no GatewayInfo and no EgressRules.
	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:      "port-1-uuid-abcdef",
				VmId:        "vm-1",
				VmName:      "web-1",
				NetworkId:   "net-1",
				NetworkName: "prod",
				GroupId:     "11111111-1111-1111-1111-111111111111",
				GroupName:   "web",
				MacAddress:  "aa:bb:cc:dd:ee:01",
				IpAddress:   "100.64.0.1",
				GatewayIp:   "100.64.0.2",
				Vni:         100,
			},
		},
		// No GatewayInfo and no EgressRules — non-GW host
	}

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed for non-GW host: %v", err)
	}

	// Should NOT install any SNAT (AddFlow) commands with nat(src=...)
	cmds := mock.GetRecordedCommands()
	for _, cmd := range cmds {
		if cmd.Op == "add-flow" && containsSubstr(cmd.Actions, "nat(src=") {
			t.Error("unexpected SNAT flow installed on non-GW host")
		}
	}
}

func TestPipeline_Apply_IngressRules_GWHost(t *testing.T) {
	pipeline, mock := newTestPipeline(t)

	// A GW-role host receives IngressRules and GatewayInfo in the state.
	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:      "port-1-uuid-abcdef",
				VmId:        "vm-1",
				VmName:      "web-1",
				NetworkId:   "net-1",
				NetworkName: "prod",
				GroupId:     "11111111-1111-1111-1111-111111111111",
				GroupName:   "web",
				MacAddress:  "aa:bb:cc:dd:ee:01",
				IpAddress:   "100.64.0.1",
				GatewayIp:   "100.64.0.2",
				Vni:         100,
			},
		},
		GatewayInfo: &pb.GatewayInfo{
			ExternalIp: "203.0.113.1",
			InternalIp: "10.0.0.1",
		},
		IngressRules: []*pb.IngressRule{
			{
				IngressId: "ingress-1-uuid",
				NetworkId: "net-1",
				Type:      "direct_ip",
				PublicIp:  "203.0.113.42",
				TargetIp:  "100.64.0.1",
			},
		},
	}

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify a DNAT flow was installed via AddFlow
	cmds := mock.GetRecordedCommands()
	hasDNATFlow := false
	for _, cmd := range cmds {
		if cmd.Op == "add-flow" && containsSubstr(cmd.Match, "203.0.113.42") && containsSubstr(cmd.Actions, "nat(dst=") {
			hasDNATFlow = true
			break
		}
	}
	if !hasDNATFlow {
		t.Error("expected DNAT flow installed for direct_ip ingress rule")
	}
}

func TestPipeline_Apply_NoIngressRules_NonGWHost(t *testing.T) {
	pipeline, mock := newTestPipeline(t)

	// A non-GW host does NOT receive IngressRules or GatewayInfo.
	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:      "port-1-uuid-abcdef",
				VmId:        "vm-1",
				VmName:      "web-1",
				NetworkId:   "net-1",
				NetworkName: "prod",
				GroupId:     "11111111-1111-1111-1111-111111111111",
				GroupName:   "web",
				MacAddress:  "aa:bb:cc:dd:ee:01",
				IpAddress:   "100.64.0.1",
				GatewayIp:   "100.64.0.2",
				Vni:         100,
			},
		},
		// No GatewayInfo, no IngressRules
	}

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed for non-GW host: %v", err)
	}

	// Should NOT install any DNAT (AddFlow) commands with nat(dst=...)
	cmds := mock.GetRecordedCommands()
	for _, cmd := range cmds {
		if cmd.Op == "add-flow" && containsSubstr(cmd.Actions, "nat(dst=") {
			t.Error("unexpected DNAT flow installed on non-GW host")
		}
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
