package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/tjst-t/cirrus/internal/storage"
)

// StorageReconciler periodically checks volumes in the DB against storage
// backend metadata and fires DriftEvents for anomalies.
//
// Current capability: backend reachability and DB consistency checks.
// Volume-level comparison against actual backend state (Driver.ListVolumes)
// is deferred until the storage Driver interface exposes that method.
type StorageReconciler struct {
	storageSvc storage.Service
	handler    *DriftHandler
	logger     *slog.Logger
	interval   time.Duration
}

// NewStorageReconciler creates a new StorageReconciler.
func NewStorageReconciler(storageSvc storage.Service, handler *DriftHandler, logger *slog.Logger, interval time.Duration) *StorageReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &StorageReconciler{
		storageSvc: storageSvc,
		handler:    handler,
		logger:     logger.With("component", "storage-reconciler"),
		interval:   interval,
	}
}

// Run starts the reconcile loop. Blocks until ctx is cancelled.
func (r *StorageReconciler) Run(ctx context.Context) error {
	r.logger.Info("storage reconciler started", "interval", r.interval)

	r.reconcile(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("storage reconciler stopped")
			return nil
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *StorageReconciler) reconcile(ctx context.Context) {
	backends, err := r.storageSvc.ListBackends(ctx)
	if err != nil {
		r.logger.Error("storage reconciler: list backends failed", "error", err)
		r.handler.Handle(ctx, DriftEvent{
			Layer:      DriftLayerStorage,
			Type:       DriftTypeExpectedMissing,
			Severity:   DriftSeverityCritical,
			Resource:   "backend",
			ResourceID: "all",
			Expected:   "backends reachable",
			Actual:     "list backends failed: " + err.Error(),
			DetectedBy: "storage_reconciler",
		})
		return
	}

	for _, b := range backends {
		if b.State != storage.BackendStateActive {
			continue
		}
		dbVolumes, err := r.storageSvc.ListVolumesOnBackend(ctx, b.ID)
		if err != nil {
			r.logger.Error("storage reconciler: list volumes failed",
				"backend_id", b.ID, "error", err)
			r.handler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerStorage,
				Type:       DriftTypeExpectedMissing,
				Severity:   DriftSeverityHigh,
				Resource:   "backend",
				ResourceID: b.ID.String(),
				Expected:   "backend reachable",
				Actual:     "list volumes failed: " + err.Error(),
				DetectedBy: "storage_reconciler",
			})
			continue
		}

		// Skip volumes in transient states (creating, deleting).
		var stableCount int
		for _, v := range dbVolumes {
			switch v.State {
			case storage.VolumeStateCreating, storage.VolumeStateDeleting:
				// transient, skip
			default:
				stableCount++
			}
		}

		r.logger.Debug("storage reconciler: backend checked",
			"backend_id", b.ID,
			"backend_name", b.Name,
			"db_volumes", stableCount,
		)
		// TODO: compare stableCount with Driver.ListVolumes() once that method
		// is added to the storage.Driver interface, and emit expected_missing /
		// unexpected_present DriftEvents per volume.
	}
}
