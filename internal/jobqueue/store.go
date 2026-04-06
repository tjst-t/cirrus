package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrJobNotRunning is returned by Complete and Fail when the job is not in running state.
var ErrJobNotRunning = errors.New("jobqueue: job is not in running state")

// Store is the PostgreSQL implementation of Queue.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Enqueue inserts a new pending job and returns the created record.
func (s *Store) Enqueue(ctx context.Context, p EnqueueParams) (*Job, error) {
	const q = `
		INSERT INTO jobs (type, status, payload, tenant_id, created_by, created_at, updated_at)
		VALUES ($1, 'pending', $2, $3, $4, now(), now())
		RETURNING id, type, status, payload, tenant_id, created_by,
		          created_at, updated_at, started_at, completed_at, error`

	var payload []byte
	if p.Payload != nil {
		payload = p.Payload
	}

	row := s.pool.QueryRow(ctx, q, p.Type, payload, p.TenantID, p.CreatedBy)
	job, err := scanJob(row)
	if err != nil {
		return nil, fmt.Errorf("jobqueue: enqueue: %w", err)
	}
	return job, nil
}

// Dequeue atomically claims the next pending job matching one of jobTypes.
// Returns nil, nil when no job is available.
func (s *Store) Dequeue(ctx context.Context, jobTypes []string) (*Job, error) {
	const q = `
		WITH candidate AS (
			SELECT id FROM jobs
			WHERE status = 'pending' AND type = ANY($1)
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE jobs
		SET status = 'running', started_at = now(), updated_at = now()
		FROM candidate
		WHERE jobs.id = candidate.id
		RETURNING jobs.id, jobs.type, jobs.status, jobs.payload, jobs.tenant_id, jobs.created_by,
		          jobs.created_at, jobs.updated_at, jobs.started_at, jobs.completed_at, jobs.error`

	row := s.pool.QueryRow(ctx, q, jobTypes)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("jobqueue: dequeue: %w", err)
	}
	return job, nil
}

// Complete marks a job as completed. Returns ErrJobNotRunning if the job is not
// currently in running state (prevents double-complete).
func (s *Store) Complete(ctx context.Context, id uuid.UUID) error {
	const q = `
		UPDATE jobs
		SET    status = 'completed', completed_at = now(), updated_at = now()
		WHERE  id = $1
		  AND  status = 'running'`

	ct, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("jobqueue: complete: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("jobqueue: complete %s: %w", id, ErrJobNotRunning)
	}
	return nil
}

// Fail marks a job as failed and records an error message. Returns ErrJobNotRunning
// if the job is not currently in running state (prevents double-fail).
func (s *Store) Fail(ctx context.Context, id uuid.UUID, errMsg string) error {
	const q = `
		UPDATE jobs
		SET    status = 'failed', error = $2, completed_at = now(), updated_at = now()
		WHERE  id = $1
		  AND  status = 'running'`

	ct, err := s.pool.Exec(ctx, q, id, errMsg)
	if err != nil {
		return fmt.Errorf("jobqueue: fail: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("jobqueue: fail %s: %w", id, ErrJobNotRunning)
	}
	return nil
}

// ListStuck returns jobs that have been in running state for longer than stuckAfter.
func (s *Store) ListStuck(ctx context.Context, stuckAfter time.Duration) ([]Job, error) {
	const q = `
		SELECT id, type, status, payload, tenant_id, created_by,
		       created_at, updated_at, started_at, completed_at, error
		FROM   jobs
		WHERE  status = 'running'
		  AND  started_at < now() - make_interval(secs => $1::float8)
		ORDER  BY started_at`

	rows, err := s.pool.Query(ctx, q, stuckAfter.Seconds())
	if err != nil {
		return nil, fmt.Errorf("jobqueue: list_stuck: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("jobqueue: list_stuck: %w", err)
		}
		jobs = append(jobs, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("jobqueue: list_stuck: %w", err)
	}
	return jobs, nil
}

// Get retrieves a single job by ID.
func (s *Store) Get(ctx context.Context, id uuid.UUID) (*Job, error) {
	const q = `
		SELECT id, type, status, payload, tenant_id, created_by,
		       created_at, updated_at, started_at, completed_at, error
		FROM   jobs
		WHERE  id = $1`

	row := s.pool.QueryRow(ctx, q, id)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("jobqueue: get: job %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("jobqueue: get: %w", err)
	}
	return job, nil
}

// scanner is a common interface satisfied by both pgx.Row and pgx.Rows.
type scanner interface{ Scan(dest ...any) error }

// scanJob scans a row (pgx.Row or pgx.Rows) into a Job.
func scanJob(row scanner) (*Job, error) {
	var j Job
	var payload []byte
	if err := row.Scan(
		&j.ID, &j.Type, &j.Status, &payload, &j.TenantID, &j.CreatedBy,
		&j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.CompletedAt, &j.Error,
	); err != nil {
		return nil, err
	}
	if payload != nil {
		j.Payload = json.RawMessage(payload)
	}
	return &j, nil
}
