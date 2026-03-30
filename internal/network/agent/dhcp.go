package agent

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

// DHCPServer responds to DHCP requests from VMs using port state from the cache.
type DHCPServer struct {
	cache  *StateCache
	logger *slog.Logger
	server *server4.Server
}

// NewDHCPServer creates a new DHCP server.
func NewDHCPServer(cache *StateCache, logger *slog.Logger) *DHCPServer {
	return &DHCPServer{
		cache:  cache,
		logger: logger,
	}
}

// ListenAndServe starts the DHCP server on the given interface.
func (s *DHCPServer) ListenAndServe(ifname string) error {
	server, err := server4.NewServer(
		ifname,
		nil, // address: nil means listen on 0.0.0.0:67
		s.handler,
	)
	if err != nil {
		return fmt.Errorf("dhcp: create server: %w", err)
	}
	s.server = server
	return server.Serve()
}

// Close stops the DHCP server.
func (s *DHCPServer) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

// handler processes incoming DHCP packets.
func (s *DHCPServer) handler(conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4) {
	if msg == nil {
		return
	}

	// Lookup port by client MAC
	port := s.cache.GetPortByMAC(msg.ClientHWAddr.String())
	if port == nil {
		s.logger.Debug("dhcp: unknown MAC, ignoring", "mac", msg.ClientHWAddr.String())
		return
	}

	resp, err := s.buildResponse(msg, port.IpAddress, port.GatewayIp)
	if err != nil {
		s.logger.Warn("dhcp: build response failed", "error", err)
		return
	}

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		s.logger.Warn("dhcp: send response failed", "error", err)
	}
}

// buildResponse creates a DHCP Offer or Ack based on the request type.
func (s *DHCPServer) buildResponse(req *dhcpv4.DHCPv4, vmIP, gwIP string) (*dhcpv4.DHCPv4, error) {
	ip := net.ParseIP(vmIP).To4()
	gw := net.ParseIP(gwIP).To4()
	if ip == nil || gw == nil {
		return nil, fmt.Errorf("invalid IP: vm=%s gw=%s", vmIP, gwIP)
	}

	// /30 subnet mask
	mask := net.CIDRMask(30, 32)

	resp, err := dhcpv4.NewReplyFromRequest(req,
		dhcpv4.WithYourIP(ip),
		dhcpv4.WithServerIP(gw),
		dhcpv4.WithOption(dhcpv4.OptSubnetMask(mask)),
		dhcpv4.WithOption(dhcpv4.OptRouter(gw)),
		dhcpv4.WithOption(dhcpv4.OptDNS(gw)),
		dhcpv4.WithLeaseTime(86400), // 24h
	)
	if err != nil {
		return nil, err
	}

	// Set message type based on request
	msgType := req.MessageType()
	switch msgType {
	case dhcpv4.MessageTypeDiscover:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	case dhcpv4.MessageTypeRequest:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	default:
		return nil, fmt.Errorf("unexpected DHCP message type: %s", msgType)
	}

	return resp, nil
}

// BuildDHCPResponse is exported for testing: given a DHCP request and VM/GW IPs,
// build the appropriate DHCP response.
func BuildDHCPResponse(req *dhcpv4.DHCPv4, vmIP, gwIP string) (*dhcpv4.DHCPv4, error) {
	s := &DHCPServer{}
	return s.buildResponse(req, vmIP, gwIP)
}
