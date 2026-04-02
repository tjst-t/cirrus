package controller

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/host"
)

// HeartbeatQuerier abstracts the DB queries needed by HeartbeatMonitor so the
// monitor can be unit-tested without a real database.
type HeartbeatQuerier interface {
	// UpdateMissedCounts atomically resets missed_heartbeat_count to 0 for hosts
	// that sent a heartbeat within the interval, and increments it for those that
	// did not. One round-trip replaces two.
	UpdateMissedCounts(ctx context.Context, interval time.Duration) error
	// FaultyHostIDs returns IDs of active/draining hosts whose missed count ≥ threshold.
	FaultyHostIDs(ctx context.Context, threshold int) ([]uuid.UUID, error)
	// ResetMissedCountForHost resets missed_heartbeat_count for a single host.
	ResetMissedCountForHost(ctx context.Context, hostID uuid.UUID) error
	// DrainingCompleteHostIDs returns IDs of draining hosts with no active VMs.
	DrainingCompleteHostIDs(ctx context.Context) ([]uuid.UUID, error)
}

// pgHeartbeatQuerier is the production implementation of HeartbeatQuerier.
type pgHeartbeatQuerier struct{ pool *pgxpool.Pool }

func (q *pgHeartbeatQuerier) UpdateMissedCounts(ctx context.Context, interval time.Duration) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE hosts SET
		   missed_heartbeat_count = CASE
		     WHEN last_heartbeat >= now() - $1::interval THEN 0
		     ELSE missed_heartbeat_count + 1
		   END,
		   updated_at = now()
		 WHERE operational_state IN ('active', 'draining')`,
		interval.String())
	return err
}

func (q *pgHeartbeatQuerier) FaultyHostIDs(ctx context.Context, threshold int) ([]uuid.UUID, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id FROM hosts
		 WHERE operational_state IN ('active', 'draining')
		   AND missed_heartbeat_count >= $1`,
		threshold)
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

func (q *pgHeartbeatQuerier) ResetMissedCountForHost(ctx context.Context, hostID uuid.UUID) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE hosts SET missed_heartbeat_count = 0, updated_at = now() WHERE id = $1`,
		hostID)
	return err
}

func (q *pgHeartbeatQuerier) DrainingCompleteHostIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id FROM hosts
		 WHERE operational_state = 'draining'
		   AND NOT EXISTS (
		       SELECT 1 FROM vms
		       WHERE vms.host_id = hosts.id
		         AND vms.status NOT IN ('deleted', 'error')
		   )`)
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

// FaultyHandler is invoked after a host transitions to faulty.
type FaultyHandler interface {
	Handle(ctx context.Context, hostID uuid.UUID)
}

// HeartbeatMonitor periodically checks host heartbeats and transitions hosts to
// faulty when they miss too many consecutive heartbeats. It also auto-transitions
// draining hosts to maintenance when their VM count reaches zero.
type HeartbeatMonitor struct {
	querier   HeartbeatQuerier
	hostSvc   host.Service
	handler   FaultyHandler
	interval  time.Duration
	threshold int
	logger    *slog.Logger
}

// NewHeartbeatMonitor creates a HeartbeatMonitor that ticks at the given interval
// and marks hosts faulty after 3 consecutive missed heartbeats.
func NewHeartbeatMonitor(pool *pgxpool.Pool, hostSvc host.Service, handler FaultyHandler, logger *slog.Logger, interval time.Duration) *HeartbeatMonitor {
	return &HeartbeatMonitor{
		querier:   &pgHeartbeatQuerier{pool: pool},
		hostSvc:   hostSvc,
		handler:   handler,
		interval:  interval,
		threshold: 3,
		logger:    logger,
	}
}

// NewHeartbeatMonitorWithQuerier creates a HeartbeatMonitor with an injectable
// querier and handler for testing.
func NewHeartbeatMonitorWithQuerier(q HeartbeatQuerier, hostSvc host.Service, handler FaultyHandler, logger *slog.Logger, interval time.Duration) *HeartbeatMonitor {
	return &HeartbeatMonitor{
		querier:   q,
		hostSvc:   hostSvc,
		handler:   handler,
		interval:  interval,
		threshold: 3,
		logger:    logger,
	}
}

// CheckForTest exposes the internal check cycle for unit testing.
func (m *HeartbeatMonitor) CheckForTest(ctx context.Context) {
	m.check(ctx)
}

// Run starts the monitor loop. It returns when ctx is cancelled.
func (m *HeartbeatMonitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

// check performs one monitoring cycle.
func (m *HeartbeatMonitor) check(ctx context.Context) {
	m.checkHeartbeats(ctx)
	m.checkDrainingCompletion(ctx)
}

// checkHeartbeats increments missed_heartbeat_count for hosts that haven't sent
// a heartbeat within the monitor interval, resets it for those that have, and
// transitions to faulty when the threshold is reached.
func (m *HeartbeatMonitor) checkHeartbeats(ctx context.Context) {
	if err := m.querier.UpdateMissedCounts(ctx, m.interval); err != nil {
		m.logger.Error("heartbeat monitor: update missed counts", "error", err)
		return
	}
	faultyIDs, err := m.querier.FaultyHostIDs(ctx, m.threshold)
	if err != nil {
		m.logger.Error("heartbeat monitor: query faulty candidates", "error", err)
		return
	}
	for _, id := range faultyIDs {
		m.transitionFaulty(ctx, id)
	}
}

// transitionFaulty transitions a host to faulty and resets its missed heartbeat
// counter, then invokes the cascade handler.
func (m *HeartbeatMonitor) transitionFaulty(ctx context.Context, hostID uuid.UUID) {
	if err := m.hostSvc.SetOperationalState(ctx, hostID, host.StateFaulty); err != nil {
		m.logger.Error("heartbeat monitor: set faulty", "host_id", hostID, "error", err)
		return
	}
	if err := m.querier.ResetMissedCountForHost(ctx, hostID); err != nil {
		m.logger.Warn("heartbeat monitor: reset counter after faulty", "host_id", hostID, "error", err)
	}
	m.logger.Warn("heartbeat monitor: host transitioned to faulty", "host_id", hostID)
	if m.handler != nil {
		m.handler.Handle(ctx, hostID)
	}
}

// checkDrainingCompletion auto-transitions draining hosts to maintenance when
// all their VMs are in deleted or error state.
func (m *HeartbeatMonitor) checkDrainingCompletion(ctx context.Context) {
	doneIDs, err := m.querier.DrainingCompleteHostIDs(ctx)
	if err != nil {
		m.logger.Error("heartbeat monitor: query draining completion", "error", err)
		return
	}
	for _, id := range doneIDs {
		if err := m.hostSvc.SetOperationalState(ctx, id, host.StateMaintenance); err != nil {
			m.logger.Error("heartbeat monitor: draining→maintenance", "host_id", id, "error", err)
			continue
		}
		m.logger.Info("heartbeat monitor: draining host transitioned to maintenance", "host_id", id)
	}
}
