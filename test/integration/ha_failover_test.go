//go:build integration

package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/controller/fencing"
	"github.com/tjst-t/cirrus/internal/controller/reconcile"
	"github.com/tjst-t/cirrus/internal/host"
)

// waitForFailover polls VM status allowing intermediate states (error, failing_over)
// until the VM reaches running on a different host, or timeout.
// Returns the final VM and its host_id from DB.
func waitForFailover(
	t *testing.T,
	pool *pgxpool.Pool,
	c *client.Client,
	ctx context.Context,
	tenantID, vmID uuid.UUID,
	originalHostID uuid.UUID,
	timeout time.Duration,
) *compute.VM {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		vm, err := c.GetVM(ctx, tenantID, vmID)
		if err != nil {
			t.Logf("waitForFailover: get VM %s: %v (retrying)", vmID, err)
			time.Sleep(3 * time.Second)
			continue
		}
		t.Logf("waitForFailover: VM %s status=%s", vmID, vm.Status)

		if vm.Status == compute.VMStatusRunning {
			// Verify the VM is on a different host.
			var currentHostID uuid.UUID
			err = pool.QueryRow(ctx,
				`SELECT host_id FROM vms WHERE id = $1`, vmID,
			).Scan(&currentHostID)
			if err != nil {
				t.Fatalf("waitForFailover: get host_id: %v", err)
			}
			if currentHostID != originalHostID {
				t.Logf("waitForFailover: VM %s failed over from host %s to host %s",
					vmID, originalHostID, currentHostID)
				return vm
			}
			// Running but on same host — may be a transient state, keep polling.
			t.Logf("waitForFailover: VM %s is running but still on original host %s (waiting)", vmID, originalHostID)
		}
		// Intermediate states (error, failing_over) are accepted — keep polling.
		time.Sleep(3 * time.Second)
	}
	// Print final state for diagnosis.
	vm, _ := c.GetVM(ctx, tenantID, vmID)
	var status compute.VMStatus
	if vm != nil {
		status = vm.Status
	}
	t.Fatalf("timeout: VM %s did not complete failover within %s (final status: %s)", vmID, timeout, status)
	return nil
}

// markHostFaulty sets missed_heartbeat_count and last_heartbeat so HeartbeatMonitor
// will detect this host as faulty on its next tick.
func markHostFaultyViaHeartbeat(ctx context.Context, pool *pgxpool.Pool, hostID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE hosts
		 SET missed_heartbeat_count = 10,
		     last_heartbeat = now() - interval '1 hour',
		     updated_at = now()
		 WHERE id = $1`,
		hostID,
	)
	return err
}

// waitForHostFaulty polls until hosts.operational_state = 'faulty', or timeout.
func waitForHostFaulty(t *testing.T, pool *pgxpool.Pool, ctx context.Context, hostID uuid.UUID, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var state string
		err := pool.QueryRow(ctx,
			`SELECT operational_state FROM hosts WHERE id = $1`, hostID,
		).Scan(&state)
		if err != nil {
			t.Fatalf("waitForHostFaulty: query: %v", err)
		}
		t.Logf("waitForHostFaulty: host %s operational_state=%s", hostID, state)
		if state == string(host.StateFaulty) {
			return
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("timeout: host %s did not become faulty within %s", hostID, timeout)
}

// restoreHost resets the host to active state.
func restoreHost(t *testing.T, pool *pgxpool.Pool, hostID uuid.UUID) {
	_, err := pool.Exec(context.Background(),
		`UPDATE hosts
		 SET operational_state = 'active',
		     missed_heartbeat_count = 0,
		     last_heartbeat = now(),
		     updated_at = now()
		 WHERE id = $1`,
		hostID,
	)
	if err != nil {
		t.Logf("restoreHost: failed to restore host %s: %v", hostID, err)
	}
}

// TestHAFailover_HappyPath verifies the full HA failover pipeline:
// host marked faulty → HeartbeatMonitor detects → fencing (power-off via sim) →
// FailoverTrigger rescheduled VM on new host → VM returns to running.
//
// Prerequisites:
//
//	CIRRUS_ENDPOINT    — controller API base URL
//	CIRRUS_TOKEN       — bearer token
//	CIRRUS_TENANT_ID   — tenant UUID
//	TEST_DB_DSN        — DB DSN
//	LIBVIRT_SIM_URL    — libvirt-sim management URL (required; fencing calls this)
func TestHAFailover_HappyPath(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping HA failover happy-path integration test")
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
	if os.Getenv("LIBVIRT_SIM_URL") == "" {
		t.Skip("LIBVIRT_SIM_URL not set; skipping HA failover test (requires fencing sim)")
	}

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgresql://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping DB: %v", err)
	}

	c := client.New(endpoint, token)

	// Step 1: Create a VM and wait for running.
	flavors, err := c.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Skipf("no flavors available: %v", err)
	}

	vmName := fmt.Sprintf("test-ha-happy-%d", time.Now().Unix())
	vm, err := c.CreateVM(ctx, tenantID, client.CreateVMRequest{
		Name:     vmName,
		FlavorID: flavors[0].ID.String(),
	})
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	t.Logf("created VM %s (%s)", vm.Name, vm.ID)

	// Step 2: Wait for VM to reach running.
	waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)
	t.Logf("VM is running: %s", vm.ID)

	// Step 3: Query host_id from DB.
	var originalHostID uuid.UUID
	err = pool.QueryRow(ctx,
		`SELECT host_id FROM vms WHERE id = $1`, vm.ID,
	).Scan(&originalHostID)
	if err != nil {
		t.Fatalf("get host_id from DB: %v", err)
	}
	t.Logf("VM host_id: %s", originalHostID)

	// Cleanup: restore host and force-stop + delete VM.
	t.Cleanup(func() {
		restoreHost(t, pool, originalHostID)
		_ = c.VMAction(context.Background(), tenantID, vm.ID, "force-stop")
		time.Sleep(2 * time.Second)
		_ = c.DeleteVM(context.Background(), tenantID, vm.ID)
	})

	// Step 4: Manipulate DB to make HeartbeatMonitor detect missed beats.
	if err := markHostFaultyViaHeartbeat(ctx, pool, originalHostID); err != nil {
		t.Fatalf("mark host faulty via heartbeat manipulation: %v", err)
	}
	t.Logf("host %s: set missed_heartbeat_count=10, last_heartbeat=1h ago", originalHostID)

	// Step 5: Wait up to 60s for host to become faulty (HeartbeatMonitor tick).
	waitForHostFaulty(t, pool, ctx, originalHostID, 60*time.Second)
	t.Logf("host %s is now faulty", originalHostID)

	// Step 6: Wait up to 120s for VM to failover and run on a different host.
	// Do NOT use waitForVMStatus here — it fatally exits on error state.
	waitForFailover(t, pool, c, ctx, tenantID, vm.ID, originalHostID, 120*time.Second)
	t.Logf("HA failover happy-path: VM %s successfully failed over from host %s", vm.ID, originalHostID)
}

// noopCascade is a no-op HostFaultCascader used when cascade has already been applied.
type noopCascade struct{}

func (noopCascade) Handle(context.Context, uuid.UUID) {}

// TestHAFailover_FencingFailure verifies that when fencing fails, the failover
// is aborted (Option A: safe mode), the VM stays in error state, and a critical
// drift event is fired.
//
// This test uses synthetic DB rows (no real VM/worker) to avoid live-system races.
func TestHAFailover_FencingFailure(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping HA failover fencing-failure integration test")
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

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgresql://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping DB: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Anchor timestamp for drift event queries.
	testStart := time.Now()

	// Step 1: Insert a synthetic host row with operational_state = 'faulty'.
	// This avoids any live-system race with HeartbeatMonitor reacting to the same host.
	synthHostID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO hosts (id, name, operational_state, worker_grpc_addr, created_at, updated_at)
		 VALUES ($1, $2, 'faulty', '', now(), now())`,
		synthHostID, "test-fencing-failure-host-"+synthHostID.String()[:8],
	)
	if err != nil {
		t.Fatalf("insert synthetic host: %v", err)
	}

	// Step 2: Insert a synthetic VM row on this host in error state.
	synthVMID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO vms (id, tenant_id, name, status, host_id, created_at, updated_at)
		 VALUES ($1, $2, $3, 'error', $4, now(), now())`,
		synthVMID, tenantID, "test-fencing-failure-vm-"+synthVMID.String()[:8], synthHostID,
	)
	if err != nil {
		t.Fatalf("insert synthetic vm: %v", err)
	}

	// Cleanup: remove synthetic rows.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM vms WHERE id = $1`, synthVMID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM hosts WHERE id = $1`, synthHostID)
	})

	t.Logf("synthetic host %s (faulty) and VM %s (error) inserted", synthHostID, synthVMID)

	// Step 3: Create FailoverTrigger with a fencing agent pointing at localhost:1
	// (guaranteed connection refused → fencing failure).
	driftHandler := reconcile.NewDriftHandler(reconcile.DriftHandlerConfig{
		Pool:            pool,
		Logger:          logger,
		AutoHealEnabled: false,
	})

	failoverTrigger := reconcile.NewFailoverTrigger(
		noopCascade{},
		fencing.NewSimFencingAgent("http://localhost:1", 2*time.Second),
		nil, // computeSvc — never reached because fencing will fail first
		driftHandler,
		pool,
		logger,
	)

	// Step 4: Call Handle (non-blocking goroutine).
	// Poll DB until the fencing-failure drift event appears (up to 15s, poll every 500ms).
	failoverTrigger.Handle(ctx, synthHostID)
	t.Logf("FailoverTrigger.Handle called; polling for drift event (up to 15s)...")

	driftDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(driftDeadline) {
		var cnt int
		_ = pool.QueryRow(ctx,
			`SELECT count(*) FROM drift_events
			 WHERE resource = 'host' AND resource_id = $1
			   AND detected_by = 'failover_trigger'
			   AND created_at >= $2`,
			synthHostID.String(), testStart,
		).Scan(&cnt)
		if cnt > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Step 5: Assert synthetic VM is still in error state (failover was aborted).
	var vmStatus string
	err = pool.QueryRow(ctx,
		`SELECT status FROM vms WHERE id = $1`, synthVMID,
	).Scan(&vmStatus)
	if err != nil {
		t.Fatalf("get vm status after fencing failure: %v", err)
	}
	t.Logf("VM status after fencing failure: %s", vmStatus)
	if vmStatus != "error" {
		t.Errorf("expected VM status=error after fencing failure (safe-mode abort), got %s", vmStatus)
	}

	// Step 6: Assert a critical drift_event was fired for the host.
	var driftCount int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM drift_events
		 WHERE resource = 'host'
		   AND resource_id = $1
		   AND detected_by = 'failover_trigger'
		   AND created_at >= $2`,
		synthHostID.String(), testStart,
	).Scan(&driftCount)
	if err != nil {
		t.Fatalf("query drift_events: %v", err)
	}
	t.Logf("drift_events for host %s (detected_by=failover_trigger): %d", synthHostID, driftCount)
	if driftCount == 0 {
		t.Errorf("expected a drift_event with resource=host, resource_id=%s, detected_by=failover_trigger", synthHostID)
	}

}

// TestHAFailover_MultipleVMs verifies that when a host with multiple VMs
// goes faulty, all VMs are failed over to new hosts.
func TestHAFailover_MultipleVMs(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping HA failover multi-VM integration test")
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
	if os.Getenv("LIBVIRT_SIM_URL") == "" {
		t.Skip("LIBVIRT_SIM_URL not set; skipping HA failover multi-VM test (requires fencing sim)")
	}

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgresql://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping DB: %v", err)
	}

	c := client.New(endpoint, token)

	// Step 1: Find a flavor.
	flavors, err := c.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Skipf("no flavors available: %v", err)
	}

	const vmCount = 3
	vms := make([]*compute.VM, vmCount)
	vmIDs := make([]uuid.UUID, vmCount)

	// Step 2: Create all 3 VMs first (parallel creation), then wait for each to reach running.
	for i := range vmCount {
		vmName := fmt.Sprintf("test-ha-multi-%d-%d", time.Now().Unix(), i)
		created, err := c.CreateVM(ctx, tenantID, client.CreateVMRequest{
			Name:     vmName,
			FlavorID: flavors[0].ID.String(),
		})
		if err != nil {
			t.Fatalf("create vm %d: %v", i, err)
		}
		t.Logf("created VM[%d] %s (%s)", i, created.Name, created.ID)
		vms[i] = created
		vmIDs[i] = created.ID
	}

	// Wait for all 3 VMs to reach running.
	for i, v := range vms {
		waitForVMStatus(t, c, ctx, tenantID, v.ID, compute.VMStatusRunning, 90*time.Second)
		t.Logf("VM[%d] %s is running", i, v.ID)
	}

	// Step 3: Query host_id for each VM.
	hostIDs := make([]uuid.UUID, vmCount)
	for i, vmID := range vmIDs {
		err = pool.QueryRow(ctx,
			`SELECT host_id FROM vms WHERE id = $1`, vmID,
		).Scan(&hostIDs[i])
		if err != nil {
			t.Fatalf("get host_id for VM[%d]: %v", i, err)
		}
		t.Logf("VM[%d] %s host_id=%s", i, vmID, hostIDs[i])
	}

	// Step 4: Verify all 3 VMs are on the same host.
	for i := 1; i < vmCount; i++ {
		if hostIDs[i] != hostIDs[0] {
			t.Skipf("VMs spread across multiple hosts (%v), cannot test multi-VM failover", hostIDs)
		}
	}
	originalHostID := hostIDs[0]
	t.Logf("all 3 VMs are on host %s — proceeding with multi-VM failover test", originalHostID)

	// Cleanup: restore host and delete all VMs.
	t.Cleanup(func() {
		restoreHost(t, pool, originalHostID)
		for _, vmID := range vmIDs {
			_ = c.VMAction(context.Background(), tenantID, vmID, "force-stop")
		}
		time.Sleep(2 * time.Second)
		for _, vmID := range vmIDs {
			_ = c.DeleteVM(context.Background(), tenantID, vmID)
		}
	})

	// Step 5: Manipulate DB to trigger HeartbeatMonitor detection.
	if err := markHostFaultyViaHeartbeat(ctx, pool, originalHostID); err != nil {
		t.Fatalf("mark host faulty via heartbeat: %v", err)
	}
	t.Logf("host %s: set missed_heartbeat_count=10, last_heartbeat=1h ago", originalHostID)

	// Step 6: Wait up to 60s for host to become faulty.
	waitForHostFaulty(t, pool, ctx, originalHostID, 60*time.Second)
	t.Logf("host %s is faulty", originalHostID)

	// Step 7: Wait up to 180s for all 3 VMs to failover and run on different hosts.
	// Allow intermediate states (error, failing_over).
	deadline := time.Now().Add(180 * time.Second)
	allDone := false
	for time.Now().Before(deadline) {
		allDone = true
		for i, vmID := range vmIDs {
			vm, err := c.GetVM(ctx, tenantID, vmID)
			if err != nil {
				t.Logf("get VM[%d] %s: %v (retrying)", i, vmID, err)
				allDone = false
				continue
			}
			t.Logf("VM[%d] %s status=%s", i, vmID, vm.Status)

			if vm.Status != compute.VMStatusRunning {
				allDone = false
				continue
			}
			// Running — check it's on a different host.
			var currentHostID uuid.UUID
			err = pool.QueryRow(ctx,
				`SELECT host_id FROM vms WHERE id = $1`, vmID,
			).Scan(&currentHostID)
			if err != nil {
				t.Fatalf("get host_id for VM[%d]: %v", i, err)
			}
			if currentHostID == originalHostID {
				t.Logf("VM[%d] %s running but still on original host — waiting", i, vmID)
				allDone = false
			}
		}
		if allDone {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if !allDone {
		t.Fatalf("timeout: not all VMs completed failover within 180s")
	}

	// Step 8: Final assertions — all VMs running on different hosts.
	for i, vmID := range vmIDs {
		vm, err := c.GetVM(ctx, tenantID, vmID)
		if err != nil {
			t.Errorf("VM[%d] %s: get vm: %v", i, vmID, err)
			continue
		}
		if vm.Status != compute.VMStatusRunning {
			t.Errorf("VM[%d] %s: expected status=running, got %s", i, vmID, vm.Status)
		}

		var currentHostID uuid.UUID
		err = pool.QueryRow(ctx,
			`SELECT host_id FROM vms WHERE id = $1`, vmID,
		).Scan(&currentHostID)
		if err != nil {
			t.Errorf("VM[%d] %s: get host_id: %v", i, vmID, err)
			continue
		}
		if currentHostID == originalHostID {
			t.Errorf("VM[%d] %s: expected host_id != %s after failover, but still on same host",
				i, vmID, originalHostID)
		} else {
			t.Logf("VM[%d] %s: failed over from %s to %s (running)", i, vmID, originalHostID, currentHostID)
		}
	}
}
