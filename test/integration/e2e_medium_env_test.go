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

// TestE2EMediumEnvSmoke is a smoke test for the cirrus-sim medium environment.
//
// Prerequisites:
//   - CIRRUS_SIM_ENV=medium (cirrus-sim was started with the medium environment YAML)
//   - CIRRUS_ENDPOINT, CIRRUS_TOKEN, CIRRUS_TENANT_ID are set
//
// The test verifies:
//  1. Host count >= 100 (medium env has 400 total hosts across two sites).
//  2. Tenant count >= 20 (medium env preloads 20 tenants under a default org).
//  3. A new VM can reach "running" state within 60 seconds.
func TestE2EMediumEnvSmoke(t *testing.T) {
	// This test only makes sense when running against a medium environment.
	if os.Getenv("CIRRUS_SIM_ENV") == "" {
		t.Skip("CIRRUS_SIM_ENV not set; skipping medium env smoke test")
	}

	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping medium env smoke test")
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

	// --- Check 1: Host count >= 100 ---
	t.Run("host_count_100_plus", func(t *testing.T) {
		hosts, err := c.ListHosts(ctx)
		if err != nil {
			t.Fatalf("list hosts: %v", err)
		}
		t.Logf("total hosts: %d", len(hosts))
		if len(hosts) < 100 {
			t.Errorf("expected >= 100 hosts in medium env, got %d", len(hosts))
		}
	})

	// --- Check 2: Tenant count >= 20 ---
	t.Run("tenant_count_20_plus", func(t *testing.T) {
		orgs, err := c.ListOrganizations(ctx)
		if err != nil {
			t.Fatalf("list organizations: %v", err)
		}

		totalTenants := 0
		for _, org := range orgs {
			tenants, err := c.ListTenants(ctx, org.ID)
			if err != nil {
				t.Logf("list tenants for org %s: %v (skipping)", org.ID, err)
				continue
			}
			totalTenants += len(tenants)
		}
		t.Logf("total tenants across all orgs: %d", totalTenants)
		if totalTenants < 20 {
			t.Errorf("expected >= 20 tenants in medium env, got %d", totalTenants)
		}
	})

	// --- Check 3: VM creation reaches running within 60 seconds ---
	t.Run("vm_start_within_60s", func(t *testing.T) {
		flavors, err := c.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Fatalf("list flavors: %v (count=%d)", err, len(flavors))
		}
		flavorID := flavors[0].ID
		t.Logf("using flavor: %s (%s)", flavors[0].Name, flavorID)

		vmName := fmt.Sprintf("medium-smoke-%d", time.Now().Unix())
		vm, err := c.CreateVM(ctx, tenantID, client.CreateVMRequest{
			Name:     vmName,
			FlavorID: flavorID.String(),
		})
		if err != nil {
			t.Fatalf("create vm: %v", err)
		}
		t.Logf("VM created: %s (initial status=%s)", vm.ID, vm.Status)

		t.Cleanup(func() {
			_ = c.VMAction(ctx, tenantID, vm.ID, "force-stop")
			time.Sleep(2 * time.Second)
			if err := c.DeleteVM(ctx, tenantID, vm.ID); err != nil {
				t.Logf("cleanup: delete vm %s: %v", vm.ID, err)
			}
		})

		start := time.Now()
		vm = waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)
		elapsed := time.Since(start)
		t.Logf("VM %s reached running in %s", vm.ID, elapsed)

		if elapsed > 60*time.Second {
			t.Errorf("VM took %s to reach running state (expected <= 60s in medium env)", elapsed)
		}
	})
}
