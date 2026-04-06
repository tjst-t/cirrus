package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mockQueue is an in-memory Queue for testing the Dispatcher.
type mockQueue struct {
	mu       sync.Mutex
	jobs     []*Job
	dequeued []*Job
	failed   []failRecord
	completed []uuid.UUID
}

type failRecord struct {
	id  uuid.UUID
	msg string
}

func (m *mockQueue) Enqueue(_ context.Context, p EnqueueParams) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := &Job{
		ID:      uuid.New(),
		Type:    p.Type,
		Status:  StatusPending,
		Payload: p.Payload,
	}
	m.jobs = append(m.jobs, j)
	return j, nil
}

func (m *mockQueue) Dequeue(_ context.Context, jobTypes []string) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	typeSet := make(map[string]bool, len(jobTypes))
	for _, t := range jobTypes {
		typeSet[t] = true
	}
	for i, j := range m.jobs {
		if j.Status == StatusPending && typeSet[j.Type] {
			j.Status = StatusRunning
			m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
			m.dequeued = append(m.dequeued, j)
			return j, nil
		}
	}
	return nil, nil
}

func (m *mockQueue) Complete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completed = append(m.completed, id)
	return nil
}

func (m *mockQueue) Fail(_ context.Context, id uuid.UUID, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed = append(m.failed, failRecord{id, errMsg})
	return nil
}

func (m *mockQueue) ListStuck(_ context.Context, _ time.Duration) ([]Job, error) {
	return nil, nil
}

func (m *mockQueue) Get(_ context.Context, id uuid.UUID) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return nil, errors.New("not found")
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestDispatcher_Register_and_Start_processes_job(t *testing.T) {
	q := &mockQueue{}
	payload, _ := json.Marshal(map[string]string{"key": "value"})
	_, _ = q.Enqueue(context.Background(), EnqueueParams{Type: "test-job", Payload: payload})

	processed := make(chan uuid.UUID, 1)

	d := NewDispatcher(q, 1, newTestLogger())
	d.Register("test-job", func(ctx context.Context, job *Job) error {
		processed <- job.ID
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go d.Start(ctx)

	select {
	case id := <-processed:
		// Verify Complete was eventually called
		time.Sleep(50 * time.Millisecond)
		q.mu.Lock()
		defer q.mu.Unlock()
		found := false
		for _, cid := range q.completed {
			if cid == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected job %s to be marked completed", id)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for job to be processed")
	}
}

func TestDispatcher_handler_error_marks_failed(t *testing.T) {
	q := &mockQueue{}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{Type: "fail-job"})

	done := make(chan struct{})

	d := NewDispatcher(q, 1, newTestLogger())
	d.Register("fail-job", func(ctx context.Context, job *Job) error {
		close(done)
		return errors.New("handler failed")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go d.Start(ctx)

	select {
	case <-done:
		time.Sleep(50 * time.Millisecond)
		q.mu.Lock()
		defer q.mu.Unlock()
		if len(q.failed) == 0 {
			t.Fatal("expected job to be marked failed")
		}
		if q.failed[0].msg != "handler failed" {
			t.Errorf("unexpected error message: %s", q.failed[0].msg)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for handler to be called")
	}
}

func TestDispatcher_no_handler_marks_failed(t *testing.T) {
	q := &mockQueue{}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{Type: "unknown-job"})

	// Register a different type so workers start, but unknown-job has no handler.
	d := NewDispatcher(q, 1, newTestLogger())
	d.Register("unknown-job", nil) // explicitly nil handler

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go d.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		n := len(q.failed)
		q.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for job to be marked failed")
}

func TestDispatcher_graceful_shutdown(t *testing.T) {
	q := &mockQueue{} // empty queue

	d := NewDispatcher(q, 2, newTestLogger())
	d.Register("noop", func(_ context.Context, _ *Job) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// OK: dispatcher exited cleanly after ctx cancellation
	case <-time.After(3 * time.Second):
		t.Fatal("dispatcher did not shut down within 3 seconds")
	}
}
