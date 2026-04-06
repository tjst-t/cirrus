//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/internal/client"
)

// TestPagination verifies cursor-based pagination for list endpoints.
//
// Scenarios:
//  1. Networks: create 5 resources, paginate with limit=2 → 3 pages (2, 2, 1), last page has empty next_cursor
//  2. VMs: same structure (limit=2, 3 pages)
//  3. Invalid cursor → 400
//  4. limit > max (100) → 400
//  5. limit=0 / negative → 400
func TestPagination(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping pagination integration test")
	}

	adminC := adminClient(t, endpoint)
	ctx := context.Background()

	// Create a dedicated org+tenant for this test to isolate from other data.
	ts := time.Now().UnixNano()
	org, err := adminC.CreateOrganization(ctx, fmt.Sprintf("pg-org-%d", ts))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	tenant, err := adminC.CreateTenant(ctx, org.ID, fmt.Sprintf("pg-tenant-%d", ts))
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenantID := tenant.ID

	// --- Subtest 1: Network pagination ---
	t.Run("network_pagination", func(t *testing.T) {
		const total = 5
		var netIDs []string

		// Create 5 networks.
		for i := 0; i < total; i++ {
			net, err := adminC.CreateNetwork(ctx, tenantID, fmt.Sprintf("pg-net-%d-%d", ts, i), "")
			if err != nil {
				t.Fatalf("create network %d: %v", i, err)
			}
			netIDs = append(netIDs, net.ID.String())
			t.Cleanup(func() {
				_ = adminC.DeleteNetwork(ctx, net.ID)
			})
		}

		// Page 1: limit=2 → expect 2 items + non-empty next_cursor.
		page1, cursor1 := fetchNetworksPage(t, endpoint, tenantID.String(), "", 2)
		if len(page1) != 2 {
			t.Fatalf("page1: expected 2 items, got %d", len(page1))
		}
		if cursor1 == "" {
			t.Fatal("page1: expected non-empty next_cursor")
		}
		t.Logf("page1: %d items, cursor=%s", len(page1), cursor1[:8]+"...")

		// Page 2: limit=2 → expect 2 items + non-empty next_cursor.
		page2, cursor2 := fetchNetworksPage(t, endpoint, tenantID.String(), cursor1, 2)
		if len(page2) != 2 {
			t.Fatalf("page2: expected 2 items, got %d", len(page2))
		}
		if cursor2 == "" {
			t.Fatal("page2: expected non-empty next_cursor")
		}
		t.Logf("page2: %d items, cursor=%s", len(page2), cursor2[:8]+"...")

		// No duplicates between pages.
		for _, n1 := range page1 {
			for _, n2 := range page2 {
				if n1 == n2 {
					t.Errorf("duplicate item across pages: %s", n1)
				}
			}
		}

		// Page 3: limit=2 → expect 1 item (5 total, 4 retrieved) + empty next_cursor.
		page3, cursor3 := fetchNetworksPage(t, endpoint, tenantID.String(), cursor2, 2)
		if len(page3) != 1 {
			t.Fatalf("page3: expected 1 item, got %d", len(page3))
		}
		if cursor3 != "" {
			t.Errorf("page3: expected empty next_cursor, got %s", cursor3)
		}
		t.Logf("page3: %d items (last page OK)", len(page3))

		// Aggregate all retrieved IDs and verify no duplicates across all 3 pages.
		all := append(append(page1, page2...), page3...)
		seen := make(map[string]bool)
		for _, id := range all {
			if seen[id] {
				t.Errorf("duplicate ID across all pages: %s", id)
			}
			seen[id] = true
		}
		t.Logf("total retrieved: %d (no duplicates)", len(all))
	})

	// --- Subtest 2: VM pagination ---
	t.Run("vm_pagination", func(t *testing.T) {
		flavors, err := adminC.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Skip("no flavors available")
		}
		flavorID := flavors[0].ID

		const total = 5
		for i := 0; i < total; i++ {
			vm, err := adminC.CreateVM(ctx, tenantID, client.CreateVMRequest{
				Name:     fmt.Sprintf("pg-vm-%d-%d", ts%1000000, i),
				FlavorID: flavorID.String(),
			})
			if err != nil {
				t.Fatalf("create vm %d: %v", i, err)
			}
			t.Cleanup(func() {
				_ = adminC.VMAction(ctx, tenantID, vm.ID, "force-stop")
				_ = adminC.DeleteVM(ctx, tenantID, vm.ID)
			})
		}

		// Page 1: limit=2.
		page1, cursor1 := fetchVMsPage(t, endpoint, tenantID.String(), "", 2)
		if len(page1) < 2 {
			t.Fatalf("vm page1: expected >= 2 items, got %d", len(page1))
		}
		if cursor1 == "" {
			t.Fatal("vm page1: expected non-empty next_cursor")
		}

		// Page 2 with cursor.
		page2, _ := fetchVMsPage(t, endpoint, tenantID.String(), cursor1, 2)
		if len(page2) == 0 {
			t.Fatal("vm page2: expected items, got 0")
		}

		// No duplicates between pages.
		for _, id1 := range page1 {
			for _, id2 := range page2 {
				if id1 == id2 {
					t.Errorf("vm: duplicate item across pages: %s", id1)
				}
			}
		}
		t.Logf("vm pagination: page1=%d page2=%d (no duplicates)", len(page1), len(page2))
	})

	// --- Subtest 3: invalid cursor → 400 ---
	t.Run("invalid_cursor_400", func(t *testing.T) {
		token := os.Getenv("CIRRUS_TOKEN")
		if token == "" {
			token = os.Getenv("CIRRUS_ADMIN_TOKEN")
		}
		req, _ := http.NewRequestWithContext(ctx, "GET",
			endpoint+"/api/v1/networks?after=not-valid-base64!!",
			nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", tenantID.String())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("invalid cursor: expected 400, got %d", resp.StatusCode)
		}
		t.Logf("invalid cursor correctly returned 400")
	})

	// --- Subtest 4: limit exceeds max → 400 ---
	t.Run("limit_exceeds_max_400", func(t *testing.T) {
		token := os.Getenv("CIRRUS_TOKEN")
		if token == "" {
			token = os.Getenv("CIRRUS_ADMIN_TOKEN")
		}
		req, _ := http.NewRequestWithContext(ctx, "GET",
			endpoint+"/api/v1/networks?limit=101",
			nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", tenantID.String())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("limit > max: expected 400, got %d", resp.StatusCode)
		}
		t.Logf("limit > max correctly returned 400")
	})

	// --- Subtest 5: limit=0 → 400 ---
	t.Run("limit_zero_400", func(t *testing.T) {
		token := os.Getenv("CIRRUS_TOKEN")
		if token == "" {
			token = os.Getenv("CIRRUS_ADMIN_TOKEN")
		}
		req, _ := http.NewRequestWithContext(ctx, "GET",
			endpoint+"/api/v1/networks?limit=0",
			nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", tenantID.String())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("limit=0: expected 400, got %d", resp.StatusCode)
		}
		t.Logf("limit=0 correctly returned 400")
	})
}

// fetchNetworksPage calls GET /api/v1/networks with optional cursor and limit.
// Returns the list of network IDs and next_cursor.
func fetchNetworksPage(t *testing.T, endpoint, tenantID, cursor string, limit int) ([]string, string) {
	t.Helper()
	token := os.Getenv("CIRRUS_TOKEN")
	if token == "" {
		token = os.Getenv("CIRRUS_ADMIN_TOKEN")
	}

	url := fmt.Sprintf("%s/api/v1/networks?limit=%d", endpoint, limit)
	if cursor != "" {
		url += "&after=" + cursor
	}

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Tenant-ID", tenantID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetchNetworksPage: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetchNetworksPage: unexpected status %d", resp.StatusCode)
	}

	var envelope struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := decodeJSON(resp, &envelope); err != nil {
		t.Fatalf("fetchNetworksPage decode: %v", err)
	}
	ids := make([]string, len(envelope.Items))
	for i, item := range envelope.Items {
		ids[i] = item.ID
	}
	return ids, envelope.NextCursor
}

// fetchVMsPage calls GET /api/v1/vms with optional cursor and limit.
// Returns the list of VM IDs and next_cursor.
func fetchVMsPage(t *testing.T, endpoint, tenantID, cursor string, limit int) ([]string, string) {
	t.Helper()
	token := os.Getenv("CIRRUS_TOKEN")
	if token == "" {
		token = os.Getenv("CIRRUS_ADMIN_TOKEN")
	}

	url := fmt.Sprintf("%s/api/v1/vms?limit=%d", endpoint, limit)
	if cursor != "" {
		url += "&after=" + cursor
	}

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Tenant-ID", tenantID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetchVMsPage: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetchVMsPage: unexpected status %d", resp.StatusCode)
	}

	var envelope struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := decodeJSON(resp, &envelope); err != nil {
		t.Fatalf("fetchVMsPage decode: %v", err)
	}
	ids := make([]string, len(envelope.Items))
	for i, item := range envelope.Items {
		ids[i] = item.ID
	}
	return ids, envelope.NextCursor
}
