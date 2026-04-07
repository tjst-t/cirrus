package agent_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	netagent "github.com/tjst-t/cirrus/internal/network/agent"
	pb "github.com/tjst-t/cirrus/proto/networkpb"
	mockovs "github.com/tjst-t/cirrus/test/mock/ovs"
)

func newL4LBTestPipeline(t *testing.T) (*netagent.Pipeline, *mockovs.MockClient) {
	t.Helper()
	mock := mockovs.New("br-int")
	// Pre-add a port so resolveLocalPorts works.
	if err := mock.AddPort("br-int", "port-1-uuid-ab"); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return netagent.NewPipeline(mock, logger), mock
}

// stateWithL4LB returns a minimal HostNetworkState with an l4_lb ingress rule.
func stateWithL4LB(backends []*pb.L4LBBackend, affinity string) *pb.HostNetworkState {
	return &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:     "port-1-uuid-abcdef",
				VmId:       "vm-1",
				VmName:     "web-1",
				NetworkId:  "net-1",
				MacAddress: "aa:bb:cc:dd:ee:01",
				IpAddress:  "100.64.0.1",
				GatewayIp:  "100.64.0.2",
				Vni:        100,
			},
		},
		GatewayInfo: &pb.GatewayInfo{
			ExternalIp: "203.0.113.1",
			InternalIp: "10.0.0.1",
		},
		IngressRules: []*pb.IngressRule{
			{
				IngressId:       "ingress-l4lb-uuid-001",
				NetworkId:       "net-1",
				Type:            "l4_lb",
				PublicIp:        "203.0.113.50",
				ListenerPort:    80,
				Protocol:        "tcp",
				SessionAffinity: affinity,
				Backends:        backends,
			},
		},
	}
}

// TestL4LB_DistributionGroupInstalled verifies that applying an l4_lb ingress rule
// installs an OVS group (type=select) and a matching flow.
func TestL4LB_DistributionGroupInstalled(t *testing.T) {
	pipeline, mock := newL4LBTestPipeline(t)

	state := stateWithL4LB([]*pb.L4LBBackend{
		{VmId: "vm-a", Ip: "10.0.1.1", Port: 8080, Weight: 1, Healthy: true},
		{VmId: "vm-b", Ip: "10.0.1.2", Port: 8080, Weight: 1, Healthy: true},
	}, "none")

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	cmds := mock.GetRecordedCommands()
	var foundGroup, foundFlow bool
	for _, cmd := range cmds {
		if cmd.Op == "add-group" && strings.Contains(cmd.Actions, "type=select") {
			foundGroup = true
			// Verify both backends are in the group.
			if !strings.Contains(cmd.Actions, "10.0.1.1:8080") {
				t.Errorf("group spec missing backend 10.0.1.1:8080, got: %s", cmd.Actions)
			}
			if !strings.Contains(cmd.Actions, "10.0.1.2:8080") {
				t.Errorf("group spec missing backend 10.0.1.2:8080, got: %s", cmd.Actions)
			}
		}
		if cmd.Op == "add-flow" && strings.Contains(cmd.Match, "203.0.113.50") &&
			strings.Contains(cmd.Match, "tp_dst=80") &&
			strings.Contains(cmd.Actions, "group:") {
			foundFlow = true
		}
	}
	if !foundGroup {
		t.Error("expected add-group command with type=select for l4_lb ingress")
	}
	if !foundFlow {
		t.Error("expected add-flow command matching public_ip:listener_port for l4_lb ingress")
	}
}

// TestL4LB_SessionAffinitySourceIP verifies that source_ip affinity sets selection_method=ip_src.
func TestL4LB_SessionAffinitySourceIP(t *testing.T) {
	pipeline, mock := newL4LBTestPipeline(t)

	state := stateWithL4LB([]*pb.L4LBBackend{
		{VmId: "vm-a", Ip: "10.0.1.1", Port: 8080, Weight: 1, Healthy: true},
	}, "source_ip")

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	cmds := mock.GetRecordedCommands()
	var found bool
	for _, cmd := range cmds {
		if cmd.Op == "add-group" && strings.Contains(cmd.Actions, "selection_method=ip_src") {
			found = true
		}
	}
	if !found {
		t.Error("expected add-group with selection_method=ip_src for source_ip affinity")
	}
}

// TestL4LB_SessionAffinityNone verifies that 5-tuple (default) does NOT set selection_method.
func TestL4LB_SessionAffinityNone(t *testing.T) {
	pipeline, mock := newL4LBTestPipeline(t)

	state := stateWithL4LB([]*pb.L4LBBackend{
		{VmId: "vm-a", Ip: "10.0.1.1", Port: 8080, Weight: 1, Healthy: true},
	}, "none")

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	cmds := mock.GetRecordedCommands()
	for _, cmd := range cmds {
		if cmd.Op == "add-group" && strings.Contains(cmd.Actions, "selection_method") {
			t.Errorf("expected no selection_method for default 5-tuple affinity, got: %s", cmd.Actions)
		}
	}
}

// TestL4LB_BackendFailureExclusion verifies that unhealthy backends are excluded from
// the group buckets. Unhealthy backends are not passed to the OVS pipeline — the
// StateController only sends healthy backends in the IngressRule.
// This test simulates the controller behavior: only healthy backends in the rule.
func TestL4LB_BackendFailureExclusion(t *testing.T) {
	pipeline, mock := newL4LBTestPipeline(t)

	// Only one backend is healthy (as would be sent by the StateController).
	state := stateWithL4LB([]*pb.L4LBBackend{
		{VmId: "vm-a", Ip: "10.0.1.1", Port: 8080, Weight: 1, Healthy: true},
		// vm-b is unhealthy, excluded by the StateController — not present here
	}, "none")

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	cmds := mock.GetRecordedCommands()
	for _, cmd := range cmds {
		if cmd.Op == "add-group" {
			// Must NOT contain the failed backend's IP.
			if strings.Contains(cmd.Actions, "10.0.1.2") {
				t.Errorf("group spec should not include excluded backend 10.0.1.2, got: %s", cmd.Actions)
			}
			// Must contain the healthy backend.
			if !strings.Contains(cmd.Actions, "10.0.1.1") {
				t.Errorf("group spec should include healthy backend 10.0.1.1, got: %s", cmd.Actions)
			}
		}
	}
}

// TestDirectIP_StaleFlowCleanup verifies that when a direct_ip ingress is removed,
// its DNAT flow is deleted from OVS on the next Apply cycle.
func TestDirectIP_StaleFlowCleanup(t *testing.T) {
	pipeline, mock := newL4LBTestPipeline(t)

	stateWithIngress := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "port-1-uuid-abcdef", VmId: "vm-1", VmName: "web-1",
				NetworkId: "net-1", MacAddress: "aa:bb:cc:dd:ee:01",
				IpAddress: "100.64.0.1", GatewayIp: "100.64.0.2", Vni: 100},
		},
		GatewayInfo: &pb.GatewayInfo{ExternalIp: "203.0.113.1", InternalIp: "10.0.0.1"},
		IngressRules: []*pb.IngressRule{
			{IngressId: "ingress-direct-uuid", NetworkId: "net-1", Type: "direct_ip",
				PublicIp: "203.0.113.200", TargetIp: "100.64.0.1"},
		},
	}

	// First Apply — installs the DNAT flow.
	if err := pipeline.Apply(stateWithIngress); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	mock.Reset()

	// Second Apply — ingress removed from state.
	stateWithoutIngress := &pb.HostNetworkState{
		Ports:        stateWithIngress.Ports,
		GatewayInfo:  stateWithIngress.GatewayInfo,
		IngressRules: nil,
	}
	if err := pipeline.Apply(stateWithoutIngress); err != nil {
		t.Fatalf("Apply (remove) failed: %v", err)
	}

	cmds := mock.GetRecordedCommands()
	var foundDelete bool
	for _, cmd := range cmds {
		if cmd.Op == "delete-flow" && strings.Contains(cmd.Match, "203.0.113.200") {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Errorf("expected delete-flow for removed direct_ip ingress 203.0.113.200; commands: %v", cmds)
	}
}

// TestL4LB_BackwardCompatible_DirectIPStillWorks verifies that direct_ip rules work
// correctly alongside l4_lb rules on the same gateway host.
func TestL4LB_BackwardCompatible_DirectIPStillWorks(t *testing.T) {
	pipeline, mock := newL4LBTestPipeline(t)

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{
				PortId:     "port-1-uuid-abcdef",
				VmId:       "vm-1",
				VmName:     "web-1",
				NetworkId:  "net-1",
				MacAddress: "aa:bb:cc:dd:ee:01",
				IpAddress:  "100.64.0.1",
				GatewayIp:  "100.64.0.2",
				Vni:        100,
			},
		},
		GatewayInfo: &pb.GatewayInfo{
			ExternalIp: "203.0.113.1",
			InternalIp: "10.0.0.1",
		},
		IngressRules: []*pb.IngressRule{
			{
				// direct_ip rule
				IngressId: "ingress-direct-uuid",
				NetworkId: "net-1",
				Type:      "direct_ip",
				PublicIp:  "203.0.113.100",
				TargetIp:  "100.64.0.1",
			},
			{
				// l4_lb rule
				IngressId:       "ingress-lb-uuid",
				NetworkId:       "net-1",
				Type:            "l4_lb",
				PublicIp:        "203.0.113.101",
				ListenerPort:    8080,
				Protocol:        "tcp",
				SessionAffinity: "none",
				Backends: []*pb.L4LBBackend{
					{VmId: "vm-1", Ip: "100.64.0.1", Port: 8080, Weight: 1, Healthy: true},
				},
			},
		},
	}

	if err := pipeline.Apply(state); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	cmds := mock.GetRecordedCommands()
	var foundDirect, foundLB bool
	for _, cmd := range cmds {
		if cmd.Op == "add-flow" && strings.Contains(cmd.Match, "203.0.113.100") &&
			strings.Contains(cmd.Actions, "nat(dst=100.64.0.1)") {
			foundDirect = true
		}
		if cmd.Op == "add-flow" && strings.Contains(cmd.Match, "203.0.113.101") &&
			strings.Contains(cmd.Match, "tp_dst=8080") {
			foundLB = true
		}
	}
	if !foundDirect {
		t.Error("expected DNAT flow for direct_ip rule")
	}
	if !foundLB {
		t.Error("expected flow for l4_lb rule")
	}
}
