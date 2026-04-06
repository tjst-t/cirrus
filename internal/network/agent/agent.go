package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/network"
	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// Config holds configuration for the NetworkAgent.
type Config struct {
	HostID         string
	ControllerAddr string
	RegToken       string
	OVSBridge      string
	Logger         *slog.Logger
}

// NetworkAgent manages the network data plane on a single worker host.
// It connects to the controller's NetworkStateService, receives state updates,
// and applies them via OVS flows, DHCP, DNS, metadata, and VPN services.
type NetworkAgent struct {
	config          Config
	conn            *grpc.ClientConn
	state           *StateCache
	pipeline        *Pipeline
	dhcp            *DHCPServer
	dns             *DNSServer
	metadata        *MetadataServer
	vpnManager      VPNManager
	dcManager       DirectConnectManager
	logger          *slog.Logger
}

// New creates a new NetworkAgent. Pass the shared gRPC connection from the worker agent.
// ovsClient can be nil; if so, pipeline Apply will be skipped (state-only mode for dev).
// vpnManager can be nil; if so, VPN configuration is skipped. Use NewSimVPNManager for
// simulation environments.
func New(cfg Config, conn *grpc.ClientConn, ovsClient OVSClient) *NetworkAgent {
	if cfg.OVSBridge == "" {
		cfg.OVSBridge = BridgeName
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	cache := NewStateCache()
	var pipeline *Pipeline
	if ovsClient != nil {
		pipeline = NewPipeline(ovsClient, cfg.Logger)
	}
	dhcpSrv := NewDHCPServer(cache, cfg.Logger)
	dnsSrv := NewDNSServer(cache, cfg.Logger, "")
	metaSrv := NewMetadataServer(cache, cfg.Logger)

	// Default to a no-op sim VPN manager so VPN egress rules are at least logged.
	vpnMgr := VPNManager(NewSimVPNManager(cfg.Logger))

	// Default to a no-op sim Direct Connect manager.
	dcMgr := DirectConnectManager(NewSimDirectConnectManager(cfg.Logger))

	return &NetworkAgent{
		config:    cfg,
		conn:      conn,
		state:     cache,
		pipeline:  pipeline,
		dhcp:      dhcpSrv,
		dns:       dnsSrv,
		metadata:  metaSrv,
		vpnManager: vpnMgr,
		dcManager:  dcMgr,
		logger:    cfg.Logger,
	}
}

// WithVPNManager sets the VPN manager used to apply VPN egress rules.
// Call this before Run to override the default SimVPNManager with a real implementation.
func (a *NetworkAgent) WithVPNManager(mgr VPNManager) *NetworkAgent {
	a.vpnManager = mgr
	return a
}

// WithDirectConnectManager sets the Direct Connect manager used to configure VLAN trunk rules.
// Call this before Run to override the default SimDirectConnectManager with a real implementation.
func (a *NetworkAgent) WithDirectConnectManager(mgr DirectConnectManager) *NetworkAgent {
	a.dcManager = mgr
	return a
}

// Run starts the network agent. It connects to the controller's
// NetworkStateService and processes state updates. Blocks until ctx is cancelled.
func (a *NetworkAgent) Run(ctx context.Context) error {
	a.logger.Info("network agent starting", "host_id", a.config.HostID)

	// Start service goroutines (DHCP/DNS/metadata)
	// These will be started when integration tests are set up.
	// For now, they depend on OVS bridge interface being ready.

	// Main state streaming loop with reconnection
	for {
		if err := a.streamLoop(ctx); err != nil {
			if ctx.Err() != nil {
				return nil // context cancelled, clean shutdown
			}
			a.logger.Warn("network state stream error, reconnecting", "error", err)
		}

		// Exponential backoff for reconnection
		if err := a.backoff(ctx); err != nil {
			return nil // context cancelled
		}
	}
}

// streamLoop connects to the controller and processes state updates.
func (a *NetworkAgent) streamLoop(ctx context.Context) error {
	client := pb.NewNetworkStateServiceClient(a.conn)

	stream, err := client.WatchHostNetworkState(ctx, &pb.WatchHostNetworkStateRequest{
		HostId:            a.config.HostID,
		RegistrationToken: a.config.RegToken,
	})
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	a.logger.Info("connected to network state service")

	for {
		update, err := stream.Recv()
		if err == io.EOF {
			return fmt.Errorf("stream closed by server")
		}
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}

		if err := a.applyUpdate(update); err != nil {
			a.logger.Error("failed to apply state update", "error", err, "version", update.Version)
		}
	}
}

// applyUpdate processes a HostNetworkStateUpdate from the controller.
func (a *NetworkAgent) applyUpdate(update *pb.HostNetworkStateUpdate) error {
	ctx := context.Background()

	// Apply VPN/DC removal dispatch BEFORE updating the state cache so that
	// EgressTypeForID lookups still find the rule being removed.
	if err := a.applyVPNRules(ctx, update); err != nil {
		a.logger.Error("failed to apply VPN rules", "error", err)
	}

	// Update state cache
	if update.Full {
		a.state.ApplyFull(update)
	} else {
		a.state.ApplyDelta(update)
	}

	// Apply OVS pipeline (skip if no OVS client configured)
	snapshot := a.state.Snapshot()
	if a.pipeline != nil {
		if err := a.pipeline.Apply(snapshot); err != nil {
			return fmt.Errorf("pipeline apply: %w", err)
		}
	}

	a.logger.Info("state update applied",
		"version", update.Version,
		"full", update.Full,
		"ports", len(snapshot.Ports),
		"policies", len(snapshot.Policies),
		"remote_ports", len(snapshot.RemotePorts),
	)

	return nil
}

// applyVPNRules applies VPN and Direct Connect egress rule configuration.
// It must be called BEFORE ApplyFull/ApplyDelta so that EgressTypeForID lookups
// can find rules that are being removed.
func (a *NetworkAgent) applyVPNRules(ctx context.Context, update *pb.HostNetworkStateUpdate) error {
	// Non-GW hosts never receive egress rules; skip all processing.
	if len(update.RemovedEgressIds) == 0 &&
		(update.GetState() == nil || len(update.GetState().EgressRules) == 0) {
		return nil
	}

	// Handle removals: look up the type from the state cache before dispatching.
	// The state cache still holds the rule at this point because applyVPNRules is
	// called before ApplyDelta processes the removal.
	for _, egressID := range update.RemovedEgressIds {
		egressType := a.state.EgressTypeForID(egressID)
		switch egressType {
		case network.EgressTypeVPNIPsec:
			if err := a.vpnManager.RemoveIPsec(ctx, egressID); err != nil {
				a.logger.Warn("vpn: remove ipsec failed", "egress_id", egressID, "error", err)
			}
		case network.EgressTypeVPNWireGuard:
			if err := a.vpnManager.RemoveWireGuard(ctx, egressID); err != nil {
				a.logger.Warn("vpn: remove wireguard failed", "egress_id", egressID, "error", err)
			}
		case network.EgressTypeDirectConnect:
			if err := a.dcManager.RemoveVLANTrunk(ctx, egressID); err != nil {
				a.logger.Warn("direct_connect: remove vlan trunk failed", "egress_id", egressID, "error", err)
			}
		default:
			// Not a managed egress (e.g. nat_gateway) or unknown — nothing to do.
		}
	}

	// Handle additions/updates from the state payload.
	state := update.GetState()
	if state == nil {
		return nil
	}
	for _, rule := range state.EgressRules {
		switch rule.Type {
		case network.EgressTypeVPNIPsec:
			if err := a.vpnManager.ConfigureIPsec(ctx, rule); err != nil {
				a.logger.Error("vpn: configure ipsec failed", "egress_id", rule.EgressId, "error", err)
			}
		case network.EgressTypeVPNWireGuard:
			if err := a.vpnManager.ConfigureWireGuard(ctx, rule); err != nil {
				a.logger.Error("vpn: configure wireguard failed", "egress_id", rule.EgressId, "error", err)
			}
		case network.EgressTypeDirectConnect:
			if err := a.dcManager.ConfigureVLANTrunk(ctx, rule); err != nil {
				a.logger.Error("direct_connect: configure vlan trunk failed", "egress_id", rule.EgressId, "error", err)
			}
		}
	}
	return nil
}

// backoff waits before reconnecting. Returns error only if ctx is cancelled.
func (a *NetworkAgent) backoff(ctx context.Context) error {
	const delay = 5 * time.Second
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// Close stops the network agent.
func (a *NetworkAgent) Close() {
	a.dhcp.Close()
	a.dns.Close()
	a.metadata.Close()
}
