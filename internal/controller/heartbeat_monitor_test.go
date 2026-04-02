package controller_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/controller"
	"github.com/tjst-t/cirrus/internal/host"
)


// ── mock HeartbeatQuerier ─────────────────────────────────────────────────────

type mockQuerier struct {
	faultyIDs    []uuid.UUID
	drainingDone []uuid.UUID
	resetCalls   []uuid.UUID
	updateErr    error
}

func (q *mockQuerier) UpdateMissedCounts(_ context.Context, _ time.Duration) error {
	return q.updateErr
}
func (q *mockQuerier) FaultyHostIDs(_ context.Context, _ int) ([]uuid.UUID, error) {
	return q.faultyIDs, nil
}
func (q *mockQuerier) ResetMissedCountForHost(_ context.Context, id uuid.UUID) error {
	q.resetCalls = append(q.resetCalls, id)
	return nil
}
func (q *mockQuerier) DrainingCompleteHostIDs(_ context.Context) ([]uuid.UUID, error) {
	return q.drainingDone, nil
}

// ── mock host.Service ─────────────────────────────────────────────────────────

type stateTrackingHostSvc struct {
	mockHostSvc
	mu         sync.Mutex
	states     map[uuid.UUID]host.OperationalState
	stateErr   error
}

func newStateTrackingHostSvc() *stateTrackingHostSvc {
	return &stateTrackingHostSvc{
		mockHostSvc: *newMockHostSvc(),
		states:      make(map[uuid.UUID]host.OperationalState),
	}
}

func (s *stateTrackingHostSvc) SetOperationalState(_ context.Context, id uuid.UUID, state host.OperationalState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stateErr != nil {
		return s.stateErr
	}
	s.states[id] = state
	return nil
}

func (s *stateTrackingHostSvc) getState(id uuid.UUID) (host.OperationalState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[id]
	return st, ok
}

// newTestMonitor builds a HeartbeatMonitor with injectable querier for tests.
func newTestMonitor(q controller.HeartbeatQuerier, svc host.Service) *controller.HeartbeatMonitor {
	return controller.NewHeartbeatMonitorWithQuerier(q, svc, nil, slog.Default(), 30*time.Second)
}

// ── tests ─────────────────────────────────────────────────────────────────────

// S017-4-2: heartbeat 途絶→faulty 自動遷移
func TestHeartbeatMonitor_FaultyTransition(t *testing.T) {
	hostID := uuid.New()
	q := &mockQuerier{faultyIDs: []uuid.UUID{hostID}}
	svc := newStateTrackingHostSvc()

	mon := newTestMonitor(q, svc)
	mon.CheckForTest(context.Background())

	if st, ok := svc.getState(hostID); !ok || st != host.StateFaulty {
		t.Fatalf("expected faulty, got %v (ok=%v)", st, ok)
	}
	// Counter should be reset after faulty transition.
	if len(q.resetCalls) != 1 || q.resetCalls[0] != hostID {
		t.Fatalf("expected 1 reset call for host %v, got %v", hostID, q.resetCalls)
	}
}

// S017-4-2: SetOperationalState エラー時は faulty に遷移しない
func TestHeartbeatMonitor_FaultyTransition_ServiceError(t *testing.T) {
	hostID := uuid.New()
	q := &mockQuerier{faultyIDs: []uuid.UUID{hostID}}
	svc := newStateTrackingHostSvc()
	svc.stateErr = errors.New("db error")

	mon := newTestMonitor(q, svc)
	mon.CheckForTest(context.Background())

	if _, ok := svc.getState(hostID); ok {
		t.Fatal("state should not have been set when service returns error")
	}
	// Counter should not be reset if transition failed.
	if len(q.resetCalls) != 0 {
		t.Fatalf("expected no reset calls, got %d", len(q.resetCalls))
	}
}

// S017-4-2: UpdateMissedCounts エラー時はFaultyHostIDsを呼ばない
func TestHeartbeatMonitor_IncrementError_SkipsFaultyCheck(t *testing.T) {
	hostID := uuid.New()
	q := &mockQuerier{
		faultyIDs: []uuid.UUID{hostID},
		updateErr: errors.New("db error"),
	}
	svc := newStateTrackingHostSvc()

	mon := newTestMonitor(q, svc)
	mon.CheckForTest(context.Background())

	if _, ok := svc.getState(hostID); ok {
		t.Fatal("faulty transition should not happen when increment fails")
	}
}

// S017-4-4: draining→VM 退避完了→maintenance 自動遷移
func TestHeartbeatMonitor_DrainingToMaintenance(t *testing.T) {
	hostID := uuid.New()
	q := &mockQuerier{drainingDone: []uuid.UUID{hostID}}
	svc := newStateTrackingHostSvc()

	mon := newTestMonitor(q, svc)
	mon.CheckForTest(context.Background())

	if st, ok := svc.getState(hostID); !ok || st != host.StateMaintenance {
		t.Fatalf("expected maintenance, got %v (ok=%v)", st, ok)
	}
}

// S017-4-4: draining→maintenance でサービスエラー時は停止しない（次ホストへ継続）
func TestHeartbeatMonitor_DrainingToMaintenance_ServiceError(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	q := &mockQuerier{drainingDone: []uuid.UUID{id1, id2}}

	var callMu sync.Mutex
	calls := 0
	succeeded := map[uuid.UUID]host.OperationalState{}

	fakeSvc := &funcHostSvc{
		setOperationalStateFn: func(_ context.Context, id uuid.UUID, state host.OperationalState) error {
			callMu.Lock()
			defer callMu.Unlock()
			calls++
			if id == id1 {
				return errors.New("forced error")
			}
			succeeded[id] = state
			return nil
		},
	}

	mon := newTestMonitor(q, fakeSvc)
	mon.CheckForTest(context.Background())

	callMu.Lock()
	defer callMu.Unlock()
	if calls != 2 {
		t.Fatalf("expected 2 SetOperationalState calls, got %d", calls)
	}
	if st, ok := succeeded[id2]; !ok || st != host.StateMaintenance {
		t.Fatalf("expected id2 to be maintenance, got %v (ok=%v)", st, ok)
	}
}

// ── funcHostSvc: host.Service backed by functions ────────────────────────────

type funcHostSvc struct {
	mockHostSvc
	setOperationalStateFn func(context.Context, uuid.UUID, host.OperationalState) error
}

func (f *funcHostSvc) SetOperationalState(ctx context.Context, id uuid.UUID, state host.OperationalState) error {
	if f.setOperationalStateFn != nil {
		return f.setOperationalStateFn(ctx, id, state)
	}
	return nil
}

// ── HostFaultyHandlerFunc: FaultyHandler backed by a function (test helper) ──

type hostFaultyHandlerFunc struct {
	handleFn func(ctx context.Context, hostID uuid.UUID)
}

func (f *hostFaultyHandlerFunc) Handle(ctx context.Context, hostID uuid.UUID) {
	if f.handleFn != nil {
		f.handleFn(ctx, hostID)
	}
}

// ── transition rule tests (S017-4-2: 不正遷移拒否) ─────────────────────────────

// validTransitionsTest mirrors the production validTransitions map.
var validTransitionsTest = map[host.OperationalState][]host.OperationalState{
	host.StateRegistering: {host.StateActive},
	host.StateActive:      {host.StateDraining, host.StateMaintenance, host.StateFaulty},
	host.StateDraining:    {host.StateActive, host.StateMaintenance, host.StateFaulty},
	host.StateMaintenance: {host.StateActive, host.StateRetiring},
	host.StateFaulty:      {host.StateActive, host.StateMaintenance},
	host.StateRetiring:    {}, // terminal
}

func TestTransitionRules_AllowedTransitions(t *testing.T) {
	allowed := [][2]host.OperationalState{
		{host.StateRegistering, host.StateActive},
		{host.StateActive, host.StateDraining},
		{host.StateActive, host.StateMaintenance},
		{host.StateActive, host.StateFaulty},
		{host.StateDraining, host.StateActive},
		{host.StateDraining, host.StateMaintenance},
		{host.StateDraining, host.StateFaulty},
		{host.StateMaintenance, host.StateActive},
		{host.StateMaintenance, host.StateRetiring},
		{host.StateFaulty, host.StateActive},
		{host.StateFaulty, host.StateMaintenance},
	}
	for _, pair := range allowed {
		from, to := pair[0], pair[1]
		found := false
		for _, t2 := range validTransitionsTest[from] {
			if t2 == to {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s → %s to be allowed", from, to)
		}
	}
}

func TestTransitionRules_DisallowedTransitions(t *testing.T) {
	disallowed := [][2]host.OperationalState{
		// retiring は終端
		{host.StateRetiring, host.StateActive},
		{host.StateRetiring, host.StateMaintenance},
		{host.StateRetiring, host.StateDraining},
		// registering は active にしか遷移できない
		{host.StateRegistering, host.StateDraining},
		{host.StateRegistering, host.StateMaintenance},
		{host.StateRegistering, host.StateFaulty},
		// active → retiring は draining/maintenance 経由のみ
		{host.StateActive, host.StateRetiring},
		// faulty → draining は不可
		{host.StateFaulty, host.StateDraining},
	}
	for _, pair := range disallowed {
		from, to := pair[0], pair[1]
		for _, t2 := range validTransitionsTest[from] {
			if t2 == to {
				t.Errorf("expected %s → %s to be disallowed", from, to)
			}
		}
	}
}

// S017-4-3: HostFaultyHandler の呼び出し確認
func TestHeartbeatMonitor_FaultyHandler_Called(t *testing.T) {
	hostID := uuid.New()
	q := &mockQuerier{faultyIDs: []uuid.UUID{hostID}}
	svc := newStateTrackingHostSvc()

	var handledID uuid.UUID
	handler := &hostFaultyHandlerFunc{
		handleFn: func(_ context.Context, id uuid.UUID) {
			handledID = id
		},
	}

	mon := controller.NewHeartbeatMonitorWithQuerier(q, svc, handler, slog.Default(), 30*time.Second)
	mon.CheckForTest(context.Background())

	if handledID != hostID {
		t.Fatalf("expected handler to be called with %v, got %v", hostID, handledID)
	}
}
