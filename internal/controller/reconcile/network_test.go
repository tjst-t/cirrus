package reconcile

import (
	"context"
	"log/slog"
	"testing"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// newTestDriftHandler returns a DriftHandler with no DB (no persistence) for testing.
func newTestDriftHandler() *DriftHandler {
	return NewDriftHandler(DriftHandlerConfig{
		Pool:            nil, // no DB in unit tests
		Logger:          slog.Default(),
		AutoHealEnabled: false,
	})
}

func newTestNetworkReconciler() *NetworkReconciler {
	return &NetworkReconciler{
		handler: newTestDriftHandler(),
		logger:  slog.Default(),
	}
}

func TestCheckConsistency_NoWarnings(t *testing.T) {
	r := newTestNetworkReconciler()

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

	drift := r.checkConsistency(context.Background(), state, "host-id-1", "test-host")
	if drift != 0 {
		t.Fatalf("expected 0 drift events, got %d", drift)
	}
}

func TestCheckConsistency_PortMissingGroup(t *testing.T) {
	r := newTestNetworkReconciler()

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "p1", GroupId: "", NetworkId: "n1"},
		},
	}

	drift := r.checkConsistency(context.Background(), state, "host-id-1", "test-host")
	if drift != 1 {
		t.Fatalf("expected 1 drift event for missing group, got %d", drift)
	}
}

func TestCheckConsistency_PolicyUnknownGroups(t *testing.T) {
	r := newTestNetworkReconciler()

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "p1", GroupId: "g1", NetworkId: "n1"},
		},
		Policies: []*pb.PolicyRule{
			{PolicyId: "pol1", SrcGroupId: "g-unknown", DstGroupId: "g-also-unknown"},
		},
	}

	drift := r.checkConsistency(context.Background(), state, "host-id-1", "test-host")
	if drift != 1 {
		t.Fatalf("expected 1 drift event for unknown groups, got %d", drift)
	}
}

func TestCheckConsistency_DNSUnknownNetwork(t *testing.T) {
	r := newTestNetworkReconciler()

	state := &pb.HostNetworkState{
		Ports: []*pb.PortState{
			{PortId: "p1", GroupId: "g1", NetworkId: "n1"},
		},
		DnsRecords: []*pb.DnsRecord{
			{Name: "vm1.web.net.internal", Ip: "10.0.0.1", NetworkId: "n-unknown"},
		},
	}

	drift := r.checkConsistency(context.Background(), state, "host-id-1", "test-host")
	if drift != 1 {
		t.Fatalf("expected 1 drift event for unknown network, got %d", drift)
	}
}
