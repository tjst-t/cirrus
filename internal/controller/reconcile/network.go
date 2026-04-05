package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/network"
	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// NetworkReconciler periodically checks the desired HostNetworkState for
// each active host and fires DriftEvents for any inconsistencies detected.
type NetworkReconciler struct {
	stateCtrl *network.StateController
	hostSvc   host.Service
	handler   *DriftHandler
	logger    *slog.Logger
	interval  time.Duration
	pool      *pgxpool.Pool
}

// NewNetworkReconciler creates a new NetworkReconciler.
func NewNetworkReconciler(stateCtrl *network.StateController, hostSvc host.Service, handler *DriftHandler, logger *slog.Logger, interval time.Duration) *NetworkReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &NetworkReconciler{
		stateCtrl: stateCtrl,
		hostSvc:   hostSvc,
		handler:   handler,
		logger:    logger.With("component", "network-reconciler"),
		interval:  interval,
	}
}

// WithPool sets the DB pool for egress/ingress reconciliation.
func (r *NetworkReconciler) WithPool(pool *pgxpool.Pool) *NetworkReconciler {
	r.pool = pool
	return r
}

// Run starts the reconcile loop. Blocks until ctx is cancelled.
func (r *NetworkReconciler) Run(ctx context.Context) error {
	r.logger.Info("network reconciler started", "interval", r.interval)

	// Run immediately on startup.
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
	var driftCount int

	for _, h := range hosts {
		state, err := r.stateCtrl.ComputeHostNetworkState(ctx, h.ID)
		if err != nil {
			r.logger.Warn("reconcile: compute state failed",
				"host_id", h.ID, "host_name", h.Name, "error", err)
			continue
		}

		portCount := len(state.Ports)
		policyCount := len(state.Policies)
		remoteCount := len(state.RemotePorts)
		totalPorts += portCount
		totalPolicies += policyCount
		totalRemote += remoteCount

		hostDrift := r.checkConsistency(ctx, state, h.ID.String(), h.Name)
		driftCount += hostDrift

		r.logger.Debug("reconcile: host state",
			"host_id", h.ID,
			"host_name", h.Name,
			"ports", portCount,
			"policies", policyCount,
			"remote_ports", remoteCount,
			"dns_records", len(state.DnsRecords),
			"drift_events", hostDrift,
		)
	}

	level := slog.LevelInfo
	if driftCount > 0 {
		level = slog.LevelWarn
	}
	r.logger.Log(ctx, level, "reconcile: pass complete",
		"hosts", len(hosts),
		"total_ports", totalPorts,
		"total_policies", totalPolicies,
		"total_remote_ports", totalRemote,
		"drift_events", driftCount,
	)

	// Also reconcile egress/ingress state for GW nodes.
	if err := r.reconcileEgressIngress(ctx); err != nil {
		r.logger.Error("reconcile: egress/ingress check failed", "error", err)
	}
}

// checkConsistency validates internal consistency of a host's desired state and
// fires DriftEvents for any issues found. Returns the number of drift events fired.
func (r *NetworkReconciler) checkConsistency(ctx context.Context, state *pb.HostNetworkState, hostID, hostName string) int {
	drift := 0

	// Check: ports should have non-empty group_id.
	for _, p := range state.Ports {
		if p.GroupId == "" {
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerNetwork,
				Type:       DriftTypeStateMismatch,
				Severity:   DriftSeverityHigh,
				Resource:   "port",
				ResourceID: p.PortId,
				HostID:     hostID,
				Expected:   "non-empty group_id",
				Actual:     "empty group_id",
				DetectedBy: "network_reconciler",
			})
			drift++
		}
	}

	// Build group ID sets for downstream checks.
	groupIDs := make(map[string]bool)
	for _, p := range state.Ports {
		groupIDs[p.GroupId] = true
	}
	for _, rp := range state.RemotePorts {
		groupIDs[rp.GroupId] = true
	}

	// Check: policies should reference at least one known group ID.
	for _, pol := range state.Policies {
		if !groupIDs[pol.SrcGroupId] && !groupIDs[pol.DstGroupId] {
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerNetwork,
				Type:       DriftTypeStateMismatch,
				Severity:   DriftSeverityHigh,
				Resource:   "policy_flow",
				ResourceID: pol.PolicyId,
				HostID:     hostID,
				Expected:   "valid group references",
				Actual:     "src=" + pol.SrcGroupId + " dst=" + pol.DstGroupId + " both unknown",
				DetectedBy: "network_reconciler",
			})
			drift++
		}
	}

	// Check: DNS records should reference networks that have ports on this host.
	portNetworks := make(map[string]bool)
	for _, p := range state.Ports {
		portNetworks[p.NetworkId] = true
	}
	for _, rp := range state.RemotePorts {
		portNetworks[rp.NetworkId] = true
	}
	for _, dns := range state.DnsRecords {
		if !portNetworks[dns.NetworkId] {
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerNetwork,
				Type:       DriftTypeStateMismatch,
				Severity:   DriftSeverityMedium,
				Resource:   "dns_record",
				ResourceID: dns.NetworkId,
				HostID:     hostID,
				Expected:   "network has local/remote ports",
				Actual:     "DNS record for network with no ports on this host",
				DetectedBy: "network_reconciler",
			})
			drift++
		}
	}

	return drift
}

// gwNetworkRow holds data for the egress/ingress reconcile check.
type gwNetworkRow struct {
	networkID     uuid.UUID
	lastHeartbeat *time.Time
}

// reconcileEgressIngress checks active GW node networks for stale heartbeats and
// fires DriftEvents when GW host state is uncertain.
func (r *NetworkReconciler) reconcileEgressIngress(ctx context.Context) error {
	if r.pool == nil {
		return nil
	}

	// Query networks that have a gateway_node_id AND have at least one egress or ingress.
	rows, err := r.pool.Query(ctx, `
		SELECT n.id, h.last_heartbeat
		FROM networks n
		JOIN gateway_nodes gn ON gn.id = n.gateway_node_id
		JOIN hosts h ON h.id = gn.host_id
		WHERE (
			EXISTS (SELECT 1 FROM egresses e WHERE e.network_id = n.id)
			OR EXISTS (SELECT 1 FROM ingresses i WHERE i.network_id = n.id)
		)
	`)
	if err != nil {
		return fmt.Errorf("reconcile egress/ingress: query: %w", err)
	}
	defer rows.Close()

	staleThreshold := time.Now().Add(-r.interval * 2)

	for rows.Next() {
		var row gwNetworkRow
		if err := rows.Scan(&row.networkID, &row.lastHeartbeat); err != nil {
			return fmt.Errorf("reconcile egress/ingress: scan: %w", err)
		}

		if row.lastHeartbeat == nil || row.lastHeartbeat.Before(staleThreshold) {
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerNetwork,
				Type:       DriftTypeStateMismatch,
				Severity:   DriftSeverityMedium,
				Resource:   DriftLayerNetwork,
				ResourceID: row.networkID.String(),
				Expected:   "GW node heartbeat fresh",
				Actual:     "GW node heartbeat stale; egress/ingress state uncertain",
				DetectedBy: "network_reconciler",
			})
		}
	}
	return rows.Err()
}
