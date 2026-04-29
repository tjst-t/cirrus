// Package drs provides the DRS (Distributed Resource Scheduler) controller runner.
// It owns the periodic evaluation loop and bridges the DRS engine with the
// compute orchestrator to execute the planned VM migrations.
package drs

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/scheduler"
)

// Migrator is the interface satisfied by *compute.Orchestrator.
// Using a narrow interface keeps the drs package free of compute imports.
type Migrator interface {
	// MigrateVM live-migrates vmID to targetHostID (nil = scheduler chooses).
	MigrateVM(ctx context.Context, tenantID, vmID uuid.UUID, targetHostID *uuid.UUID) error
}

// RunReport summarises the outcome of a single DRS evaluation cycle.
type RunReport struct {
	StartedAt  time.Time
	FinishedAt time.Time
	AZResults  []scheduler.DRSResult
	Successes  int
	Failures   int
	Errors     []string
}

// Runner owns the DRS periodic loop.
type Runner struct {
	engine   scheduler.Engine
	migrator Migrator
	policy   scheduler.DRSPolicy
	interval time.Duration
	logger   *slog.Logger

	// inFlight is 1 when a RunOnce execution is in progress.
	inFlight atomic.Int32

	// mu guards lastReport.
	mu         sync.Mutex
	lastReport *RunReport
}

// NewRunner creates a DRS Runner.
func NewRunner(
	engine scheduler.Engine,
	migrator Migrator,
	policy scheduler.DRSPolicy,
	interval time.Duration,
	logger *slog.Logger,
) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		engine:   engine,
		migrator: migrator,
		policy:   policy,
		interval: interval,
		logger:   logger,
	}
}

// Start spawns a background goroutine that ticks every r.interval.
// If the previous tick has not yet finished, the new tick is skipped.
//
// TODO(S031): wrap with leader-only execution once controller HA is implemented.
// See docs/controller-ha.md — DRS must run on the leader instance only.
func (r *Runner) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !r.inFlight.CompareAndSwap(0, 1) {
					r.logger.Warn("DRS: skipping tick — previous run still in progress")
					continue
				}
				go func() {
					defer r.inFlight.Store(0)
					report, err := r.RunOnce(ctx)
					if err != nil {
						r.logger.Error("DRS RunOnce error", "error", err)
						return
					}
					r.logger.Info("DRS cycle complete",
						"successes", report.Successes,
						"failures", report.Failures,
						"az_results", len(report.AZResults),
					)
				}()
			}
		}
	}()
}

// RunOnce executes one DRS evaluation + migration cycle.
// Failures from individual migrations are logged at warn level and do not abort
// the cycle.
func (r *Runner) RunOnce(ctx context.Context) (*RunReport, error) {
	report := &RunReport{StartedAt: time.Now()}

	plans, err := r.engine.Plan(ctx, r.policy)
	if err != nil {
		return nil, err
	}
	report.AZResults = plans

	for _, azResult := range plans {
		for _, plan := range azResult.PlannedMoves {
			destID := plan.DestHostID
			if err := r.migrator.MigrateVM(ctx, plan.TenantID, plan.VMID, &destID); err != nil {
				report.Failures++
				report.Errors = append(report.Errors, err.Error())
				r.logger.Warn("DRS migration failed",
					"vm_id", plan.VMID,
					"src_host", plan.SrcHostID,
					"dest_host", plan.DestHostID,
					"error", err,
				)
				continue
			}
			report.Successes++
			r.logger.Info("DRS migration succeeded",
				"vm_id", plan.VMID,
				"src_host", plan.SrcHostID,
				"dest_host", plan.DestHostID,
			)
		}
	}

	report.FinishedAt = time.Now()
	r.mu.Lock()
	r.lastReport = report
	r.mu.Unlock()
	return report, nil
}

// LastReport returns the most recently completed RunReport, or nil if no cycle
// has completed yet.  The returned pointer is safe for concurrent reads.
func (r *Runner) LastReport() *RunReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastReport
}
