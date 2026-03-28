package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/internal/network/ovn"
)

// OVNReconciler periodically compares DB state with OVN NB state and logs drift.
type OVNReconciler struct {
	pool     *pgxpool.Pool
	ovn      ovn.Client
	logger   *slog.Logger
	interval time.Duration
}

// NewOVNReconciler creates a new OVN reconciler.
func NewOVNReconciler(pool *pgxpool.Pool, ovnClient ovn.Client, logger *slog.Logger, interval time.Duration) *OVNReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &OVNReconciler{
		pool:     pool,
		ovn:      ovnClient,
		logger:   logger.With("component", "ovn_reconciler"),
		interval: interval,
	}
}

// Run starts the reconcile loop. It blocks until ctx is cancelled.
func (r *OVNReconciler) Run(ctx context.Context) {
	r.logger.Info("OVN reconciler started", "interval", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("OVN reconciler stopped")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *OVNReconciler) reconcile(ctx context.Context) {
	r.logger.Debug("starting reconciliation cycle")

	r.reconcileLogicalSwitches(ctx)
	r.reconcileLogicalSwitchPorts(ctx)

	r.logger.Debug("reconciliation cycle complete")
}

func (r *OVNReconciler) reconcileLogicalSwitches(ctx context.Context) {
	// Get expected networks from DB (only active ones, skip transitional statuses)
	rows, err := r.pool.Query(ctx,
		`SELECT id::TEXT FROM networks WHERE status = 'active'`)
	if err != nil {
		r.logger.Error("reconcile: failed to query networks", "error", err)
		return
	}
	defer rows.Close()

	expected := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			r.logger.Error("reconcile: scan network", "error", err)
			return
		}
		expected[id] = true
	}
	if rows.Err() != nil {
		r.logger.Error("reconcile: iterate networks", "error", rows.Err())
		return
	}

	// Get actual logical switches from OVN
	actual, err := r.ovn.ListLogicalSwitches(ctx)
	if err != nil {
		r.logger.Error("reconcile: failed to list OVN logical switches", "error", err)
		return
	}

	actualSet := make(map[string]bool)
	for _, name := range actual {
		actualSet[name] = true
	}

	// expected_missing: in DB but not in OVN
	for id := range expected {
		if !actualSet[id] {
			r.logger.Warn("DRIFT: expected_missing — network in DB but Logical Switch missing in OVN",
				"type", "expected_missing",
				"severity", "critical",
				"resource", "logical_switch",
				"network_id", id,
			)
		}
	}

	// unexpected_present: in OVN but not in DB
	for _, name := range actual {
		if !expected[name] {
			r.logger.Warn("DRIFT: unexpected_present — Logical Switch in OVN but no corresponding network in DB",
				"type", "unexpected_present",
				"severity", "high",
				"resource", "logical_switch",
				"name", name,
			)
		}
	}
}

func (r *OVNReconciler) reconcileLogicalSwitchPorts(ctx context.Context) {
	// Get expected ports from DB (only non-transitional statuses)
	rows, err := r.pool.Query(ctx,
		`SELECT id::TEXT FROM ports WHERE status IN ('down', 'active')`)
	if err != nil {
		r.logger.Error("reconcile: failed to query ports", "error", err)
		return
	}
	defer rows.Close()

	expected := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			r.logger.Error("reconcile: scan port", "error", err)
			return
		}
		expected[id] = true
	}
	if rows.Err() != nil {
		r.logger.Error("reconcile: iterate ports", "error", rows.Err())
		return
	}

	// Get actual LSPs from OVN (all switches)
	actual, err := r.ovn.ListAllLogicalSwitchPorts(ctx)
	if err != nil {
		r.logger.Error("reconcile: failed to list OVN LSPs", "error", err)
		return
	}

	actualSet := make(map[string]bool)
	for _, name := range actual {
		actualSet[name] = true
	}

	for id := range expected {
		if !actualSet[id] {
			r.logger.Warn("DRIFT: expected_missing — port in DB but LSP missing in OVN",
				"type", "expected_missing",
				"severity", "critical",
				"resource", "logical_switch_port",
				"port_id", id,
			)
		}
	}

	for _, name := range actual {
		if !expected[name] {
			r.logger.Warn("DRIFT: unexpected_present — LSP in OVN but no corresponding port in DB",
				"type", "unexpected_present",
				"severity", "high",
				"resource", "logical_switch_port",
				"name", name,
			)
		}
	}
}
