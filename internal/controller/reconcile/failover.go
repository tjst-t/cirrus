package reconcile

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/controller/fencing"
)

// vmStatusError is the VM status value stored in the DB when a VM has failed.
// Mirrors compute.VMStatusError; defined here to avoid an import cycle.
const vmStatusError = "error"

// failoverConcurrency is the maximum number of VMs to failover in parallel per host.
const failoverConcurrency = 4

// DriftEventSink accepts drift events for logging or alerting.
// Implemented by DriftHandler; defined as an interface so FailoverTrigger
// is not bound to the concrete type in tests.
type DriftEventSink interface {
	Handle(ctx context.Context, event DriftEvent)
}

// HostFaultCascader is the subset of controller.FaultyHandler used by FailoverTrigger.
// Defined here to avoid an import cycle: controller imports controller/reconcile,
// so controller/reconcile must not import controller.
type HostFaultCascader interface {
	Handle(ctx context.Context, hostID uuid.UUID)
}

// VMFailoverer is the minimal interface required by FailoverTrigger.
// Implemented by compute.Orchestrator; defined here to avoid an import cycle
// (compute imports controller, controller/reconcile must not import compute).
type VMFailoverer interface {
	FailoverVM(ctx context.Context, vmID uuid.UUID) error
}

// FailoverTrigger implements controller.FaultyHandler.
// When a host transitions to faulty it:
//  1. Cascades the failure (VMs→error, ports→down) via the wrapped cascadeHandler.
//  2. Fences the host via the FencingAgent to ensure it is powered off.
//  3. If fencing fails: fires a critical Alert and aborts (safe-mode).
//  4. If fencing succeeds: calls computeSvc.FailoverVM for each error VM on the host.
//
// Handle is non-blocking: it launches a goroutine for each host failover and
// tracks in-flight operations with a per-host mutex to prevent duplicate failovers.
type FailoverTrigger struct {
	cascadeHandler HostFaultCascader
	fencingAgent   fencing.FencingAgent
	computeSvc     VMFailoverer
	driftHandler   DriftEventSink
	pool           *pgxpool.Pool
	logger         *slog.Logger

	mu       sync.Mutex
	inFlight map[uuid.UUID]bool
}

// NewFailoverTrigger creates a FailoverTrigger with all required dependencies.
func NewFailoverTrigger(
	cascade HostFaultCascader,
	fencingAgent fencing.FencingAgent,
	computeSvc VMFailoverer,
	driftHandler DriftEventSink,
	pool *pgxpool.Pool,
	logger *slog.Logger,
) *FailoverTrigger {
	return &FailoverTrigger{
		cascadeHandler: cascade,
		fencingAgent:   fencingAgent,
		computeSvc:     computeSvc,
		driftHandler:   driftHandler,
		pool:           pool,
		logger:         logger,
		inFlight:       make(map[uuid.UUID]bool),
	}
}

// Handle is called when a host transitions to faulty.
// It is non-blocking: if a failover for the given host is already in progress,
// it logs and returns immediately. Otherwise it launches a goroutine to perform
// the failover using context.Background() so it is not cancelled when the
// monitor ticks again.
func (ft *FailoverTrigger) Handle(ctx context.Context, hostID uuid.UUID) {
	ft.mu.Lock()
	if ft.inFlight[hostID] {
		ft.mu.Unlock()
		ft.logger.Info("failover: already in progress for host, skipping", "host_id", hostID)
		return
	}
	ft.inFlight[hostID] = true
	ft.mu.Unlock()

	go func() {
		defer func() {
			ft.mu.Lock()
			delete(ft.inFlight, hostID)
			ft.mu.Unlock()
		}()
		ft.doFailover(context.Background(), hostID)
	}()
}

// doFailover performs the actual failover sequence for a host.
// It is run inside a goroutine by Handle.
func (ft *FailoverTrigger) doFailover(ctx context.Context, hostID uuid.UUID) {
	ft.cascadeHandler.Handle(ctx, hostID)

	if err := ft.fencingAgent.Fence(ctx, hostID); err != nil {
		ft.logger.Error("failover: fencing failed, aborting failover (safe-mode)",
			"host_id", hostID, "error", err)
		if ft.driftHandler != nil {
			ft.driftHandler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerCompute,
				Type:       DriftTypeExpectedMissing,
				Severity:   DriftSeverityCritical,
				Resource:   "host",
				ResourceID: hostID.String(),
				DetectedBy: "failover_trigger",
				Expected:   "fenced",
				Actual:     "fencing_failed",
			})
		}
		return
	}
	ft.logger.Info("failover: host fenced successfully", "host_id", hostID)

	vmIDs, err := ft.listErrorVMsOnHost(ctx, hostID)
	if err != nil {
		ft.logger.Error("failover: list error VMs failed, aborting",
			"host_id", hostID, "error", err)
		return
	}
	if len(vmIDs) == 0 {
		ft.logger.Info("failover: no error VMs to recover", "host_id", hostID)
		return
	}
	ft.logger.Info("failover: starting VM failover", "host_id", hostID, "vm_count", len(vmIDs))

	// Best-effort: up to failoverConcurrency VMs in parallel; one failure does not abort others.
	sem := make(chan struct{}, failoverConcurrency)
	var wg sync.WaitGroup
	for _, vmID := range vmIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func(id uuid.UUID) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ft.computeSvc.FailoverVM(ctx, id); err != nil {
				ft.logger.Error("failover: VM failover failed",
					"host_id", hostID, "vm_id", id, "error", err)
			} else {
				ft.logger.Info("failover: VM failed over successfully",
					"host_id", hostID, "vm_id", id)
			}
		}(vmID)
	}
	wg.Wait()
}

// listErrorVMsOnHost returns the IDs of VMs in 'error' status assigned to the given host.
func (ft *FailoverTrigger) listErrorVMsOnHost(ctx context.Context, hostID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := ft.pool.Query(ctx,
		`SELECT id FROM vms WHERE host_id = $1 AND status = $2`, hostID, vmStatusError)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
