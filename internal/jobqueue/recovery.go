package jobqueue

import (
	"context"
	"fmt"
	"log/slog"
)

// RecoverAllRunningJobs resets ALL jobs that are currently in status='running' back
// to status='pending' and clears started_at. Intended for startup use only — it
// resets every running job unconditionally, not just long-stuck ones.
func (s *Store) RecoverAllRunningJobs(ctx context.Context, logger *slog.Logger) error {
	const q = `UPDATE jobs SET status = 'pending', started_at = NULL, updated_at = now() WHERE status = 'running'`

	ct, err := s.pool.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("jobqueue: recover_stuck_jobs: %w", err)
	}

	n := ct.RowsAffected()
	if n > 0 {
		logger.Warn("jobqueue: recovered running jobs at startup", "count", n)
	} else {
		logger.Info("jobqueue: no running jobs found at startup")
	}
	return nil
}
