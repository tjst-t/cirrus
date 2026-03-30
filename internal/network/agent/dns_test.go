package agent

import (
	"log/slog"
	"os"
	"testing"

	"github.com/miekg/dns"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

func setupDNSTest() (*DNSServer, *StateCache) {
	cache := NewStateCache()
	cache.ApplyFull(&pb.HostNetworkStateUpdate{
		Full:    true,
		Version: 1,
		State: &pb.HostNetworkState{
			Ports: []*pb.PortState{
				makePort("p1", "vm1", "web-1", "net1", "prod", "g1", "web", "aa:bb:cc:dd:ee:01", "100.64.0.1", "100.64.0.2", 100),
				makePort("p2", "vm2", "web-2", "net1", "prod", "g1", "web", "aa:bb:cc:dd:ee:02", "100.64.0.5", "100.64.0.6", 100),
			},
			DnsRecords: []*pb.DnsRecord{
				makeDNS("web-1.web.prod.internal", "100.64.0.1", "net1"),
				makeDNS("web-2.web.prod.internal", "100.64.0.5", "net1"),
				// Record in a different network (should be isolated)
				makeDNS("api-1.api.staging.internal", "100.64.4.1", "net2"),
			},
		},
	})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	server := NewDNSServer(cache, logger, "")
	return server, cache
}

func TestDNS_ResolveA_ExactMatch(t *testing.T) {
	server, _ := setupDNSTest()

	answers := server.ResolveA("web-1.web.prod.internal.", "net1")
	if len(answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(answers))
	}

	a, ok := answers[0].(*dns.A)
	if !ok {
		t.Fatal("expected A record")
	}
	if a.A.String() != "100.64.0.1" {
		t.Errorf("expected 100.64.0.1, got %s", a.A)
	}
}

func TestDNS_ResolveA_GroupLevel(t *testing.T) {
	server, _ := setupDNSTest()

	// "web.prod.internal" should match all VMs in the web group
	answers := server.ResolveA("web.prod.internal.", "net1")
	if len(answers) != 2 {
		t.Fatalf("expected 2 answers (group-level), got %d", len(answers))
	}

	ips := make(map[string]bool)
	for _, rr := range answers {
		a := rr.(*dns.A)
		ips[a.A.String()] = true
	}
	if !ips["100.64.0.1"] || !ips["100.64.0.5"] {
		t.Errorf("expected both web VMs, got %v", ips)
	}
}

func TestDNS_NetworkIsolation(t *testing.T) {
	server, _ := setupDNSTest()

	// net1 should not see net2's records
	answers := server.ResolveA("api-1.api.staging.internal.", "net1")
	if len(answers) != 0 {
		t.Errorf("net1 should not resolve net2 records, got %d answers", len(answers))
	}
}

func TestDNS_ResolvePTR(t *testing.T) {
	server, _ := setupDNSTest()

	answers := server.ResolvePTR("1.0.64.100.in-addr.arpa.", "net1")
	if len(answers) != 1 {
		t.Fatalf("expected 1 PTR answer, got %d", len(answers))
	}

	ptr, ok := answers[0].(*dns.PTR)
	if !ok {
		t.Fatal("expected PTR record")
	}
	if ptr.Ptr != "web-1.web.prod.internal." {
		t.Errorf("expected web-1.web.prod.internal., got %s", ptr.Ptr)
	}
}

func TestPtrToIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0.64.100.in-addr.arpa.", "100.64.0.1"},
		{"5.0.64.100.in-addr.arpa.", "100.64.0.5"},
		{"invalid", ""},
		{"1.2.3.in-addr.arpa.", ""},
	}

	for _, tt := range tests {
		got := ptrToIP(tt.input)
		if got != tt.want {
			t.Errorf("ptrToIP(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
