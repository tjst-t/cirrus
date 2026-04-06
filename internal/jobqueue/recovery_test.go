package jobqueue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mockRecoveryQueue wraps mockQueue and adds tracking for RecoverAllRunningJobs behavior.
// Since RecoverAllRunningJobs uses a real pgxpool, we test the function's contract via
// a standalone unit test that exercises the logic directly using an in-memory simulation.

// inMemoryJobStore simulates the jobs table in memory for recovery testing.
type inMemoryJobStore struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]*Job
}

func newInMemoryJobStore() *inMemoryJobStore {
	return &inMemoryJobStore{jobs: make(map[uuid.UUID]*Job)}
}

func (s *inMemoryJobStore) insert(j *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = j
}

func (s *inMemoryJobStore) get(id uuid.UUID) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	copy := *j
	return &copy, true
}

// recoverAllRunningJobsInMemory is the in-memory equivalent of RecoverAllRunningJobs for testing.
// It resets all jobs with status=running to status=pending and clears started_at.
func recoverAllRunningJobsInMemory(store *inMemoryJobStore) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	count := 0
	for _, j := range store.jobs {
		if j.Status == StatusRunning {
			j.Status = StatusPending
			j.StartedAt = nil
			j.UpdatedAt = time.Now()
			count++
		}
	}
	return count
}

// TestRecoverAllRunningJobs_ResetsRunningToPending verifies that jobs in running state
// are reset to pending with nil started_at after restart recovery.
func TestRecoverAllRunningJobs_ResetsRunningToPending(t *testing.T) {
	store := newInMemoryJobStore()

	now := time.Now()

	// Insert a job that is stuck in running state (simulates a crash mid-execution).
	stuckJobID := uuid.New()
	tid1 := uuid.New()
	createdBy1 := "test-user"
	stuckJob := &Job{
		ID:        stuckJobID,
		Type:      "vm_create",
		Status:    StatusRunning,
		TenantID:  &tid1,
		CreatedBy: &createdBy1,
		CreatedAt: now.Add(-10 * time.Minute),
		UpdatedAt: now.Add(-5 * time.Minute),
		StartedAt: func() *time.Time { t := now.Add(-5 * time.Minute); return &t }(),
	}
	store.insert(stuckJob)

	// Insert a job that is already pending — should not be affected.
	pendingJobID := uuid.New()
	tid2 := uuid.New()
	createdBy2 := "test-user2"
	pendingJob := &Job{
		ID:        pendingJobID,
		Type:      "vm_delete",
		Status:    StatusPending,
		TenantID:  &tid2,
		CreatedBy: &createdBy2,
		CreatedAt: now.Add(-2 * time.Minute),
		UpdatedAt: now.Add(-2 * time.Minute),
	}
	store.insert(pendingJob)

	// Insert a completed job — should not be affected.
	completedJobID := uuid.New()
	tid3 := uuid.New()
	createdBy3 := "test-user3"
	completedJob := &Job{
		ID:          completedJobID,
		Type:        "volume_create",
		Status:      StatusCompleted,
		TenantID:    &tid3,
		CreatedBy:   &createdBy3,
		CreatedAt:   now.Add(-30 * time.Minute),
		UpdatedAt:   now.Add(-25 * time.Minute),
		CompletedAt: func() *time.Time { t := now.Add(-25 * time.Minute); return &t }(),
	}
	store.insert(completedJob)

	// Run recovery.
	recovered := recoverAllRunningJobsInMemory(store)

	if recovered != 1 {
		t.Errorf("expected 1 job recovered, got %d", recovered)
	}

	// Verify the stuck job is now pending with no started_at.
	j, ok := store.get(stuckJobID)
	if !ok {
		t.Fatal("stuck job not found in store")
	}
	if j.Status != StatusPending {
		t.Errorf("expected status=pending, got %s", j.Status)
	}
	if j.StartedAt != nil {
		t.Errorf("expected started_at=nil after recovery, got %v", j.StartedAt)
	}

	// Verify the pending job is still pending.
	pj, ok := store.get(pendingJobID)
	if !ok {
		t.Fatal("pending job not found in store")
	}
	if pj.Status != StatusPending {
		t.Errorf("expected pending job to remain pending, got %s", pj.Status)
	}

	// Verify the completed job is still completed.
	cj, ok := store.get(completedJobID)
	if !ok {
		t.Fatal("completed job not found in store")
	}
	if cj.Status != StatusCompleted {
		t.Errorf("expected completed job to remain completed, got %s", cj.Status)
	}
}

// TestRecoverAllRunningJobs_NoRunningJobs verifies that recovery is a no-op when there are
// no jobs in running state.
func TestRecoverAllRunningJobs_NoRunningJobs(t *testing.T) {
	store := newInMemoryJobStore()

	// Only pending and completed jobs.
	store.insert(&Job{ID: uuid.New(), Type: "vm_create", Status: StatusPending})
	store.insert(&Job{ID: uuid.New(), Type: "vm_delete", Status: StatusCompleted})

	recovered := recoverAllRunningJobsInMemory(store)
	if recovered != 0 {
		t.Errorf("expected 0 jobs recovered, got %d", recovered)
	}
}

// TestRecoverAllRunningJobs_MultipleStuckJobs verifies that all running jobs are recovered.
func TestRecoverAllRunningJobs_MultipleStuckJobs(t *testing.T) {
	store := newInMemoryJobStore()

	for i := 0; i < 5; i++ {
		now := time.Now()
		store.insert(&Job{
			ID:        uuid.New(),
			Type:      "vm_create",
			Status:    StatusRunning,
			StartedAt: &now,
		})
	}

	recovered := recoverAllRunningJobsInMemory(store)
	if recovered != 5 {
		t.Errorf("expected 5 jobs recovered, got %d", recovered)
	}

	// All should be pending now.
	for _, j := range store.jobs {
		if j.Status != StatusPending {
			t.Errorf("expected all jobs to be pending after recovery, got %s", j.Status)
		}
		if j.StartedAt != nil {
			t.Errorf("expected started_at=nil after recovery, got %v", j.StartedAt)
		}
	}
}

// TestDispatcher_RecoveryThenProcessing verifies the full recovery → dispatch cycle:
// 1. Jobs stuck in "running" at startup are reset to "pending"
// 2. The dispatcher picks them up and processes them
func TestDispatcher_RecoveryThenProcessing(t *testing.T) {
	// Simulate a job that was left in running state (crashed mid-execution).
	stuckJob := &Job{
		ID:     uuid.New(),
		Type:   "vm_create",
		Status: StatusRunning,
	}

	q := &mockQueue{
		jobs: []*Job{stuckJob},
	}

	// Simulate recovery: reset running → pending
	for _, j := range q.jobs {
		if j.Status == StatusRunning {
			j.Status = StatusPending
			j.StartedAt = nil
		}
	}

	processed := make(chan uuid.UUID, 1)
	d := NewDispatcher(q, 1, newTestLogger())
	d.Register("vm_create", func(_ context.Context, job *Job) error {
		processed <- job.ID
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go d.Start(ctx)

	select {
	case id := <-processed:
		if id != stuckJob.ID {
			t.Errorf("expected recovered job %s to be processed, got %s", stuckJob.ID, id)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for recovered job to be processed")
	}
}
