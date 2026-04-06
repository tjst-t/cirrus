// Package jobqueue provides an async job queue backed by PostgreSQL.
package jobqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Status represents the lifecycle state of a job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Job is a unit of async work stored in the jobs table.
type Job struct {
	ID          uuid.UUID
	Type        string
	Status      Status
	Payload     json.RawMessage
	TenantID    *uuid.UUID
	CreatedBy   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       *string
}

// EnqueueParams holds the fields required to create a new job.
type EnqueueParams struct {
	Type      string
	Payload   json.RawMessage
	TenantID  *uuid.UUID
	CreatedBy *string
}

// Queue is the interface for enqueuing and managing jobs.
type Queue interface {
	// Enqueue inserts a new job with status=pending and returns it.
	Enqueue(ctx context.Context, p EnqueueParams) (*Job, error)

	// Dequeue atomically claims the next pending job matching one of jobTypes
	// using SELECT … FOR UPDATE SKIP LOCKED.
	// Returns nil, nil if no jobs are available.
	Dequeue(ctx context.Context, jobTypes []string) (*Job, error)

	// Complete marks a job as completed.
	Complete(ctx context.Context, id uuid.UUID) error

	// Fail marks a job as failed and records an error message.
	Fail(ctx context.Context, id uuid.UUID, errMsg string) error

	// ListStuck returns jobs that have been in running state for longer than stuckAfter.
	ListStuck(ctx context.Context, stuckAfter time.Duration) ([]Job, error)

	// Get retrieves a single job by ID.
	Get(ctx context.Context, id uuid.UUID) (*Job, error)
}
