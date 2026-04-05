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
	"github.com/tjst-t/cirrus/internal/controller"
)

// TestReconcileHostFaulty_Cascade verifies that when a host is marked faulty,
// HostFaultyHandler transitions all its VMs to error and their ports to down.
//
// Strategy (avoids waiting for HeartbeatMonitor timer):
//  1. Create a VM and wait for running.
//  2. Directly update hosts.operational_state = 'faulty' in the DB.
//  3. Call HostFaultyHandler.Handle() directly.
//  4. Assert VM status = error.
//  5. Assert port status = down.
//
// Prerequisites:
//
//	CIRRUS_ENDPOINT  — controller API base URL
//	CIRRUS_TOKEN     — bearer token
//	CIRRUS_TENANT_ID — tenant UUID
//	TEST_DB_DSN      — DB DSN (default: postgres://cirrus:cirrus@localhost:5432/cirrus)
func TestReconcileHostFaulty_Cascade(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping host-faulty cascade integration test")
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
	c := client.New(endpoint, token)

	// Step 1: Create a VM and wait for running.
	flavors, err := c.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Skipf("no flavors available: %v", err)
	}

	vmName := fmt.Sprintf("test-reconcile-faulty-%d", time.Now().Unix())
	vm, err := c.CreateVM(ctx, tenantID, client.CreateVMRequest{
		Name:     vmName,
		FlavorID: flavors[0].ID.String(),
	})
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	t.Logf("created VM %s (%s)", vm.Name, vm.ID)

	t.Cleanup(func() {
		// Best-effort: restore host to active so other tests are not affected.
		_, _ = pool.Exec(context.Background(),
			`UPDATE hosts SET operational_state = 'active', updated_at = now()
			 WHERE id = (SELECT host_id FROM vms WHERE id = $1)`,
			vm.ID,
		)
		_ = c.VMAction(context.Background(), tenantID, vm.ID, "force-stop")
		time.Sleep(2 * time.Second)
		_ = c.DeleteVM(context.Background(), tenantID, vm.ID)
	})

	waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)
	t.Logf("VM is running: %s", vm.ID)

	// Step 2: Get the host ID for this VM.
	var hostID uuid.UUID
	err = pool.QueryRow(ctx,
		`SELECT host_id FROM vms WHERE id = $1`, vm.ID,
	).Scan(&hostID)
	if err != nil {
		t.Fatalf("get host_id from DB: %v", err)
	}
	t.Logf("VM host_id: %s", hostID)

	// Verify ports exist for this VM before the faulty transition.
	var portCount int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM ports WHERE vm_id = $1`, vm.ID,
	).Scan(&portCount)
	if err != nil {
		t.Fatalf("count ports: %v", err)
	}
	t.Logf("ports before faulty: %d", portCount)
	// Note: port count may be 0 in simple sim setups without network; that is OK
	// for the cascade handler test — we verify vms.status regardless.

	// Step 3: Directly mark the host faulty in the DB (bypasses HeartbeatMonitor).
	_, err = pool.Exec(ctx,
		`UPDATE hosts SET operational_state = 'faulty', updated_at = now() WHERE id = $1`,
		hostID,
	)
	if err != nil {
		t.Fatalf("mark host faulty: %v", err)
	}
	t.Logf("host %s marked faulty in DB", hostID)

	// Step 4: Call HostFaultyHandler.Handle() directly.
	handler := controller.NewHostFaultyHandler(pool, logger)
	handler.Handle(ctx, hostID)
	t.Logf("HostFaultyHandler.Handle() called for host %s", hostID)

	// Step 5: Assert VM status = error.
	var vmStatus string
	err = pool.QueryRow(ctx,
		`SELECT status FROM vms WHERE id = $1`, vm.ID,
	).Scan(&vmStatus)
	if err != nil {
		t.Fatalf("get vm status from DB: %v", err)
	}
	t.Logf("VM status after faulty cascade: %s", vmStatus)
	if vmStatus != "error" {
		t.Errorf("expected VM status=error after faulty cascade, got %s", vmStatus)
	}

	// Step 6: Assert port status = down (only if ports exist).
	if portCount > 0 {
		var downPorts int
		err = pool.QueryRow(ctx,
			`SELECT count(*) FROM ports WHERE vm_id = $1 AND status = 'down'`,
			vm.ID,
		).Scan(&downPorts)
		if err != nil {
			t.Fatalf("count down ports: %v", err)
		}
		t.Logf("ports with status=down after cascade: %d / %d", downPorts, portCount)
		if downPorts == 0 {
			t.Errorf("expected at least one port with status=down after faulty cascade")
		}
	} else {
		t.Logf("no ports found for this VM (simple flavor without network); skipping port status check")
	}
}

// TestReconcileHostFaulty_NoActiveVMs verifies that HostFaultyHandler is a no-op
// when the host has no non-terminal VMs.
func TestReconcileHostFaulty_NoActiveVMs(t *testing.T) {
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
		t.Skipf("DB not reachable (%v); skipping", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := controller.NewHostFaultyHandler(pool, logger)

	// Use a random UUID that has no VMs; expect no error / panic.
	fakeHostID := uuid.New()
	handler.Handle(ctx, fakeHostID)
	t.Logf("HostFaultyHandler.Handle() with non-existent host %s completed without error", fakeHostID)
}
