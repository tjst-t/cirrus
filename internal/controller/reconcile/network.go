package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/network"
	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// NetworkReconciler periodically checks the desired HostNetworkState for
// each active host and logs any inconsistencies. This is the initial
// implementation (log-only); DriftEvent infrastructure comes in Sprint 8.5.
type NetworkReconciler struct {
	stateCtrl *network.StateController
	hostSvc   host.Service
	logger    *slog.Logger
	interval  time.Duration
}

// NewNetworkReconciler creates a new NetworkReconciler.
func NewNetworkReconciler(stateCtrl *network.StateController, hostSvc host.Service, logger *slog.Logger, interval time.Duration) *NetworkReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &NetworkReconciler{
		stateCtrl: stateCtrl,
		hostSvc:   hostSvc,
		logger:    logger.With("component", "network-reconciler"),
		interval:  interval,
	}
}

// Run starts the reconcile loop. Blocks until ctx is cancelled.
func (r *NetworkReconciler) Run(ctx context.Context) error {
	r.logger.Info("network reconciler started", "interval", r.interval)

	// Run immediately on startup
	r.reconcileOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("network reconciler stopped")
			return nil
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

// reconcileOnce runs a single reconciliation pass across all active hosts.
func (r *NetworkReconciler) reconcileOnce(ctx context.Context) {
	hosts, err := r.hostSvc.ListHostsByState(ctx, host.StateActive)
	if err != nil {
		r.logger.Error("reconcile: failed to list active hosts", "error", err)
		return
	}

	if len(hosts) == 0 {
		r.logger.Debug("reconcile: no active hosts")
		return
	}

	var totalPorts, totalPolicies, totalRemote int
	var warnings int

	for _, h := range hosts {
		state, err := r.stateCtrl.ComputeHostNetworkState(ctx, h.ID)
		if err != nil {
			r.logger.Warn("reconcile: compute state failed", "host_id", h.ID, "host_name", h.Name, "error", err)
			warnings++
			continue
		}

		portCount := len(state.Ports)
		policyCount := len(state.Policies)
		remoteCount := len(state.RemotePorts)
		dnsCount := len(state.DnsRecords)
		totalPorts += portCount
		totalPolicies += policyCount
		totalRemote += remoteCount

		// Consistency checks
		hostWarnings := r.checkConsistency(state, h.Name)
		warnings += hostWarnings

		r.logger.Debug("reconcile: host state",
			"host_id", h.ID,
			"host_name", h.Name,
			"ports", portCount,
			"policies", policyCount,
			"remote_ports", remoteCount,
			"dns_records", dnsCount,
			"warnings", hostWarnings,
		)
	}

	level := slog.LevelInfo
	if warnings > 0 {
		level = slog.LevelWarn
	}
	r.logger.Log(ctx, level, "reconcile: pass complete",
		"hosts", len(hosts),
		"total_ports", totalPorts,
		"total_policies", totalPolicies,
		"total_remote_ports", totalRemote,
		"warnings", warnings,
	)
}

// checkConsistency validates internal consistency of a host's desired state.
func (r *NetworkReconciler) checkConsistency(state *pb.HostNetworkState, hostName string) int {
	warnings := 0

	// Check: ports should have non-empty group_id
	for _, p := range state.Ports {
		if p.GroupId == "" {
			r.logger.Warn("reconcile: port has no group",
				"host", hostName,
				"port_id", p.PortId,
				"ip", p.IpAddress,
			)
			warnings++
		}
	}

	// Check: policies should reference valid group IDs
	groupIDs := make(map[string]bool)
	for _, p := range state.Ports {
		groupIDs[p.GroupId] = true
	}
	for _, rp := range state.RemotePorts {
		groupIDs[rp.GroupId] = true
	}
	for _, pol := range state.Policies {
		if !groupIDs[pol.SrcGroupId] && !groupIDs[pol.DstGroupId] {
			r.logger.Warn("reconcile: policy references unknown groups",
				"host", hostName,
				"policy_id", pol.PolicyId,
				"src_group", pol.SrcGroupId,
				"dst_group", pol.DstGroupId,
			)
			warnings++
		}
	}

	// Check: DNS records should reference ports in this host's network
	portNetworks := make(map[string]bool)
	for _, p := range state.Ports {
		portNetworks[p.NetworkId] = true
	}
	for _, rp := range state.RemotePorts {
		portNetworks[rp.NetworkId] = true
	}
	for _, dns := range state.DnsRecords {
		if !portNetworks[dns.NetworkId] {
			r.logger.Warn("reconcile: DNS record for unknown network",
				"host", hostName,
				"dns_name", dns.Name,
				"network_id", dns.NetworkId,
			)
			warnings++
		}
	}

	return warnings
}
