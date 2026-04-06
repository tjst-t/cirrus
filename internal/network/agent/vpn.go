package agent

import (
	"context"
	"log/slog"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// VPNManager manages VPN tunnels (IPsec and WireGuard) on a GW-role worker host.
type VPNManager interface {
	// ConfigureIPsec creates or updates a strongSwan IKEv2 IPsec connection
	// according to the EgressRule configuration.
	ConfigureIPsec(ctx context.Context, rule *pb.EgressRule) error

	// RemoveIPsec tears down and removes the strongSwan IPsec connection for
	// the given egress ID.
	RemoveIPsec(ctx context.Context, egressID string) error

	// ConfigureWireGuard creates or updates a WireGuard interface and peer
	// according to the EgressRule configuration.
	ConfigureWireGuard(ctx context.Context, rule *pb.EgressRule) error

	// RemoveWireGuard tears down and removes the WireGuard interface for the
	// given egress ID.
	RemoveWireGuard(ctx context.Context, egressID string) error
}

// SimVPNManager is a no-op VPNManager for simulation environments.
// It logs intended operations without performing any real system calls.
type SimVPNManager struct {
	logger *slog.Logger
}

// NewSimVPNManager creates a new SimVPNManager.
func NewSimVPNManager(logger *slog.Logger) *SimVPNManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SimVPNManager{logger: logger}
}

// ConfigureIPsec is a no-op that logs what it would do.
func (m *SimVPNManager) ConfigureIPsec(ctx context.Context, rule *pb.EgressRule) error {
	cfg := rule.GetVpnIpsec()
	if cfg == nil {
		m.logger.Info("sim: configure ipsec (no config)", "egress_id", rule.EgressId)
		return nil
	}
	m.logger.Info("sim: configure ipsec",
		"egress_id", rule.EgressId,
		"peer_ip", cfg.PeerIp,
		"local_cidr", cfg.LocalCidr,
		"remote_cidr", cfg.RemoteCidr,
	)
	return nil
}

// RemoveIPsec is a no-op that logs what it would do.
func (m *SimVPNManager) RemoveIPsec(ctx context.Context, egressID string) error {
	m.logger.Info("sim: remove ipsec", "egress_id", egressID)
	return nil
}

// ConfigureWireGuard is a no-op that logs what it would do.
func (m *SimVPNManager) ConfigureWireGuard(ctx context.Context, rule *pb.EgressRule) error {
	cfg := rule.GetVpnWireguard()
	if cfg == nil {
		m.logger.Info("sim: configure wireguard (no config)", "egress_id", rule.EgressId)
		return nil
	}
	m.logger.Info("sim: configure wireguard",
		"egress_id", rule.EgressId,
		"peer_endpoint", cfg.PeerEndpoint,
		"listen_port", cfg.ListenPort,
		"allowed_ips", cfg.AllowedIps,
	)
	return nil
}

// RemoveWireGuard is a no-op that logs what it would do.
func (m *SimVPNManager) RemoveWireGuard(ctx context.Context, egressID string) error {
	m.logger.Info("sim: remove wireguard", "egress_id", egressID)
	return nil
}
