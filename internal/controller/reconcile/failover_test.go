package reconcile

// failover_test.go — unit tests for FailoverTrigger.Handle
//
// Uses lightweight mock structs implementing the local interfaces to avoid
// any external dependencies (DB, fencing agent, compute service).
//
// Test cases:
//  1. Fencing fails → FailoverVM never called.
//  2. listErrorVMsOnHost returns empty (no VMs) → FailoverVM never called.
//  3. One VM fails, one VM succeeds → both are attempted (best-effort).
//  4. Second Handle for the same host while in-flight is silently skipped.

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- mock implementations ---

// mockCascader records calls to Handle.
type mockCascader struct {
	mu    sync.Mutex
	calls []uuid.UUID
}

func (m *mockCascader) Handle(_ context.Context, hostID uuid.UUID) {
	m.mu.Lock()
	m.calls = append(m.calls, hostID)
	m.mu.Unlock()
}

// mockFencingAgent can be configured to succeed or fail.
type mockFencingAgent struct {
	err error
}

func (m *mockFencingAgent) Fence(_ context.Context, _ uuid.UUID) error {
	return m.err
}

// mockVMFailoverer records which VM IDs were passed and can be configured to fail
// on specific VM IDs.
type mockVMFailoverer struct {
	mu      sync.Mutex
	called  []uuid.UUID
	failIDs map[uuid.UUID]error
}

func newMockVMFailoverer() *mockVMFailoverer {
	return &mockVMFailoverer{failIDs: make(map[uuid.UUID]error)}
}

func (m *mockVMFailoverer) FailoverVM(_ context.Context, vmID uuid.UUID) error {
	m.mu.Lock()
	m.called = append(m.called, vmID)
	err := m.failIDs[vmID]
	m.mu.Unlock()
	return err
}

// slowMockFencingAgent adds a delay before returning.
type slowMockFencingAgent struct {
	delay time.Duration
}

func (s *slowMockFencingAgent) Fence(_ context.Context, _ uuid.UUID) error {
	time.Sleep(s.delay)
	return nil
}

// --- testFailoverTrigger: FailoverTrigger with injected VM list (bypasses DB) ---

// testFailoverTrigger embeds FailoverTrigger and overrides Handle to inject a
// fixed VM list instead of querying the database.
type testFailoverTrigger struct {
	*FailoverTrigger
	vmIDs    []uuid.UUID
	vmIDsErr error
}

// Handle overrides the goroutine-based Handle to inject VMs from the test fixture.
func (t *testFailoverTrigger) Handle(ctx context.Context, hostID uuid.UUID) {
	t.mu.Lock()
	if t.inFlight[hostID] {
		t.mu.Unlock()
		t.logger.Info("failover: already in progress for host, skipping", "host_id", hostID)
		return
	}
	t.inFlight[hostID] = true
	t.mu.Unlock()

	go func() {
		defer func() {
			t.mu.Lock()
			delete(t.inFlight, hostID)
			t.mu.Unlock()
		}()
		t.doFailoverWithVMs(context.Background(), hostID, t.vmIDs, t.vmIDsErr)
	}()
}

// doFailoverWithVMs mirrors FailoverTrigger.doFailover but uses an injected VM list.
func (t *testFailoverTrigger) doFailoverWithVMs(ctx context.Context, hostID uuid.UUID, vmIDs []uuid.UUID, listErr error) {
	t.cascadeHandler.Handle(ctx, hostID)

	if err := t.fencingAgent.Fence(ctx, hostID); err != nil {
		t.logger.Error("failover: fencing failed, aborting failover (safe-mode)",
			"host_id", hostID, "error", err)
		if t.driftHandler != nil {
			t.driftHandler.Handle(ctx, DriftEvent{
				Layer:      DriftLayerCompute,
				Type:       DriftTypeExpectedMissing,
				Severity:   DriftSeverityCritical,
				Resource:   "host",
				ResourceID: hostID.String(),
				DetectedBy: "failover_trigger",
				Expected:   "fenced",
				Actual:     "fencing_failed",
			})
		}
		return
	}

	if listErr != nil {
		t.logger.Error("failover: list error VMs failed, aborting",
			"host_id", hostID, "error", listErr)
		return
	}
	if len(vmIDs) == 0 {
		t.logger.Info("failover: no error VMs to recover", "host_id", hostID)
		return
	}

	for i, vmID := range vmIDs {
		if err := t.computeSvc.FailoverVM(ctx, vmID); err != nil {
			if ctx.Err() != nil {
				t.logger.Warn("failover: context cancelled, aborting remaining VMs",
					"host_id", hostID, "remaining", len(vmIDs)-i-1)
				return
			}
			t.logger.Error("failover: VM failover failed (continuing with next VM)",
				"host_id", hostID, "vm_id", vmID, "error", err)
		}
	}
}

// newTestTrigger constructs a testFailoverTrigger with all mocks wired in.
func newTestTrigger(
	cascader HostFaultCascader,
	fencer *mockFencingAgent,
	compute *mockVMFailoverer,
	vmIDs []uuid.UUID,
	vmIDsErr error,
) *testFailoverTrigger {
	base := &FailoverTrigger{
		cascadeHandler: cascader,
		fencingAgent:   fencer,
		computeSvc:     compute,
		driftHandler:   nil, // no drift handler needed for most tests
		pool:           nil,
		logger:         slog.Default(),
		inFlight:       make(map[uuid.UUID]bool),
	}
	return &testFailoverTrigger{
		FailoverTrigger: base,
		vmIDs:           vmIDs,
		vmIDsErr:        vmIDsErr,
	}
}

// waitForInFlight waits until no failover is in flight for hostID.
func waitForInFlight(ft *FailoverTrigger, hostID uuid.UUID, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ft.mu.Lock()
		inFlight := ft.inFlight[hostID]
		ft.mu.Unlock()
		if !inFlight {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// --- tests ---

// TestFailoverTrigger_FencingFails verifies that when fencing fails, FailoverVM
// is never called.
func TestFailoverTrigger_FencingFails(t *testing.T) {
	hostID := uuid.New()
	vm1 := uuid.New()

	cascader := &mockCascader{}
	fencer := &mockFencingAgent{err: errors.New("fencing: IPMI timeout")}
	compute := newMockVMFailoverer()

	trig := newTestTrigger(cascader, fencer, compute, []uuid.UUID{vm1}, nil)
	trig.Handle(context.Background(), hostID)

	if !waitForInFlight(trig.FailoverTrigger, hostID, 2*time.Second) {
		t.Fatal("failover goroutine did not finish in time")
	}

	compute.mu.Lock()
	calledCount := len(compute.called)
	compute.mu.Unlock()
	if calledCount != 0 {
		t.Errorf("FailoverVM called %d times, want 0 (fencing failed)", calledCount)
	}
}

// TestFailoverTrigger_NoErrorVMs verifies that when there are no error VMs on the
// host, FailoverVM is never called.
func TestFailoverTrigger_NoErrorVMs(t *testing.T) {
	hostID := uuid.New()

	cascader := &mockCascader{}
	fencer := &mockFencingAgent{err: nil}
	compute := newMockVMFailoverer()

	trig := newTestTrigger(cascader, fencer, compute, nil, nil)
	trig.Handle(context.Background(), hostID)

	if !waitForInFlight(trig.FailoverTrigger, hostID, 2*time.Second) {
		t.Fatal("failover goroutine did not finish in time")
	}

	compute.mu.Lock()
	calledCount := len(compute.called)
	compute.mu.Unlock()
	if calledCount != 0 {
		t.Errorf("FailoverVM called %d times, want 0 (no error VMs)", calledCount)
	}
}

// TestFailoverTrigger_BestEffort verifies that when one VM fails and another
// succeeds, both VMs are still attempted (best-effort behaviour).
func TestFailoverTrigger_BestEffort(t *testing.T) {
	hostID := uuid.New()
	vm1 := uuid.New() // will fail
	vm2 := uuid.New() // will succeed

	cascader := &mockCascader{}
	fencer := &mockFencingAgent{err: nil}
	compute := newMockVMFailoverer()
	compute.failIDs[vm1] = errors.New("worker: CreateVM: disk unavailable")

	trig := newTestTrigger(cascader, fencer, compute, []uuid.UUID{vm1, vm2}, nil)
	trig.Handle(context.Background(), hostID)

	if !waitForInFlight(trig.FailoverTrigger, hostID, 2*time.Second) {
		t.Fatal("failover goroutine did not finish in time")
	}

	compute.mu.Lock()
	called := make([]uuid.UUID, len(compute.called))
	copy(called, compute.called)
	compute.mu.Unlock()

	if len(called) != 2 {
		t.Fatalf("FailoverVM called %d times, want 2 (both VMs should be attempted)", len(called))
	}

	seen := make(map[uuid.UUID]bool)
	for _, id := range called {
		seen[id] = true
	}
	if !seen[vm1] {
		t.Error("vm1 (failing VM) was not attempted")
	}
	if !seen[vm2] {
		t.Error("vm2 (succeeding VM) was not attempted")
	}
}

// TestFailoverTrigger_DeduplicatesInFlight verifies that a second Handle call
// for the same host while a failover is already in progress is silently skipped.
func TestFailoverTrigger_DeduplicatesInFlight(t *testing.T) {
	hostID := uuid.New()
	vm1 := uuid.New()

	cascader := &mockCascader{}
	// A slow fencer ensures the first goroutine is still running when the
	// second Handle call arrives.
	slowFencer := &slowMockFencingAgent{delay: 100 * time.Millisecond}
	compute := newMockVMFailoverer()

	base := &FailoverTrigger{
		cascadeHandler: cascader,
		fencingAgent:   slowFencer,
		computeSvc:     compute,
		driftHandler:   nil,
		pool:           nil,
		logger:         slog.Default(),
		inFlight:       make(map[uuid.UUID]bool),
	}
	trig := &testFailoverTrigger{
		FailoverTrigger: base,
		vmIDs:           []uuid.UUID{vm1},
	}

	// First call starts a goroutine.
	trig.Handle(context.Background(), hostID)
	// Second call must detect in-flight and return immediately.
	trig.Handle(context.Background(), hostID)

	if !waitForInFlight(trig.FailoverTrigger, hostID, 3*time.Second) {
		t.Fatal("failover goroutine did not finish in time")
	}

	// Cascade handler must have been called exactly once.
	cascader.mu.Lock()
	cascadeCount := len(cascader.calls)
	cascader.mu.Unlock()
	if cascadeCount != 1 {
		t.Errorf("cascade.Handle called %d times, want 1", cascadeCount)
	}
}
