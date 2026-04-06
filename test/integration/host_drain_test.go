//go:build integration

package integration

// TestHostDrainToMaintenance verifies S017-4 behavior:
// Setting a host to draining when it has no running VMs causes the
// HeartbeatMonitor to auto-transition it to maintenance.
//
// The test uses a host that currently has no VMs (or force-stops them first),
// transitions it to draining via the API, then waits for the monitor to
// detect that VM count == 0 and move the host to maintenance.
//
// Prerequisites:
//   - CIRRUS_ENDPOINT, CIRRUS_TOKEN set
//   - At least one active host registered in the system
//   - The controller is running with a reconcile-interval short enough to
//     observe the transition within 90s (default is 30s).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/host"
)

func TestHostDrainToMaintenance(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping host drain integration test")
	}

	adminC := adminClient(t, endpoint)
	ctx := context.Background()

	// Find an active host that is not currently running VMs.
	// We prefer a host that has no VMs so we don't disrupt the running environment.
	hosts, err := adminC.ListHosts(ctx)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}

	// Find an active host with zero used resources (proxy for no VMs).
	var targetHost *host.Host
	for i := range hosts {
		h := &hosts[i]
		if h.OperationalState != host.StateActive {
			continue
		}
		// Decode resource_used to check vcpus/memory_mb.
		if hostHasNoVMs(h) {
			targetHost = h
			break
		}
	}
	if targetHost == nil {
		t.Skip("no active host with zero resource usage found; skipping drain test to avoid disrupting running VMs")
	}
	t.Logf("selected host: %s (%s) for drain test", targetHost.Name, targetHost.ID)

	// Restore to active after test.
	t.Cleanup(func() {
		h, err := adminC.GetHost(ctx, targetHost.ID)
		if err != nil {
			t.Logf("cleanup: get host: %v", err)
			return
		}
		if h.OperationalState == host.StateMaintenance || h.OperationalState == host.StateDraining {
			if _, err := adminC.HostAction(ctx, targetHost.ID, "activate"); err != nil {
				t.Logf("cleanup: re-activate host %s: %v", targetHost.ID, err)
			} else {
				t.Logf("cleanup: host %s re-activated", targetHost.Name)
			}
		}
	})

	// Step 1: drain the host.
	h, err := adminC.HostAction(ctx, targetHost.ID, "drain")
	if err != nil {
		t.Fatalf("drain host %s: %v", targetHost.ID, err)
	}
	if h.OperationalState != host.StateDraining {
		t.Fatalf("expected draining after drain action, got %s", h.OperationalState)
	}
	t.Logf("host %s is now draining", targetHost.Name)

	// Step 2: wait for HeartbeatMonitor to detect VM count == 0 and auto-transition
	// to maintenance. The monitor runs every reconcile-interval (default 30s).
	// We poll for up to 90 seconds.
	t.Logf("waiting for host to auto-transition draining → maintenance (up to 90s)...")
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		current, err := adminC.GetHost(ctx, targetHost.ID)
		if err != nil {
			t.Logf("get host (polling): %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		switch current.OperationalState {
		case host.StateMaintenance:
			t.Logf("host %s auto-transitioned draining → maintenance (S017-4 verified)", targetHost.Name)
			return
		case host.StateDraining:
			t.Logf("still draining... (elapsed=%s)", time.Until(deadline).Round(time.Second))
		default:
			t.Fatalf("unexpected host state during drain wait: %s", current.OperationalState)
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("timeout: host %s did not transition to maintenance within 90s (draining→maintenance auto-transition failed)", targetHost.Name)
}

// TestHostMaintenanceBlockedWithRunningVMs verifies S017-1-3:
// active→maintenance is only allowed when VM count == 0.
// We create a VM, wait for it to be running, then try the maintenance action
// on that host and expect a 409 Conflict.
func TestHostMaintenanceBlockedWithRunningVMs(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping")
	}
	tenantIDStr := os.Getenv("CIRRUS_TENANT_ID")
	if tenantIDStr == "" {
		t.Skip("CIRRUS_TENANT_ID not set; skipping")
	}

	adminC := adminClient(t, endpoint)
	c, tenantID := e2eEnv(t)
	ctx := context.Background()

	flavors, err := adminC.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Skip("no flavors available")
	}

	// Create a VM and wait for it to run.
	ts := time.Now().Unix()
	vm, err := c.CreateVM(ctx, tenantID, client.CreateVMRequest{
		Name:     fmt.Sprintf("drain-block-%d", ts%1000000),
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

	vm = waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)
	t.Logf("VM %s running on host", vm.ID)

	// Find the host this VM is running on.
	if vm.HostID == nil {
		t.Fatal("VM has no host_id after reaching running state")
	}
	hostID := *vm.HostID
	t.Logf("VM host: %s", hostID)

	// Verify host is active.
	h, err := adminC.GetHost(ctx, hostID)
	if err != nil {
		t.Fatalf("get host: %v", err)
	}
	if h.OperationalState != host.StateActive {
		t.Skipf("host %s is not active (state=%s); skipping", h.Name, h.OperationalState)
	}

	// "maintenance" action on a host with a running VM should return 409.
	_, err = adminC.HostAction(ctx, hostID, "maintenance")
	if err == nil {
		// Succeeded unexpectedly — re-activate and fail.
		_, _ = adminC.HostAction(ctx, hostID, "activate")
		t.Fatalf("expected 409 when setting maintenance on host with running VM, but got no error")
	}
	if !containsAny(err.Error(), "409", "conflict") {
		t.Errorf("expected 409/conflict error, got: %v", err)
	} else {
		t.Logf("maintenance correctly rejected with 409 while VM is running: %v", err)
	}
}


func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// hostHasNoVMs decodes ResourceUsed (json.RawMessage) and returns true when
// both vcpus and memory_mb are zero (or the field is absent/null).
func hostHasNoVMs(h *host.Host) bool {
	if len(h.ResourceUsed) == 0 {
		return true
	}
	var used struct {
		VCPUs    float64 `json:"vcpus"`
		MemoryMB float64 `json:"memory_mb"`
	}
	if err := json.Unmarshal(h.ResourceUsed, &used); err != nil {
		return true // can't decode → assume empty
	}
	return used.VCPUs == 0 && used.MemoryMB == 0
}
