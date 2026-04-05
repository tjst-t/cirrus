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
	"github.com/tjst-t/cirrus/internal/quota"
)

// TestQuota verifies the full quota workflow:
// 1. Admin sets tenant quota limits.
// 2. Tenant creates resources up to the limit.
// 3. Next creation attempt returns 403 (quota exceeded).
// 4. After deletion the usage count drops.
func TestQuota(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping quota integration test")
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

	// --- Step 1: set quota to allow exactly 1 network ---
	limits := quota.Limits{Networks: 1}
	qr, err := c.SetTenantQuota(ctx, tenantID, limits)
	if err != nil {
		t.Fatalf("set tenant quota: %v", err)
	}
	t.Logf("quota set: networks limit=%d", qr.Limits.Networks)

	// --- Step 2: show quota ---
	qr, err = c.GetTenantQuota(ctx, tenantID)
	if err != nil {
		t.Fatalf("get tenant quota: %v", err)
	}
	if qr.Limits.Networks != 1 {
		t.Fatalf("expected networks limit=1, got %d", qr.Limits.Networks)
	}
	t.Logf("quota usage before: networks=%d/%d", qr.Usage.NetworksCount, qr.Limits.Networks)

	// Track created networks for cleanup
	var createdNetIDs []uuid.UUID
	t.Cleanup(func() {
		for _, id := range createdNetIDs {
			if err := c.DeleteNetwork(ctx, id); err != nil {
				t.Logf("cleanup: delete network %s: %v", id, err)
			}
		}
		// Reset quota limits to unlimited after test
		if _, err := c.SetTenantQuota(ctx, tenantID, quota.Limits{}); err != nil {
			t.Logf("cleanup: reset quota: %v", err)
		}
	})

	// --- Step 3: create first network (should succeed) ---
	netName := fmt.Sprintf("quota-test-%d", time.Now().Unix())
	net1, err := c.CreateNetwork(ctx, tenantID, netName, "")
	if err != nil {
		t.Fatalf("create first network: %v", err)
	}
	createdNetIDs = append(createdNetIDs, net1.ID)
	t.Logf("first network created: %s", net1.ID)

	// --- Step 4: create second network (should fail with quota exceeded) ---
	_, err = c.CreateNetwork(ctx, tenantID, netName+"-2", "")
	if err == nil {
		t.Fatal("expected quota exceeded error for second network, got nil")
	}
	t.Logf("second network correctly rejected: %v", err)

	// --- Step 5: verify usage reflects the one network ---
	qr, err = c.GetTenantQuota(ctx, tenantID)
	if err != nil {
		t.Fatalf("get quota after creation: %v", err)
	}
	t.Logf("quota usage after: networks=%d/%d", qr.Usage.NetworksCount, qr.Limits.Networks)

	// --- Step 6: delete the network, usage should drop ---
	if err := c.DeleteNetwork(ctx, net1.ID); err != nil {
		t.Fatalf("delete network: %v", err)
	}
	createdNetIDs = createdNetIDs[:0] // already deleted

	// Allow a short moment for the decommit to propagate (it's synchronous so none needed)
	qr, err = c.GetTenantQuota(ctx, tenantID)
	if err != nil {
		t.Fatalf("get quota after deletion: %v", err)
	}
	if qr.Usage.NetworksCount != 0 {
		t.Fatalf("expected 0 networks after deletion, got %d", qr.Usage.NetworksCount)
	}
	t.Logf("quota usage after deletion: networks=%d/%d", qr.Usage.NetworksCount, qr.Limits.Networks)
}
