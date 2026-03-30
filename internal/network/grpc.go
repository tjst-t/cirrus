package network

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// GRPCStateServer implements the NetworkStateService gRPC server.
type GRPCStateServer struct {
	pb.UnimplementedNetworkStateServiceServer
	stateCtrl         *StateController
	logger            *slog.Logger
	registrationToken string
	pollInterval      time.Duration

	mu       sync.RWMutex
	watchers map[string]*hostWatcher
}

type hostWatcher struct {
	hostID uuid.UUID
	cancel func()
}

// NewGRPCStateServer creates a new NetworkStateService gRPC server.
func NewGRPCStateServer(stateCtrl *StateController, logger *slog.Logger, registrationToken string) *GRPCStateServer {
	return &GRPCStateServer{
		stateCtrl:         stateCtrl,
		logger:            logger,
		registrationToken: registrationToken,
		pollInterval:      2 * time.Second,
		watchers:          make(map[string]*hostWatcher),
	}
}

// WatchHostNetworkState implements the server streaming RPC.
func (s *GRPCStateServer) WatchHostNetworkState(req *pb.WatchHostNetworkStateRequest, stream pb.NetworkStateService_WatchHostNetworkStateServer) error {
	// Authenticate
	if s.registrationToken != "" {
		if subtle.ConstantTimeCompare([]byte(req.RegistrationToken), []byte(s.registrationToken)) != 1 {
			return fmt.Errorf("invalid registration token")
		}
	}

	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		return fmt.Errorf("invalid host_id: %w", err)
	}

	ctx := stream.Context()
	s.logger.Info("network state watcher connected", "host_id", hostID)

	// Register watcher
	s.mu.Lock()
	if existing, ok := s.watchers[hostID.String()]; ok {
		existing.cancel()
	}
	s.watchers[hostID.String()] = &hostWatcher{hostID: hostID}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.watchers, hostID.String())
		s.mu.Unlock()
		s.logger.Info("network state watcher disconnected", "host_id", hostID)
	}()

	// Send initial full snapshot
	var version uint64 = 1
	state, err := s.stateCtrl.ComputeHostNetworkState(ctx, hostID)
	if err != nil {
		return fmt.Errorf("compute initial state: %w", err)
	}

	if err := stream.Send(&pb.HostNetworkStateUpdate{
		Full:    true,
		State:   state,
		Version: version,
	}); err != nil {
		return err
	}
	s.logger.Debug("sent initial state", "host_id", hostID,
		"ports", len(state.Ports),
		"policies", len(state.Policies),
		"remote_ports", len(state.RemotePorts),
		"dns_records", len(state.DnsRecords),
	)

	lastState := state

	// Poll loop for changes
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			newState, err := s.stateCtrl.ComputeHostNetworkState(ctx, hostID)
			if err != nil {
				s.logger.Warn("compute state failed", "host_id", hostID, "error", err)
				continue
			}

			if stateEqual(lastState, newState) {
				continue
			}

			version++
			// For simplicity, send full state on every change.
			// Delta computation can be optimized later.
			if err := stream.Send(&pb.HostNetworkStateUpdate{
				Full:    true,
				State:   newState,
				Version: version,
			}); err != nil {
				return err
			}
			s.logger.Debug("sent state update", "host_id", hostID, "version", version)
			lastState = newState
		}
	}
}

// stateEqual does a simple comparison of HostNetworkState.
// For the initial implementation, compare counts and key fields.
func stateEqual(a, b *pb.HostNetworkState) bool {
	if len(a.Ports) != len(b.Ports) || len(a.Policies) != len(b.Policies) ||
		len(a.RemotePorts) != len(b.RemotePorts) || len(a.DnsRecords) != len(b.DnsRecords) {
		return false
	}

	// Compare port IDs and states
	aPortMap := make(map[string]string, len(a.Ports))
	for _, p := range a.Ports {
		aPortMap[p.PortId] = p.IpAddress + p.GroupId
	}
	for _, p := range b.Ports {
		if aPortMap[p.PortId] != p.IpAddress+p.GroupId {
			return false
		}
	}

	// Compare policy IDs
	aPolicyMap := make(map[string]bool, len(a.Policies))
	for _, p := range a.Policies {
		aPolicyMap[p.PolicyId] = true
	}
	for _, p := range b.Policies {
		if !aPolicyMap[p.PolicyId] {
			return false
		}
	}

	// Compare remote port IPs
	aRemoteMap := make(map[string]bool, len(a.RemotePorts))
	for _, rp := range a.RemotePorts {
		aRemoteMap[rp.IpAddress] = true
	}
	for _, rp := range b.RemotePorts {
		if !aRemoteMap[rp.IpAddress] {
			return false
		}
	}

	return true
}
