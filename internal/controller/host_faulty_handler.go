package controller

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HostFaultyHandler applies cascade updates when a host transitions to faulty:
// all VMs on the host are set to error, and their associated ports are set to down.
type HostFaultyHandler struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewHostFaultyHandler creates a new HostFaultyHandler.
func NewHostFaultyHandler(pool *pgxpool.Pool, logger *slog.Logger) *HostFaultyHandler {
	return &HostFaultyHandler{pool: pool, logger: logger}
}

// Handle performs the cascade update for the given host.
// Ports are updated first while VMs are still in their original state, then VMs
// are set to error. Reversing this order would cause the port subquery to find
// zero rows because all VMs would already be in 'error'.
func (h *HostFaultyHandler) Handle(ctx context.Context, hostID uuid.UUID) {
	// Set ports associated with non-terminal VMs to down — must run before VM update.
	portTag, err := h.pool.Exec(ctx,
		`UPDATE ports SET status = 'down'
		 WHERE vm_id IN (
		     SELECT id FROM vms WHERE host_id = $1 AND status NOT IN ('deleted', 'error')
		 ) AND status NOT IN ('down', 'deleting', 'error')`,
		hostID)
	if err != nil {
		h.logger.Error("host faulty handler: update ports", "host_id", hostID, "error", err)
		return
	}

	// Set all non-terminal VMs on the host to error.
	vmTag, err := h.pool.Exec(ctx,
		`UPDATE vms SET status = 'error', updated_at = now()
		 WHERE host_id = $1 AND status NOT IN ('deleted', 'error')`,
		hostID)
	if err != nil {
		h.logger.Error("host faulty handler: update vms", "host_id", hostID, "error", err)
		return
	}

	h.logger.Warn("host faulty handler: cascade complete",
		"host_id", hostID,
		"vms_errored", vmTag.RowsAffected(),
		"ports_downed", portTag.RowsAffected(),
	)
}
