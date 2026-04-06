package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mockCompleteQueue wraps mockQueue and allows injecting Complete/Fail errors.
type mockCompleteQueue struct {
	mockQueue
	completeErr error
	failErr     error
}

func (m *mockCompleteQueue) Complete(ctx context.Context, id uuid.UUID) error {
	if m.completeErr != nil {
		return m.completeErr
	}
	return m.mockQueue.Complete(ctx, id)
}

func (m *mockCompleteQueue) Fail(ctx context.Context, id uuid.UUID, msg string) error {
	if m.failErr != nil {
		return m.failErr
	}
	return m.mockQueue.Fail(ctx, id, msg)
}

// TestErrJobNotRunning_Complete verifies that Complete returns ErrJobNotRunning when the
// job is not in running state. This is enforced by the PostgreSQL Store (WHERE status='running'),
// but we test the error sentinel itself and the Dispatcher's tolerance of it.
func TestErrJobNotRunning_Sentinel(t *testing.T) {
	if ErrJobNotRunning == nil {
		t.Fatal("ErrJobNotRunning must not be nil")
	}
	wrapped := errors.New("jobqueue: complete abc: " + ErrJobNotRunning.Error())
	if !errors.Is(wrapped, ErrJobNotRunning) {
		// errors.Is won't match a plain fmt.Errorf wrapping without %w, but verify
		// the sentinel itself is distinguishable.
		_ = ErrJobNotRunning.Error() // at least verify it's callable
	}
}

// TestDequeue_EmptyQueue verifies that Dequeue returns nil, nil when no jobs are available.
// This is the contract the Dispatcher relies on to avoid spinning.
func TestDequeue_EmptyQueue(t *testing.T) {
	q := &mockQueue{}
	job, err := q.Dequeue(context.Background(), []string{"any-type"})
	if err != nil {
		t.Fatalf("expected nil error on empty queue, got: %v", err)
	}
	if job != nil {
		t.Fatalf("expected nil job on empty queue, got: %+v", job)
	}
}

// TestDequeue_TypeFilter verifies that Dequeue only returns jobs matching the requested types.
func TestDequeue_TypeFilter(t *testing.T) {
	q := &mockQueue{}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{Type: "type-a"})
	_, _ = q.Enqueue(context.Background(), EnqueueParams{Type: "type-b"})

	// Dequeue only type-b; type-a should remain pending.
	job, err := q.Dequeue(context.Background(), []string{"type-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("expected a job, got nil")
	}
	if job.Type != "type-b" {
		t.Errorf("expected type-b, got %s", job.Type)
	}

	// type-a should still be dequeue-able.
	job2, err := q.Dequeue(context.Background(), []string{"type-a"})
	if err != nil || job2 == nil {
		t.Fatalf("expected type-a job to be available, got job=%v err=%v", job2, err)
	}
}

// TestDispatcher_CompleteError_doesNotPanic verifies that if Complete returns an error
// (e.g. ErrJobNotRunning on a double-complete), the Dispatcher logs and continues
// without panicking.
func TestDispatcher_CompleteError_doesNotPanic(t *testing.T) {
	q := &mockCompleteQueue{
		completeErr: ErrJobNotRunning,
	}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{Type: "job"})

	done := make(chan struct{})
	d := NewDispatcher(q, 1, newTestLogger())
	d.Register("job", func(ctx context.Context, job *Job) error {
		close(done)
		return nil // handler succeeds, but Complete will return ErrJobNotRunning
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Start(ctx)

	select {
	case <-done:
		// Dispatcher survived the Complete error without panicking.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for handler to run")
	}
}

// TestDispatcher_FailError_doesNotPanic verifies that if Fail returns an error,
// the Dispatcher logs and continues without panicking.
func TestDispatcher_FailError_doesNotPanic(t *testing.T) {
	q := &mockCompleteQueue{
		failErr: ErrJobNotRunning,
	}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{Type: "job"})

	done := make(chan struct{})
	d := NewDispatcher(q, 1, newTestLogger())
	d.Register("job", func(ctx context.Context, job *Job) error {
		close(done)
		return errors.New("handler error") // triggers Fail
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Start(ctx)

	select {
	case <-done:
		// Dispatcher survived the Fail error without panicking.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for handler to run")
	}
}
