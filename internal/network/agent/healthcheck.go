package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/tjst-t/cirrus/internal/network"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
	networkpb "github.com/tjst-t/cirrus/proto/networkpb"
)

// HealthChecker probes backend VMs for l4_lb ingress rules and reports
// results to the controller via the ReportBackendHealth gRPC RPC.
type HealthChecker struct {
	hostID    string
	regToken  string
	cache     *StateCache
	client    pb.ControllerServiceClient
	logger    *slog.Logger
	prevState map[string]bool // key: "ingressID/vmID" → last reported healthy
}

// NewHealthChecker creates a HealthChecker.
func NewHealthChecker(hostID, regToken string, cache *StateCache, client pb.ControllerServiceClient, logger *slog.Logger) *HealthChecker {
	return &HealthChecker{
		hostID:    hostID,
		regToken:  regToken,
		cache:     cache,
		client:    client,
		logger:    logger,
		prevState: make(map[string]bool),
	}
}

// Run starts the health check loop. It probes backends for each l4_lb ingress
// in the current state and reports results to the controller.
// Blocks until ctx is cancelled.
func (hc *HealthChecker) Run(ctx context.Context) {
	hc.logger.Info("health checker started", "host_id", hc.hostID)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			hc.logger.Info("health checker stopped", "host_id", hc.hostID)
			return
		case <-ticker.C:
			hc.runOnce(ctx)
		}
	}
}

// runOnce performs one round of health checks and reports changed results.
func (hc *HealthChecker) runOnce(ctx context.Context) {
	state := hc.cache.Snapshot()
	if state == nil {
		return
	}

	// Build a set of VM IDs that have local ports on this host.
	localVMs := make(map[string]bool, len(state.Ports))
	for _, p := range state.Ports {
		localVMs[p.VmId] = true
	}

	var statuses []*pb.BackendHealthStatus
	activeKeys := make(map[string]bool)

	for _, rule := range state.IngressRules {
		if rule.Type != network.IngressTypeL4LB {
			continue
		}

		for _, backend := range rule.Backends {
			if backend.VmId == "" {
				continue
			}
			// Only probe backends whose VM resides on this host.
			if !localVMs[backend.VmId] {
				continue
			}

			key := rule.IngressId + "/" + backend.VmId
			activeKeys[key] = true

			healthy, msg := hc.probeBackend(ctx, rule, backend)
			hc.logger.Debug("health check result",
				"ingress_id", rule.IngressId,
				"vm_id", backend.VmId,
				"healthy", healthy,
				"message", msg,
			)

			// Only report when health status has changed to avoid redundant DB writes.
			if prev, seen := hc.prevState[key]; seen && prev == healthy {
				continue
			}
			hc.prevState[key] = healthy

			statuses = append(statuses, &pb.BackendHealthStatus{
				IngressId: rule.IngressId,
				VmId:      backend.VmId,
				Healthy:   healthy,
				Message:   msg,
			})
		}
	}

	// Prune prevState entries for backends no longer in the state.
	for key := range hc.prevState {
		if !activeKeys[key] {
			delete(hc.prevState, key)
		}
	}


	if len(statuses) == 0 {
		return
	}

	resp, err := hc.client.ReportBackendHealth(ctx, &pb.ReportBackendHealthRequest{
		HostId:            hc.hostID,
		RegistrationToken: hc.regToken,
		Statuses:          statuses,
	})
	if err != nil {
		hc.logger.Warn("failed to report backend health", "error", err)
		return
	}
	if !resp.Accepted {
		hc.logger.Warn("controller rejected backend health report")
	}
}

// probeBackend performs a TCP health check for a single backend.
// Returns (healthy, diagnostic_message).
func (hc *HealthChecker) probeBackend(ctx context.Context, _ *networkpb.IngressRule, backend *networkpb.L4LBBackend) (bool, string) {
	port := int(backend.Port)
	if port == 0 {
		return false, "backend port is 0"
	}
	return probeTCP(ctx, backend.Ip, port)
}

// probeTCP attempts a TCP connection to ip:port. Returns (healthy, message).
func probeTCP(ctx context.Context, ip string, port int) (bool, string) {
	addr := fmt.Sprintf("%s:%d", ip, port)
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return false, fmt.Sprintf("tcp probe failed: %v", err)
	}
	conn.Close()
	return true, "ok"
}
