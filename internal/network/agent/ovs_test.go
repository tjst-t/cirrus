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
