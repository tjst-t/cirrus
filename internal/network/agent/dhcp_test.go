package agent

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func TestBuildDHCPResponse_Discover(t *testing.T) {
	hwAddr, _ := net.ParseMAC("aa:bb:cc:dd:ee:01")
	req, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover),
		dhcpv4.WithHwAddr(hwAddr),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := BuildDHCPResponse(req, "100.64.0.1", "100.64.0.2")
	if err != nil {
		t.Fatalf("BuildDHCPResponse: %v", err)
	}

	// Check message type
	if resp.MessageType() != dhcpv4.MessageTypeOffer {
		t.Errorf("expected Offer, got %s", resp.MessageType())
	}

	// Check YourIP
	if !resp.YourIPAddr.Equal(net.ParseIP("100.64.0.1")) {
		t.Errorf("YourIP = %s, want 100.64.0.1", resp.YourIPAddr)
	}

	// Check subnet mask
	mask := resp.SubnetMask()
	if mask == nil {
		t.Fatal("subnet mask not set")
	}
	ones, _ := net.IPMask(mask).Size()
	if ones != 30 {
		t.Errorf("subnet mask = /%d, want /30", ones)
	}

	// Check router (gateway)
	routers := resp.Router()
	if len(routers) == 0 || !routers[0].Equal(net.ParseIP("100.64.0.2")) {
		t.Errorf("router = %v, want [100.64.0.2]", routers)
	}

	// Check DNS
	dns := resp.DNS()
	if len(dns) == 0 || !dns[0].Equal(net.ParseIP("100.64.0.2")) {
		t.Errorf("DNS = %v, want [100.64.0.2]", dns)
	}
}

func TestBuildDHCPResponse_Request(t *testing.T) {
	hwAddr, _ := net.ParseMAC("aa:bb:cc:dd:ee:01")
	req, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(hwAddr),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := BuildDHCPResponse(req, "100.64.0.1", "100.64.0.2")
	if err != nil {
		t.Fatalf("BuildDHCPResponse: %v", err)
	}

	if resp.MessageType() != dhcpv4.MessageTypeAck {
		t.Errorf("expected Ack, got %s", resp.MessageType())
	}
}
