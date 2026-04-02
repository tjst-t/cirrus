package compute

import (
	"testing"
)

func TestVMStateGuards(t *testing.T) {
	tests := []struct {
		status      VMStatus
		canStart    bool
		canStop     bool
		canReboot   bool
		canDelete   bool
		transitional bool
	}{
		{VMStatusPending, false, false, false, false, true},
		{VMStatusBuilding, false, false, false, false, true},
		{VMStatusRunning, false, true, true, false, false},
		{VMStatusStopped, true, false, false, true, false},
		{VMStatusError, false, false, false, true, false},
		{VMStatusDeleting, false, false, false, false, true},
	}

	for _, tc := range tests {
		vm := &VM{Status: tc.status}
		t.Run(string(tc.status), func(t *testing.T) {
			if got := vm.CanStart(); got != tc.canStart {
				t.Errorf("CanStart() = %v, want %v", got, tc.canStart)
			}
			if got := vm.CanStop(); got != tc.canStop {
				t.Errorf("CanStop() = %v, want %v", got, tc.canStop)
			}
			if got := vm.CanReboot(); got != tc.canReboot {
				t.Errorf("CanReboot() = %v, want %v", got, tc.canReboot)
			}
			if got := vm.CanDelete(); got != tc.canDelete {
				t.Errorf("CanDelete() = %v, want %v", got, tc.canDelete)
			}
			if got := vm.IsTransitional(); got != tc.transitional {
				t.Errorf("IsTransitional() = %v, want %v", got, tc.transitional)
			}
		})
	}
}

// TestRunningVMCannotBeDeleted verifies that running VMs are protected from deletion.
func TestRunningVMCannotBeDeleted(t *testing.T) {
	vm := &VM{Status: VMStatusRunning}
	if vm.CanDelete() {
		t.Error("running VM should not be deletable")
	}
}

// TestErrorVMCanOnlyBeDeletedOrRepaired verifies error-state VMs cannot be started/stopped.
func TestErrorVMCanOnlyBeDeletedOrRepaired(t *testing.T) {
	vm := &VM{Status: VMStatusError}
	if vm.CanStart() {
		t.Error("error VM should not be startable")
	}
	if vm.CanStop() {
		t.Error("error VM should not be stoppable")
	}
	if vm.CanReboot() {
		t.Error("error VM should not be rebootable")
	}
	if !vm.CanDelete() {
		t.Error("error VM should be deletable")
	}
}
