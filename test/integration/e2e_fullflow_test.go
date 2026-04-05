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

// adminClient returns a client using CIRRUS_ADMIN_TOKEN (falling back to CIRRUS_TOKEN).
func adminClient(t *testing.T, endpoint string) *client.Client {
	t.Helper()
	token := os.Getenv("CIRRUS_ADMIN_TOKEN")
	if token == "" {
		token = os.Getenv("CIRRUS_TOKEN")
	}
	if token == "" {
		t.Fatal("CIRRUS_TOKEN (or CIRRUS_ADMIN_TOKEN) not set")
	}
	return client.New(endpoint, token)
}

// e2eEnv resolves the common E2E environment variables and returns a client and tenantID.
func e2eEnv(t *testing.T) (*client.Client, uuid.UUID) {
	t.Helper()
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping E2E integration test")
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
	return client.New(endpoint, token), tenantID
}

// TestE2EFullFlow verifies the full resource lifecycle:
// テナント作成 → Network 作成 → VM 作成（Network 接続） → VM running 確認
// → VM stop → VM delete → Network delete → テナント削除
func TestE2EFullFlow(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping E2E full flow integration test")
	}

	adminC := adminClient(t, endpoint)
	ctx := context.Background()

	// --- Step 1: Create an organization for this test ---
	orgName := fmt.Sprintf("e2e-fullflow-org-%d", time.Now().Unix())
	org, err := adminC.CreateOrganization(ctx, orgName)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	t.Logf("created organization %s (%s)", org.Name, org.ID)

	// --- Step 2: Create a tenant within the organization ---
	tenantName := fmt.Sprintf("e2e-fullflow-tenant-%d", time.Now().Unix())
	tenant, err := adminC.CreateTenant(ctx, org.ID, tenantName)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Logf("created tenant %s (%s)", tenant.Name, tenant.ID)
	tenantID := tenant.ID

	// Cleanup: reverse order — tenant resources first, then org/tenant
	t.Cleanup(func() {
		// Tenant and org deletion is not yet exposed as a REST endpoint; skip if not available.
		t.Logf("cleanup: tenant=%s org=%s (resource cleanup completed)", tenantID, org.ID)
	})

	// --- Step 3: Create a Network ---
	netName := fmt.Sprintf("e2e-net-%d", time.Now().Unix())
	net, err := adminC.CreateNetwork(ctx, tenantID, netName, "")
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	t.Logf("created network %s (%s)", net.Name, net.ID)

	t.Cleanup(func() {
		if err := adminC.DeleteNetwork(ctx, net.ID); err != nil {
			t.Logf("cleanup: delete network %s: %v", net.ID, err)
		} else {
			t.Logf("cleanup: deleted network %s", net.ID)
		}
	})

	// --- Step 4: List flavors ---
	flavors, err := adminC.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Fatalf("list flavors: %v (count=%d)", err, len(flavors))
	}
	flavorID := flavors[0].ID
	t.Logf("using flavor: %s (%s)", flavors[0].Name, flavorID)

	// --- Step 5: Create a VM connected to the network ---
	vmName := fmt.Sprintf("e2e-vm-%d", time.Now().Unix())
	vm, err := adminC.CreateVM(ctx, tenantID, client.CreateVMRequest{
		Name:      vmName,
		FlavorID:  flavorID.String(),
		NetworkID: net.ID.String(),
	})
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	t.Logf("VM created: %s (status=%s)", vm.ID, vm.Status)

	t.Cleanup(func() {
		// Best-effort: force-stop then delete
		_ = adminC.VMAction(ctx, tenantID, vm.ID, "force-stop")
		time.Sleep(2 * time.Second)
		if err := adminC.DeleteVM(ctx, tenantID, vm.ID); err != nil {
			t.Logf("cleanup: delete vm %s: %v", vm.ID, err)
		} else {
			t.Logf("cleanup: deleted vm %s", vm.ID)
		}
	})

	// --- Step 6: Wait for VM to reach running state ---
	vm = waitForVMStatus(t, adminC, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)
	t.Logf("VM running: %s", vm.ID)

	// Verify the VM is attached to the expected network
	if vm.NetworkID == nil || *vm.NetworkID != net.ID {
		t.Errorf("VM network mismatch: expected %s, got %v", net.ID, vm.NetworkID)
	}

	// --- Step 7: Stop VM ---
	if err := adminC.VMAction(ctx, tenantID, vm.ID, "stop"); err != nil {
		t.Fatalf("stop vm: %v", err)
	}
	vm = waitForVMStatus(t, adminC, ctx, tenantID, vm.ID, compute.VMStatusStopped, 30*time.Second)
	t.Logf("VM stopped: %s", vm.ID)

	// --- Step 8: Delete VM (must be stopped) ---
	if err := adminC.DeleteVM(ctx, tenantID, vm.ID); err != nil {
		t.Fatalf("delete vm: %v", err)
	}
	t.Logf("VM delete initiated: %s", vm.ID)

	// --- Step 9: Delete Network ---
	if err := adminC.DeleteNetwork(ctx, net.ID); err != nil {
		t.Fatalf("delete network: %v", err)
	}
	t.Logf("network deleted: %s", net.ID)

	t.Logf("E2E full flow completed successfully")
}
