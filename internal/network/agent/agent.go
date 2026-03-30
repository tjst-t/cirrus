package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc"

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
// and applies them via OVS flows, DHCP, DNS, and metadata services.
type NetworkAgent struct {
	config   Config
	conn     *grpc.ClientConn
	state    *StateCache
	pipeline *Pipeline
	dhcp     *DHCPServer
	dns      *DNSServer
	metadata *MetadataServer
	logger   *slog.Logger
}

// New creates a new NetworkAgent. Pass the shared gRPC connection from the worker agent.
// ovsClient can be nil; if so, pipeline Apply will be skipped (state-only mode for dev).
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

	return &NetworkAgent{
		config:   cfg,
		conn:     conn,
		state:    cache,
		pipeline: pipeline,
		dhcp:     dhcpSrv,
		dns:      dnsSrv,
		metadata: metaSrv,
		logger:   cfg.Logger,
	}
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
