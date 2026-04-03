package reconcile

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// vmRow holds stable-state VM data read from the DB for reconciliation.
type vmRow struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	Status   string
}

// HeartbeatReconciler compares the VM list from a worker heartbeat against the
// DB desired state and fires DriftEvents for any detected deviations.
//
// Called synchronously within the Heartbeat gRPC handler so that each heartbeat
// triggers a reconciliation pass for that host.
type HeartbeatReconciler struct {
	pool    *pgxpool.Pool
	handler *DriftHandler
	logger  *slog.Logger
}

// NewHeartbeatReconciler creates a new HeartbeatReconciler.
func NewHeartbeatReconciler(pool *pgxpool.Pool, handler *DriftHandler, logger *slog.Logger) *HeartbeatReconciler {
	return &HeartbeatReconciler{
		pool:    pool,
		handler: handler,
		logger:  logger.With("component", "heartbeat-reconciler"),
	}
}

// Reconcile compares the VMs reported in a heartbeat against the DB for hostID.
//
// Three cases are detected:
//  1. DB有・heartbeat無 (expected_missing): VM is stable in DB but absent from
//     the heartbeat → auto-heal to error.
//  2. DB無・heartbeat有 (unexpected_present): VM appears in heartbeat but has no
//     corresponding stable record in DB → alert.
//  3. ステータス不一致 (state_mismatch): VM exists in both but states differ:
//     - DB=running, heartbeat=crashed  → auto-heal to error
//     - DB=running, heartbeat=shutoff  → alert (may be intentional external stop)
func (r *HeartbeatReconciler) Reconcile(ctx context.Context, hostID string, vms []*pb.VMInfo) {
	hID, err := uuid.Parse(hostID)
	if err != nil {
		r.logger.Warn("heartbeat reconciler: invalid host_id", "host_id", hostID)
		return
	}

	// Build index of VMs reported in heartbeat: vm_id → status.
	heartbeatVMs := make(map[string]string, len(vms))
	for _, v := range vms {
		heartbeatVMs[v.VmId] = v.Status
	}

	// Query DB: VMs assigned to this host in stable states.
	// Transitional states (pending, building, deleting) are excluded.
	dbVMs, err := r.listStableVMs(ctx, hID)
	if err != nil {
		r.logger.Error("heartbeat reconciler: list stable VMs failed",
			"host_id", hostID, "error", err)
		return
	}

	// Build index of DB VMs: vm_id → status.
	dbVMMap := make(map[string]vmRow, len(dbVMs))
	for _, v := range dbVMs {
		dbVMMap[v.ID.String()] = v
	}

	// Case 1: DB有・heartbeat無 → expected_missing
	for id, dbVM := range dbVMMap {
		if _, inHeartbeat := heartbeatVMs[id]; !inHeartbeat {
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerCompute,
				Type:       DriftTypeExpectedMissing,
				Severity:   DriftSeverityCritical,
				Resource:   "vm",
				ResourceID: id,
				TenantID:   dbVM.TenantID.String(),
				HostID:     hostID,
				Expected:   dbVM.Status,
				Actual:     "absent",
				DetectedBy: "heartbeat_reconciler",
			})
		}
	}

	// Case 2 & 3: heartbeat内のVMをDBと照合
	for id, hbStatus := range heartbeatVMs {
		dbVM, inDB := dbVMMap[id]
		if !inDB {
			// Case 2: DB無・heartbeat有 → unexpected_present
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerCompute,
				Type:       DriftTypeUnexpectedPresent,
				Severity:   DriftSeverityHigh,
				Resource:   "vm",
				ResourceID: id,
				HostID:     hostID,
				Expected:   "absent",
				Actual:     hbStatus,
				DetectedBy: "heartbeat_reconciler",
			})
			continue
		}

		// Case 3: ステータス不一致
		dbStatus := dbVM.Status
		if !statusesMatch(dbStatus, hbStatus) {
			severity := classifyMismatch(dbStatus, hbStatus)
			// DriftHandler.healCompute inspects event.Actual to decide
			// whether to auto-heal (crashed → heal, shutoff → alert only).
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerCompute,
				Type:       DriftTypeStateMismatch,
				Severity:   severity,
				Resource:   "vm",
				ResourceID: id,
				TenantID:   dbVM.TenantID.String(),
				HostID:     hostID,
				Expected:   dbStatus,
				Actual:     hbStatus,
				DetectedBy: "heartbeat_reconciler",
			})
		}
	}
}

// listStableVMs returns VMs assigned to hostID that are in stable states.
func (r *HeartbeatReconciler) listStableVMs(ctx context.Context, hostID uuid.UUID) ([]vmRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, status FROM vms
		 WHERE host_id = $1
		   AND status NOT IN ('pending', 'building', 'deleting')`,
		hostID,
	)
	if err != nil {
		return nil, fmt.Errorf("heartbeat reconciler: query vms: %w", err)
	}
	defer rows.Close()

	var result []vmRow
	for rows.Next() {
		var v vmRow
		if err := rows.Scan(&v.ID, &v.TenantID, &v.Status); err != nil {
			return nil, fmt.Errorf("heartbeat reconciler: scan vm: %w", err)
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// statusesMatch returns true when the DB status and heartbeat (libvirt) status
// represent the same expected condition.
//
// Mapping:
//
//	DB "running" ↔ libvirt "running"
//	DB "stopped" ↔ libvirt "shutoff"
//	DB "error"   ↔ libvirt "crashed" (already in error, no further action)
func statusesMatch(dbStatus, hbStatus string) bool {
	switch dbStatus {
	case "running":
		return hbStatus == "running"
	case "stopped":
		return hbStatus == "shutoff"
	case "error":
		return hbStatus == "crashed" || hbStatus == "shutoff"
	}
	return false
}

// classifyMismatch returns the drift severity for a status mismatch.
// Auto-heal decisions are made in DriftHandler.healCompute based on event.Actual,
// not here, so this function only concerns itself with severity.
//
//	DB=running, libvirt=crashed  → critical
//	DB=running, libvirt=shutoff  → medium (possible external stop, alert only)
//	DB=stopped, libvirt=running  → high (unexpected start, alert only)
//	other                        → medium
func classifyMismatch(dbStatus, hbStatus string) string {
	if dbStatus == "running" && hbStatus == "crashed" {
		return DriftSeverityCritical
	}
	if dbStatus == "stopped" && hbStatus == "running" {
		return DriftSeverityHigh
	}
	return DriftSeverityMedium
}
