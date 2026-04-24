package compute

// orchestrator_test.go — MigrateVM 障害注入ユニットテスト
//
// Orchestrator は *pgxpool.Pool を直接使用するため、実際の MigrateVM を
// 呼び出すには DB 接続が必要となる。そこで quota_integrity_test.go と同じ
// アプローチで、MigrateVM が内部で実行するステップを直接シミュレーションし、
// エラー発生時のステータス遷移が正しく行われることを検証する。
//
// 検証観点:
//   - StartMigration 失敗 → VM が VMStatusError に遷移すること
//   - Reschedule 失敗（ErrNoSuitableHost）→ VM が VMStatusError に遷移すること

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/scheduler"
)

// --- fake vm state tracker ---

// fakeVMState simulates the VM status field that Orchestrator updates via setVMStatus.
type fakeVMState struct {
	status   VMStatus
	errMsg   string
	history  []VMStatus
}

func newFakeVMState(initial VMStatus) *fakeVMState {
	return &fakeVMState{
		status:  initial,
		history: []VMStatus{initial},
	}
}

// setStatus simulates Orchestrator.setVMStatus: updates status and records the transition.
func (f *fakeVMState) setStatus(status VMStatus, errMsg string) {
	f.status = status
	f.errMsg = errMsg
	f.history = append(f.history, status)
}

// --- fake worker call results ---

// fakeWorkerCall simulates a worker RPC call that can be configured to fail.
type fakeWorkerCall struct {
	err error
}

func (w *fakeWorkerCall) call() error {
	return w.err
}

// --- migration state machine simulation ---

// migrationStateMachine reproduces the MigrateVM error-handling logic extracted
// from orchestrator.go so it can be tested without a database connection.
//
// Flow:
//  1. Set status to "migrating"
//  2. Determine destination host (may call Reschedule if targetHostID is nil)
//  3. Call StartMigration on the source worker
//  4. On success: set status to "running" and update host_id
//  5. On any error (defer): set status to "error"
func runMigrationStateMachine(
	vm *fakeVMState,
	targetHostID *uuid.UUID,
	rescheduleResult *uuid.UUID,
	rescheduleErr error,
	startMigrationErr error,
) error {
	// Step 1: set migrating
	vm.setStatus(VMStatusMigrating, "")

	var retErr error
	defer func() {
		if retErr != nil {
			// Mirrors the defer in Orchestrator.MigrateVM
			vm.setStatus(VMStatusError, retErr.Error())
		}
	}()

	// Step 2: resolve destination host (mirrors steps 3 in MigrateVM)
	var destHostID uuid.UUID
	if targetHostID != nil {
		destHostID = *targetHostID
	} else {
		// Simulate Reschedule call
		if rescheduleErr != nil {
			retErr = errors.New("compute: MigrateVM: reschedule: " + rescheduleErr.Error())
			return retErr
		}
		if rescheduleResult != nil {
			destHostID = *rescheduleResult
		}
	}
	_ = destHostID

	// Step 3: simulate PrepareMigration (always succeeds in these tests)
	// Step 4: simulate StartMigration
	if startMigrationErr != nil {
		retErr = errors.New("compute: MigrateVM: StartMigration: " + startMigrationErr.Error())
		return retErr
	}

	// Step 5: success — update host and set running
	vm.setStatus(VMStatusRunning, "")
	return nil
}

// --- tests ---

// TestMigrateVM_StartMigrationFailure verifies that when StartMigration returns an error,
// the VM transitions from "migrating" → "error" (not "running").
func TestMigrateVM_StartMigrationFailure(t *testing.T) {
	ctx := context.Background()
	_ = ctx // used in real implementation; kept for readability

	vmID := uuid.New()
	tenantID := uuid.New()
	srcHostID := uuid.New()
	destHostID := uuid.New()

	_ = vmID
	_ = tenantID
	_ = srcHostID

	// Simulate VM initial state: running on srcHost
	vm := newFakeVMState(VMStatusRunning)

	// StartMigration returns a simulated gRPC error
	startMigErr := errors.New("rpc error: code = Internal desc = migration failed: disk I/O error")

	err := runMigrationStateMachine(vm, &destHostID, nil, nil, startMigErr)
	if err == nil {
		t.Fatal("expected MigrateVM to return an error when StartMigration fails, got nil")
	}

	// Verify status transitions: running(initial) → migrating → error
	wantHistory := []VMStatus{VMStatusRunning, VMStatusMigrating, VMStatusError}
	if len(vm.history) != len(wantHistory) {
		t.Fatalf("status history length = %d, want %d; history = %v", len(vm.history), len(wantHistory), vm.history)
	}
	for i, want := range wantHistory {
		if vm.history[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, vm.history[i], want)
		}
	}

	// Verify final status is error
	if vm.status != VMStatusError {
		t.Errorf("final VM status = %q, want %q", vm.status, VMStatusError)
	}

	// Verify error message is captured
	if vm.errMsg == "" {
		t.Error("expected error message to be set on VM, got empty string")
	}
}

// TestMigrateVM_RescheduleFailure verifies that when scheduler.Reschedule returns
// ErrNoSuitableHost, the VM transitions from "migrating" → "error".
func TestMigrateVM_RescheduleFailure(t *testing.T) {
	ctx := context.Background()
	_ = ctx // used in real implementation; kept for readability

	vmID := uuid.New()
	tenantID := uuid.New()

	_ = vmID
	_ = tenantID

	// Simulate VM initial state: running (no explicit targetHostID → scheduler must be used)
	vm := newFakeVMState(VMStatusRunning)

	// Scheduler returns ErrNoSuitableHost
	err := runMigrationStateMachine(vm, nil, nil, scheduler.ErrNoSuitableHost, nil)
	if err == nil {
		t.Fatal("expected MigrateVM to return an error when Reschedule fails, got nil")
	}

	// Verify the error chain contains ErrNoSuitableHost context
	if !errors.Is(err, scheduler.ErrNoSuitableHost) {
		// The state machine wraps the error message as a string, so check string containment.
		// This matches how MigrateVM wraps the error via fmt.Errorf.
		if !containsErrMessage(err, scheduler.ErrNoSuitableHost.Error()) {
			t.Errorf("expected error to reference ErrNoSuitableHost, got: %v", err)
		}
	}

	// Verify status transitions: running(initial) → migrating → error
	wantHistory := []VMStatus{VMStatusRunning, VMStatusMigrating, VMStatusError}
	if len(vm.history) != len(wantHistory) {
		t.Fatalf("status history length = %d, want %d; history = %v", len(vm.history), len(wantHistory), vm.history)
	}
	for i, want := range wantHistory {
		if vm.history[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, vm.history[i], want)
		}
	}

	// Verify final status is error
	if vm.status != VMStatusError {
		t.Errorf("final VM status = %q, want %q", vm.status, VMStatusError)
	}

	// Verify error message is populated
	if vm.errMsg == "" {
		t.Error("expected error message to be set on VM, got empty string")
	}
}

// TestMigrateVM_Success verifies the happy path: VM transitions
// running → migrating → running and final status is running.
func TestMigrateVM_Success(t *testing.T) {
	destHostID := uuid.New()

	vm := newFakeVMState(VMStatusRunning)

	err := runMigrationStateMachine(vm, &destHostID, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify status transitions: running → migrating → running
	wantHistory := []VMStatus{VMStatusRunning, VMStatusMigrating, VMStatusRunning}
	if len(vm.history) != len(wantHistory) {
		t.Fatalf("status history = %v, want %v", vm.history, wantHistory)
	}
	for i, want := range wantHistory {
		if vm.history[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, vm.history[i], want)
		}
	}

	if vm.status != VMStatusRunning {
		t.Errorf("final VM status = %q, want %q", vm.status, VMStatusRunning)
	}
}

// TestMigrateVM_RescheduleSuccess verifies that when no targetHostID is specified
// but Reschedule succeeds, the migration proceeds and VM ends up running.
func TestMigrateVM_RescheduleSuccess(t *testing.T) {
	pickedHostID := uuid.New()

	vm := newFakeVMState(VMStatusRunning)

	err := runMigrationStateMachine(vm, nil, &pickedHostID, nil, nil)
	if err != nil {
		t.Fatalf("expected no error when Reschedule succeeds, got: %v", err)
	}

	if vm.status != VMStatusRunning {
		t.Errorf("final VM status = %q, want %q", vm.status, VMStatusRunning)
	}
}

// --- FailoverVM state machine simulation ---

// failoverStateMachine reproduces the FailoverVM error-handling logic extracted
// from orchestrator.go so it can be tested without a database connection.
//
// Flow:
//  1. Validate VM is in 'error' state.
//  2. Validate VM has a host_id.
//  3. Set status to 'failing_over'.
//  4. Reschedule (pick new host, excluding the dead one).
//  5. On success: set status to 'running'.
//  6. On any error (defer): set status back to 'error'.
func runFailoverStateMachine(
	vm *fakeVMState,
	hasHostID bool,
	rescheduleErr error,
	workerCreateErr error,
) error {
	// Step 1: validate status
	if vm.status != VMStatusError {
		return errors.New("compute: FailoverVM: vm is not in error state (current: " + string(vm.status) + ")")
	}
	// Step 2: validate host
	if !hasHostID {
		return errors.New("compute: FailoverVM: vm has no assigned host")
	}

	// Step 3: mark failing_over
	vm.setStatus(VMStatusFailingOver, "")

	var retErr error
	defer func() {
		if retErr != nil {
			vm.setStatus(VMStatusError, retErr.Error())
		}
	}()

	// Step 4: reschedule
	if rescheduleErr != nil {
		retErr = errors.New("compute: FailoverVM: reschedule: " + rescheduleErr.Error())
		return retErr
	}

	// Step 5: worker CreateVM
	if workerCreateErr != nil {
		retErr = errors.New("compute: FailoverVM: worker CreateVM: " + workerCreateErr.Error())
		return retErr
	}

	// Step 6: success
	vm.setStatus(VMStatusRunning, "")
	return nil
}

// TestFailoverVM_NotInErrorState verifies that FailoverVM returns an error
// immediately if the VM is not in 'error' state (no status transitions).
func TestFailoverVM_NotInErrorState(t *testing.T) {
	for _, status := range []VMStatus{VMStatusRunning, VMStatusStopped, VMStatusBuilding} {
		vm := newFakeVMState(status)
		err := runFailoverStateMachine(vm, true, nil, nil)
		if err == nil {
			t.Errorf("status=%s: expected error, got nil", status)
		}
		// Status must remain unchanged (no transition occurred).
		if len(vm.history) != 1 {
			t.Errorf("status=%s: status history = %v, want no transitions", status, vm.history)
		}
	}
}

// TestFailoverVM_NoHostID verifies that FailoverVM returns an error immediately
// when the VM has no assigned host_id (no status transitions).
func TestFailoverVM_NoHostID(t *testing.T) {
	vm := newFakeVMState(VMStatusError)
	err := runFailoverStateMachine(vm, false /* no host */, nil, nil)
	if err == nil {
		t.Fatal("expected error for VM with no host_id, got nil")
	}
	// No transitions: status stays at 'error'.
	if len(vm.history) != 1 {
		t.Errorf("status history = %v, want no transitions", vm.history)
	}
	if vm.status != VMStatusError {
		t.Errorf("final status = %q, want %q", vm.status, VMStatusError)
	}
}

// TestFailoverVM_RescheduleFailure verifies that when Reschedule fails, the VM
// transitions: error → failing_over → error.
func TestFailoverVM_RescheduleFailure(t *testing.T) {
	vm := newFakeVMState(VMStatusError)
	reschedErr := errors.New("no suitable host")

	err := runFailoverStateMachine(vm, true, reschedErr, nil)
	if err == nil {
		t.Fatal("expected error when reschedule fails, got nil")
	}

	wantHistory := []VMStatus{VMStatusError, VMStatusFailingOver, VMStatusError}
	if len(vm.history) != len(wantHistory) {
		t.Fatalf("status history = %v, want %v", vm.history, wantHistory)
	}
	for i, want := range wantHistory {
		if vm.history[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, vm.history[i], want)
		}
	}
	if vm.status != VMStatusError {
		t.Errorf("final status = %q, want %q", vm.status, VMStatusError)
	}
}

// TestFailoverVM_WorkerCreateFails verifies that when the worker CreateVM call fails,
// the VM transitions: error → failing_over → error.
func TestFailoverVM_WorkerCreateFails(t *testing.T) {
	vm := newFakeVMState(VMStatusError)
	createErr := errors.New("rpc error: code = Internal desc = disk I/O error")

	err := runFailoverStateMachine(vm, true, nil, createErr)
	if err == nil {
		t.Fatal("expected error when worker CreateVM fails, got nil")
	}

	wantHistory := []VMStatus{VMStatusError, VMStatusFailingOver, VMStatusError}
	if len(vm.history) != len(wantHistory) {
		t.Fatalf("status history = %v, want %v", vm.history, wantHistory)
	}
	for i, want := range wantHistory {
		if vm.history[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, vm.history[i], want)
		}
	}
	if vm.status != VMStatusError {
		t.Errorf("final status = %q, want %q", vm.status, VMStatusError)
	}
}

// TestFailoverVM_Success verifies the happy path: error → failing_over → running.
func TestFailoverVM_Success(t *testing.T) {
	vm := newFakeVMState(VMStatusError)

	err := runFailoverStateMachine(vm, true, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	wantHistory := []VMStatus{VMStatusError, VMStatusFailingOver, VMStatusRunning}
	if len(vm.history) != len(wantHistory) {
		t.Fatalf("status history = %v, want %v", vm.history, wantHistory)
	}
	for i, want := range wantHistory {
		if vm.history[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, vm.history[i], want)
		}
	}
	if vm.status != VMStatusRunning {
		t.Errorf("final status = %q, want %q", vm.status, VMStatusRunning)
	}
}

// containsErrMessage reports whether err.Error() contains the substring sub.
func containsErrMessage(err error, sub string) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
