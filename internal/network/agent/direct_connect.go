package agent

import (
	"context"
	"log/slog"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// DirectConnectManager manages Direct Connect VLAN trunk configuration on a GW-role worker host.
type DirectConnectManager interface {
	// ConfigureVLANTrunk creates or updates the VLAN trunk for the given EgressRule,
	// tagging the uplink port with the specified VLAN ID.
	ConfigureVLANTrunk(ctx context.Context, rule *pb.EgressRule) error

	// RemoveVLANTrunk tears down and removes the VLAN trunk configuration for
	// the given egress ID.
	RemoveVLANTrunk(ctx context.Context, egressID string) error
}

// SimDirectConnectManager is a no-op DirectConnectManager for simulation environments.
// It logs intended operations without performing any real system calls.
type SimDirectConnectManager struct {
	logger *slog.Logger
}

// NewSimDirectConnectManager creates a new SimDirectConnectManager.
func NewSimDirectConnectManager(logger *slog.Logger) *SimDirectConnectManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SimDirectConnectManager{logger: logger}
}

// ConfigureVLANTrunk is a no-op that logs what it would do.
func (m *SimDirectConnectManager) ConfigureVLANTrunk(ctx context.Context, rule *pb.EgressRule) error {
	cfg := rule.GetDirectConnect()
	if cfg == nil {
		m.logger.Info("sim: configure vlan trunk (no config)", "egress_id", rule.EgressId)
		return nil
	}
	m.logger.Info("sim: configure vlan trunk",
		"egress_id", rule.EgressId,
		"vlan_id", cfg.VlanId,
		"uplink_port", cfg.UplinkPort,
	)
	return nil
}

// RemoveVLANTrunk is a no-op that logs what it would do.
func (m *SimDirectConnectManager) RemoveVLANTrunk(ctx context.Context, egressID string) error {
	m.logger.Info("sim: remove vlan trunk", "egress_id", egressID)
	return nil
}
