package reconcile

import (
	"testing"

	pb "github.com/tjst-t/cirrus/proto/networkpb"

	"log/slog"
)

func TestCheckConsistency_NoWarnings(t *testing.T) {
	r := &NetworkReconciler{logger: slog.Default()}

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "p1", GroupId: "g1", NetworkId: "n1", IpAddress: "10.0.0.1"},
		},
		Policies: []*pb.PolicyRule{
			{PolicyId: "pol1", SrcGroupId: "g1", DstGroupId: "g1"},
		},
		RemotePorts: []*pb.RemotePort{
			{IpAddress: "10.0.0.5", GroupId: "g1", NetworkId: "n1"},
		},
		DnsRecords: []*pb.DnsRecord{
			{Name: "vm1.web.net.internal", Ip: "10.0.0.1", NetworkId: "n1"},
		},
	}

	warnings := r.checkConsistency(state, "test-host")
	if warnings != 0 {
		t.Fatalf("expected 0 warnings, got %d", warnings)
	}
}

func TestCheckConsistency_PortMissingGroup(t *testing.T) {
	r := &NetworkReconciler{logger: slog.Default()}

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "p1", GroupId: "", NetworkId: "n1"},
		},
	}

	warnings := r.checkConsistency(state, "test-host")
	if warnings != 1 {
		t.Fatalf("expected 1 warning for missing group, got %d", warnings)
	}
}

func TestCheckConsistency_PolicyUnknownGroups(t *testing.T) {
	r := &NetworkReconciler{logger: slog.Default()}

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "p1", GroupId: "g1", NetworkId: "n1"},
		},
		Policies: []*pb.PolicyRule{
			{PolicyId: "pol1", SrcGroupId: "g-unknown", DstGroupId: "g-also-unknown"},
		},
	}

	warnings := r.checkConsistency(state, "test-host")
	if warnings != 1 {
		t.Fatalf("expected 1 warning for unknown groups, got %d", warnings)
	}
}

func TestCheckConsistency_DNSUnknownNetwork(t *testing.T) {
	r := &NetworkReconciler{logger: slog.Default()}

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "p1", GroupId: "g1", NetworkId: "n1"},
		},
		DnsRecords: []*pb.DnsRecord{
			{Name: "vm1.web.net.internal", Ip: "10.0.0.1", NetworkId: "n-unknown"},
		},
	}

	warnings := r.checkConsistency(state, "test-host")
	if warnings != 1 {
		t.Fatalf("expected 1 warning for unknown network, got %d", warnings)
	}
}
