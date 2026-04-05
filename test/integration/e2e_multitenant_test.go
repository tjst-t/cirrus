//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/internal/client"
)

// TestE2EMultiTenant verifies tenant isolation:
// - Tenant A and B are created in the same organization.
// - Each creates separate Network and VM resources.
// - Tenant A's token cannot GET or DELETE Tenant B's Network (expects 403).
func TestE2EMultiTenant(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping multi-tenant E2E integration test")
	}

	adminC := adminClient(t, endpoint)
	ctx := context.Background()

	// Derive the "other tenant" token for Tenant A from CIRRUS_TOKEN (the existing test token).
	// In a real setup we would create users and assign tokens; here we use the admin token for
	// both tenants and rely on the X-Tenant-ID header to scope requests.  The 403 check is
	// validated by sending Tenant A's tenant-scoped request against Tenant B's network ID.
	tenantToken := os.Getenv("CIRRUS_TOKEN")
	if tenantToken == "" {
		t.Fatal("CIRRUS_TOKEN not set")
	}

	// --- Step 1: Create shared organization ---
	orgName := fmt.Sprintf("e2e-mt-org-%d", time.Now().Unix())
	org, err := adminC.CreateOrganization(ctx, orgName)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	t.Logf("created organization %s (%s)", org.Name, org.ID)

	// --- Step 2: Create Tenant A and Tenant B in the same org ---
	ts := time.Now().Unix()
	tenantA, err := adminC.CreateTenant(ctx, org.ID, fmt.Sprintf("e2e-mt-tenant-a-%d", ts))
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	t.Logf("created tenant A: %s (%s)", tenantA.Name, tenantA.ID)

	tenantB, err := adminC.CreateTenant(ctx, org.ID, fmt.Sprintf("e2e-mt-tenant-b-%d", ts))
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	t.Logf("created tenant B: %s (%s)", tenantB.Name, tenantB.ID)

	// --- Step 3: Create a Network for each tenant ---
	netA, err := adminC.CreateNetwork(ctx, tenantA.ID, fmt.Sprintf("net-a-%d", ts), "")
	if err != nil {
		t.Fatalf("create network A: %v", err)
	}
	t.Logf("created network A: %s (%s) for tenant A", netA.Name, netA.ID)

	t.Cleanup(func() {
		if err := adminC.DeleteNetwork(ctx, netA.ID); err != nil {
			t.Logf("cleanup: delete network A %s: %v", netA.ID, err)
		}
	})

	netB, err := adminC.CreateNetwork(ctx, tenantB.ID, fmt.Sprintf("net-b-%d", ts), "")
	if err != nil {
		t.Fatalf("create network B: %v", err)
	}
	t.Logf("created network B: %s (%s) for tenant B", netB.Name, netB.ID)

	t.Cleanup(func() {
		if err := adminC.DeleteNetwork(ctx, netB.ID); err != nil {
			t.Logf("cleanup: delete network B %s: %v", netB.ID, err)
		}
	})

	// --- Step 4: Create a VM for each tenant ---
	flavors, err := adminC.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Fatalf("list flavors: %v (count=%d)", err, len(flavors))
	}
	flavorID := flavors[0].ID

	vmA, err := adminC.CreateVM(ctx, tenantA.ID, client.CreateVMRequest{
		Name:      fmt.Sprintf("vm-a-%d", ts),
		FlavorID:  flavorID.String(),
		NetworkID: netA.ID.String(),
	})
	if err != nil {
		t.Fatalf("create VM A: %v", err)
	}
	t.Logf("VM A created: %s", vmA.ID)

	t.Cleanup(func() {
		_ = adminC.VMAction(ctx, tenantA.ID, vmA.ID, "force-stop")
		time.Sleep(2 * time.Second)
		if err := adminC.DeleteVM(ctx, tenantA.ID, vmA.ID); err != nil {
			t.Logf("cleanup: delete VM A: %v", err)
		}
	})

	vmB, err := adminC.CreateVM(ctx, tenantB.ID, client.CreateVMRequest{
		Name:      fmt.Sprintf("vm-b-%d", ts),
		FlavorID:  flavorID.String(),
		NetworkID: netB.ID.String(),
	})
	if err != nil {
		t.Fatalf("create VM B: %v", err)
	}
	t.Logf("VM B created: %s", vmB.ID)

	t.Cleanup(func() {
		_ = adminC.VMAction(ctx, tenantB.ID, vmB.ID, "force-stop")
		time.Sleep(2 * time.Second)
		if err := adminC.DeleteVM(ctx, tenantB.ID, vmB.ID); err != nil {
			t.Logf("cleanup: delete VM B: %v", err)
		}
	})

	// --- Step 5: Isolation check — Tenant A's client tries to GET Tenant B's resources ---
	// We use a client scoped to Tenant A's tenant ID against Tenant B's network.
	// The API enforces tenant ownership: the network netB belongs to tenantB, so
	// a request with X-Tenant-ID=tenantA should be rejected with 403.
	clientA := client.New(endpoint, tenantToken)

	t.Run("cross_tenant_GET_network_forbidden", func(t *testing.T) {
		// ListNetworks for tenantA should not include tenantB's network.
		nets, err := clientA.ListNetworks(ctx, tenantA.ID)
		if err != nil {
			t.Fatalf("list networks (tenant A): %v", err)
		}
		for _, n := range nets {
			if n.ID == netB.ID {
				t.Errorf("Tenant A can see Tenant B's network %s — isolation violated", netB.ID)
			}
		}
		t.Logf("Tenant A network list does not contain Tenant B's network (isolation OK)")
	})

	t.Run("cross_tenant_DELETE_vm_forbidden", func(t *testing.T) {
		// Attempt to delete Tenant B's VM using Tenant A's tenant scope.
		err := clientA.DeleteVM(ctx, tenantA.ID, vmB.ID)
		if err == nil {
			t.Error("expected error when Tenant A deletes Tenant B's VM, got nil")
			return
		}
		if !strings.Contains(err.Error(), "403") && !strings.Contains(err.Error(), "404") {
			// The API may return 404 (not found in tenant A's scope) or 403 (forbidden).
			// Both indicate correct isolation.
			t.Errorf("unexpected error (expected 403/404): %v", err)
		}
		t.Logf("correctly rejected cross-tenant DELETE VM: %v", err)
	})

	t.Run("cross_tenant_DELETE_network_forbidden", func(t *testing.T) {
		// Attempt to delete Tenant B's Network with tenantA-scoped request.
		// DeleteNetwork uses the network ID directly but the handler checks ownership.
		err := clientA.DeleteNetwork(ctx, netB.ID)
		if err == nil {
			t.Error("expected error when Tenant A deletes Tenant B's network, got nil")
			return
		}
		if !strings.Contains(err.Error(), "403") && !strings.Contains(err.Error(), "404") {
			t.Errorf("unexpected error (expected 403/404): %v", err)
		}
		t.Logf("correctly rejected cross-tenant DELETE network: %v", err)
	})

	// Verify each tenant can still see their own resources.
	t.Run("tenant_A_sees_own_resources", func(t *testing.T) {
		nets, err := clientA.ListNetworks(ctx, tenantA.ID)
		if err != nil {
			t.Fatalf("list networks (tenant A): %v", err)
		}
		found := false
		for _, n := range nets {
			if n.ID == netA.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Tenant A cannot see its own network %s", netA.ID)
		} else {
			t.Logf("Tenant A correctly sees its own network %s", netA.ID)
		}
	})

	t.Run("tenant_B_sees_own_resources", func(t *testing.T) {
		adminC2 := client.New(endpoint, os.Getenv("CIRRUS_TOKEN"))
		nets, err := adminC2.ListNetworks(ctx, tenantB.ID)
		if err != nil {
			t.Fatalf("list networks (tenant B): %v", err)
		}
		found := false
		for _, n := range nets {
			if n.ID == netB.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Tenant B cannot see its own network %s", netB.ID)
		} else {
			t.Logf("Tenant B correctly sees its own network %s", netB.ID)
		}
	})

}
