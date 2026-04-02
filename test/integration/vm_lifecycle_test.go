//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/compute"
)

// TestVMLifecycle verifies the full VM lifecycle:
// create → wait running → stop → start → reboot → delete
func TestVMLifecycle(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping VM lifecycle integration test")
	}
	token := os.Getenv("CIRRUS_TOKEN")
	if token == "" {
		t.Fatal("CIRRUS_TOKEN not set")
	}
	tenantIDStr := os.Getenv("CIRRUS_TENANT_ID")
	if tenantIDStr == "" {
		t.Fatal("CIRRUS_TENANT_ID not set")
	}
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		t.Fatalf("invalid CIRRUS_TENANT_ID: %v", err)
	}

	c := client.New(endpoint, token)
	ctx := context.Background()

	// 1. Find a flavor
	flavors, err := c.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Fatalf("list flavors: %v (count=%d)", err, len(flavors))
	}
	flavorID := flavors[0].ID
	t.Logf("using flavor: %s (%s)", flavors[0].Name, flavorID)

	// 2. Create VM
	vmReq := client.CreateVMRequest{
		Name:     fmt.Sprintf("test-lifecycle-%d", time.Now().Unix()),
		FlavorID: flavorID.String(),
	}
	vm, err := c.CreateVM(ctx, tenantID, vmReq)
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	t.Logf("VM created: %s (status=%s)", vm.ID, vm.Status)

	t.Cleanup(func() {
		// Best-effort cleanup: stop then delete if still running
		_ = c.VMAction(ctx, tenantID, vm.ID, "force-stop")
		time.Sleep(2 * time.Second)
		_ = c.DeleteVM(ctx, tenantID, vm.ID)
	})

	// 3. Wait for VM to reach running state
	vm = waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)
	t.Logf("VM running: %s", vm.ID)

	// 4. Stop VM (graceful)
	if err := c.VMAction(ctx, tenantID, vm.ID, "stop"); err != nil {
		t.Fatalf("stop vm: %v", err)
	}
	vm = waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusStopped, 30*time.Second)
	t.Logf("VM stopped: %s", vm.ID)

	// 5. Start VM
	if err := c.VMAction(ctx, tenantID, vm.ID, "start"); err != nil {
		t.Fatalf("start vm: %v", err)
	}
	vm = waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 30*time.Second)
	t.Logf("VM running again: %s", vm.ID)

	// 6. Reboot VM
	if err := c.VMAction(ctx, tenantID, vm.ID, "reboot"); err != nil {
		t.Fatalf("reboot vm: %v", err)
	}
	vm = waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 30*time.Second)
	t.Logf("VM rebooted: %s", vm.ID)

	// 7. Force-stop VM
	if err := c.VMAction(ctx, tenantID, vm.ID, "force-stop"); err != nil {
		t.Fatalf("force-stop vm: %v", err)
	}
	vm = waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusStopped, 30*time.Second)
	t.Logf("VM force-stopped: %s", vm.ID)

	// 8. Delete VM (must be stopped first)
	if err := c.DeleteVM(ctx, tenantID, vm.ID); err != nil {
		t.Fatalf("delete vm: %v", err)
	}
	t.Logf("VM delete initiated: %s", vm.ID)
}

// TestVMDeleteRunningBlocked verifies that deleting a running VM returns 409.
func TestVMDeleteRunningBlocked(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set")
	}
	token := os.Getenv("CIRRUS_TOKEN")
	tenantIDStr := os.Getenv("CIRRUS_TENANT_ID")
	if token == "" || tenantIDStr == "" {
		t.Fatal("CIRRUS_TOKEN and CIRRUS_TENANT_ID required")
	}
	tenantID, _ := uuid.Parse(tenantIDStr)

	c := client.New(endpoint, token)
	ctx := context.Background()

	flavors, err := c.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Skip("no flavors available")
	}

	vm, err := c.CreateVM(ctx, tenantID, client.CreateVMRequest{
		Name:     fmt.Sprintf("test-delete-block-%d", time.Now().Unix()),
		FlavorID: flavors[0].ID.String(),
	})
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	t.Cleanup(func() {
		_ = c.VMAction(ctx, tenantID, vm.ID, "force-stop")
		time.Sleep(2 * time.Second)
		_ = c.DeleteVM(ctx, tenantID, vm.ID)
	})

	// Wait for running
	waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)

	// Attempt delete while running — must fail with conflict
	err = c.DeleteVM(ctx, tenantID, vm.ID)
	if err == nil {
		t.Error("expected error when deleting running VM, got nil")
	} else {
		t.Logf("correctly rejected delete on running VM: %v", err)
	}
}

func waitForVMStatus(t *testing.T, c *client.Client, ctx context.Context, tenantID, vmID uuid.UUID, want compute.VMStatus, timeout time.Duration) *compute.VM {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		vm, err := c.GetVM(ctx, tenantID, vmID)
		if err != nil {
			t.Fatalf("get vm: %v", err)
		}
		if vm.Status == want {
			return vm
		}
		if vm.Status == compute.VMStatusError {
			t.Fatalf("VM %s entered error state: %s", vmID, vm.ErrorMessage)
		}
		time.Sleep(2 * time.Second)
	}
	vm, _ := c.GetVM(ctx, tenantID, vmID)
	var status compute.VMStatus
	if vm != nil {
		status = vm.Status
	}
	t.Fatalf("timeout waiting for VM %s to reach %s (current: %s)", vmID, want, status)
	return nil
}
