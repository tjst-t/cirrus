package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/tjst-t/cirrus/internal/storage"
)

// StorageReconciler periodically checks volumes in the DB against what
// storage backends report. Initial implementation is log-only.
// DriftEvent integration comes in Sprint 8.5b.
type StorageReconciler struct {
	storageSvc storage.Service
	logger     *slog.Logger
	interval   time.Duration
}

// NewStorageReconciler creates a new StorageReconciler.
func NewStorageReconciler(storageSvc storage.Service, logger *slog.Logger, interval time.Duration) *StorageReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &StorageReconciler{
		storageSvc: storageSvc,
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
			continue
		}

		// Skip volumes in transient states (creating, deleting).
		var stableVolumes []storage.Volume
		for _, v := range dbVolumes {
			switch v.State {
			case storage.VolumeStateCreating, storage.VolumeStateDeleting:
				// skip
			default:
				stableVolumes = append(stableVolumes, v)
			}
		}

		r.logger.Debug("storage reconciler: backend checked",
			"backend_id", b.ID,
			"backend_name", b.Name,
			"db_volumes", len(stableVolumes),
		)
		// TODO Sprint 8.5b: compare with driver.ListVolumes() and emit DriftEvents
	}
}
