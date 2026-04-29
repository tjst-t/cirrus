package drs_test

// runner_test.go — unit tests for the DRS controller runner.
//
// Uses lightweight fakes for the Engine and Migrator interfaces.
// No database, no external services required.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/controller/drs"
	"github.com/tjst-t/cirrus/internal/scheduler"
)

// --- fakes ---

// fakeEngine returns pre-canned DRSResults from Plan.
type fakeEngine struct {
	mu      sync.Mutex
	results []scheduler.DRSResult
	err     error
	called  int
}

func (f *fakeEngine) Plan(_ context.Context, _ scheduler.DRSPolicy) ([]scheduler.DRSResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called++
	return f.results, f.err
}

// fakeMigrator records MigrateVM calls and can be configured to fail.
type fakeMigrator struct {
	mu      sync.Mutex
	called  []uuid.UUID // vmIDs passed
	failIDs map[uuid.UUID]error
}

func newFakeMigrator() *fakeMigrator {
	return &fakeMigrator{failIDs: make(map[uuid.UUID]error)}
}

func (f *fakeMigrator) MigrateVM(_ context.Context, _, vmID uuid.UUID, _ *uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = append(f.called, vmID)
	return f.failIDs[vmID]
}

func (f *fakeMigrator) CalledVMIDs() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]uuid.UUID, len(f.called))
	copy(out, f.called)
	return out
}

// --- tests ---

func TestRunOnce_CallsMigratorForEachPlan(t *testing.T) {
	azID := uuid.New()
	vm1 := uuid.New()
	vm2 := uuid.New()
	tenant := uuid.New()
	src := uuid.New()
	dst := uuid.New()

	engine := &fakeEngine{
		results: []scheduler.DRSResult{
			{
				AZID: azID,
				PlannedMoves: []scheduler.MigrationPlan{
					{VMID: vm1, TenantID: tenant, SrcHostID: src, DestHostID: dst, AZID: azID},
					{VMID: vm2, TenantID: tenant, SrcHostID: src, DestHostID: dst, AZID: azID},
				},
			},
		},
	}
	migrator := newFakeMigrator()

	runner := drs.NewRunner(engine, migrator, scheduler.DRSPolicy{}, time.Minute, nil)
	report, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Successes != 2 {
		t.Errorf("expected 2 successes, got %d", report.Successes)
	}
	if report.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", report.Failures)
	}
	called := migrator.CalledVMIDs()
	if len(called) != 2 {
		t.Errorf("expected 2 MigrateVM calls, got %d", len(called))
	}
}

func TestRunOnce_MigratorFailureLogged_CycleNotAborted(t *testing.T) {
	azID := uuid.New()
	vm1 := uuid.New()
	vm2 := uuid.New()
	tenant := uuid.New()
	src := uuid.New()
	dst := uuid.New()

	engine := &fakeEngine{
		results: []scheduler.DRSResult{
			{
				AZID: azID,
				PlannedMoves: []scheduler.MigrationPlan{
					{VMID: vm1, TenantID: tenant, SrcHostID: src, DestHostID: dst, AZID: azID},
					{VMID: vm2, TenantID: tenant, SrcHostID: src, DestHostID: dst, AZID: azID},
				},
			},
		},
	}
	migrator := newFakeMigrator()
	migrator.failIDs[vm1] = errors.New("migration failed for vm1")

	runner := drs.NewRunner(engine, migrator, scheduler.DRSPolicy{}, time.Minute, nil)
	report, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error from RunOnce: %v", err)
	}
	// vm1 failed, vm2 should still succeed.
	if report.Successes != 1 {
		t.Errorf("expected 1 success, got %d", report.Successes)
	}
	if report.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", report.Failures)
	}
	if len(report.Errors) != 1 {
		t.Errorf("expected 1 error message, got %d", len(report.Errors))
	}
	// Both VMs were still attempted.
	called := migrator.CalledVMIDs()
	if len(called) != 2 {
		t.Errorf("expected 2 MigrateVM calls (failure does not abort), got %d", len(called))
	}
}

func TestStart_ConcurrentTicksDoNotOverlap(t *testing.T) {
	// Use a slow engine to ensure a tick is still in flight when the second fires.
	slow := &slowFakeEngine{delay: 50 * time.Millisecond}
	migrator := newFakeMigrator()

	// Use a very short interval so two ticks fire during the test.
	interval := 20 * time.Millisecond
	runner := drs.NewRunner(slow, migrator, scheduler.DRSPolicy{}, interval, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	runner.Start(ctx)
	<-ctx.Done()

	// The engine was slow (50ms) and interval was 20ms, so several ticks fired
	// while the first was in progress. Ensure at most one execution ran at a time
	// (the counter increments atomically inside slowFakeEngine).
	if slow.MaxConcurrent() > 1 {
		t.Errorf("concurrent ticks overlapped: max_concurrent=%d (want ≤1)", slow.MaxConcurrent())
	}
}

func TestLastReport_ReturnsNilBeforeFirstRun(t *testing.T) {
	engine := &fakeEngine{}
	runner := drs.NewRunner(engine, newFakeMigrator(), scheduler.DRSPolicy{}, time.Minute, nil)
	if runner.LastReport() != nil {
		t.Error("expected nil LastReport before any run")
	}
}

func TestLastReport_ReturnsMostRecentRun(t *testing.T) {
	azID := uuid.New()
	vm1 := uuid.New()
	tenant := uuid.New()
	src := uuid.New()
	dst := uuid.New()

	engine := &fakeEngine{
		results: []scheduler.DRSResult{
			{
				AZID: azID,
				PlannedMoves: []scheduler.MigrationPlan{
					{VMID: vm1, TenantID: tenant, SrcHostID: src, DestHostID: dst, AZID: azID},
				},
			},
		},
	}
	runner := drs.NewRunner(engine, newFakeMigrator(), scheduler.DRSPolicy{}, time.Minute, nil)

	_, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	report := runner.LastReport()
	if report == nil {
		t.Fatal("expected non-nil LastReport after RunOnce")
	}
	if report.Successes != 1 {
		t.Errorf("expected 1 success in last report, got %d", report.Successes)
	}
}

// slowFakeEngine simulates a slow Plan to test concurrency guards.
type slowFakeEngine struct {
	delay   time.Duration
	mu      sync.Mutex
	active  int
	maxSeen int
}

func (s *slowFakeEngine) Plan(ctx context.Context, _ scheduler.DRSPolicy) ([]scheduler.DRSResult, error) {
	s.mu.Lock()
	s.active++
	if s.active > s.maxSeen {
		s.maxSeen = s.active
	}
	s.mu.Unlock()

	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
	}

	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return nil, nil
}

func (s *slowFakeEngine) MaxConcurrent() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxSeen
}
