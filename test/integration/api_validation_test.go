//go:build integration

package integration

// TestAPIValidation verifies that the API correctly rejects malformed requests
// with 400 Bad Request before reaching business logic.
//
// Covers S021-2-1: 全 API エンドポイントのバリデーション強化

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/internal/client"
)

func TestAPIValidation(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping API validation integration test")
	}
	token := os.Getenv("CIRRUS_TOKEN")
	if token == "" {
		token = os.Getenv("CIRRUS_ADMIN_TOKEN")
	}
	if token == "" {
		t.Fatal("CIRRUS_TOKEN not set")
	}

	// Create a tenant to scope requests.
	adminC := adminClient(t, endpoint)
	ctx := context.Background()
	ts := time.Now().UnixNano()
	org, err := adminC.CreateOrganization(ctx, fmt.Sprintf("val-org-%d", ts))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	tenant, err := adminC.CreateTenant(ctx, org.ID, fmt.Sprintf("val-tenant-%d", ts))
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenantID := tenant.ID.String()

	post := func(t *testing.T, path string, body any, extraHeaders map[string]string) *http.Response {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST", endpoint+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		return resp
	}

	withTenant := map[string]string{"X-Tenant-ID": tenantID}

	// ── VM creation validation ──────────────────────────────────────────────

	t.Run("vm_missing_flavor_id", func(t *testing.T) {
		resp := post(t, "/api/v1/vms", map[string]any{
			"name": "valid-name",
			// flavor_id omitted
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("missing flavor_id → %d OK", resp.StatusCode)
	})

	t.Run("vm_invalid_flavor_uuid", func(t *testing.T) {
		resp := post(t, "/api/v1/vms", map[string]any{
			"name":      "valid-name",
			"flavor_id": "not-a-uuid",
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("invalid flavor UUID → %d OK", resp.StatusCode)
	})

	t.Run("vm_empty_name", func(t *testing.T) {
		flavors, err := adminC.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Skip("no flavors available")
		}
		resp := post(t, "/api/v1/vms", map[string]any{
			"name":      "",
			"flavor_id": flavors[0].ID.String(),
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("empty name → %d OK", resp.StatusCode)
	})

	t.Run("vm_name_too_long", func(t *testing.T) {
		flavors, err := adminC.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Skip("no flavors available")
		}
		resp := post(t, "/api/v1/vms", map[string]any{
			"name":      strings.Repeat("a", 64), // max is 63
			"flavor_id": flavors[0].ID.String(),
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("name too long → %d OK", resp.StatusCode)
	})

	t.Run("vm_name_invalid_chars", func(t *testing.T) {
		flavors, err := adminC.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Skip("no flavors available")
		}
		resp := post(t, "/api/v1/vms", map[string]any{
			"name":      "UPPERCASE_NAME",
			"flavor_id": flavors[0].ID.String(),
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("uppercase/underscore name → %d OK", resp.StatusCode)
	})

	t.Run("vm_name_starts_with_hyphen", func(t *testing.T) {
		flavors, err := adminC.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Skip("no flavors available")
		}
		resp := post(t, "/api/v1/vms", map[string]any{
			"name":      "-starts-with-hyphen",
			"flavor_id": flavors[0].ID.String(),
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("name starting with hyphen → %d OK", resp.StatusCode)
	})

	t.Run("vm_missing_tenant_header", func(t *testing.T) {
		flavors, err := adminC.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Skip("no flavors available")
		}
		resp := post(t, "/api/v1/vms", map[string]any{
			"name":      "valid-name",
			"flavor_id": flavors[0].ID.String(),
		}, nil) // no X-Tenant-ID
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("missing X-Tenant-ID → %d OK", resp.StatusCode)
	})

	t.Run("vm_invalid_action", func(t *testing.T) {
		// Create a VM first.
		flavors, err := adminC.ListFlavors(ctx)
		if err != nil || len(flavors) == 0 {
			t.Skip("no flavors available")
		}
		vm, err := adminC.CreateVM(ctx, tenant.ID, client.CreateVMRequest{
			Name:     fmt.Sprintf("val-vm-%d", ts%1000000),
			FlavorID: flavors[0].ID.String(),
		})
		if err != nil {
			t.Fatalf("create vm: %v", err)
		}
		t.Cleanup(func() {
			_ = adminC.VMAction(ctx, tenant.ID, vm.ID, "force-stop")
			_ = adminC.DeleteVM(ctx, tenant.ID, vm.ID)
		})

		resp := post(t, fmt.Sprintf("/api/v1/vms/%s/actions", vm.ID), map[string]any{
			"action": "fly",
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid action, got %d", resp.StatusCode)
		}
		t.Logf("invalid VM action → %d OK", resp.StatusCode)
	})

	// ── Network creation validation ─────────────────────────────────────────

	t.Run("network_empty_name", func(t *testing.T) {
		resp := post(t, "/api/v1/networks", map[string]any{
			"name": "",
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("network empty name → %d OK", resp.StatusCode)
	})

	t.Run("network_name_too_long", func(t *testing.T) {
		resp := post(t, "/api/v1/networks", map[string]any{
			"name": strings.Repeat("a", 64),
		}, withTenant)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("network name too long → %d OK", resp.StatusCode)
	})

	// ── Host action validation ──────────────────────────────────────────────

	t.Run("host_invalid_vm_id", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(ctx, "GET",
			endpoint+"/api/v1/vms/not-a-uuid",
			nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", tenantID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("invalid vm_id path param: expected 400, got %d", resp.StatusCode)
		}
		t.Logf("invalid vm_id in path → %d OK", resp.StatusCode)
	})

	t.Run("host_invalid_host_action", func(t *testing.T) {
		hosts, err := adminC.ListHosts(ctx)
		if err != nil || len(hosts) == 0 {
			t.Skip("no hosts available")
		}
		resp := post(t, fmt.Sprintf("/api/v1/hosts/%s/actions", hosts[0].ID), map[string]any{
			"action": "invalidaction",
		}, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("invalid host action: expected 400, got %d", resp.StatusCode)
		}
		t.Logf("invalid host action → %d OK", resp.StatusCode)
	})

	// ── Pagination parameter validation ─────────────────────────────────────

	t.Run("pagination_invalid_limit_string", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(ctx, "GET",
			endpoint+"/api/v1/networks?limit=abc",
			nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", tenantID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("limit=abc: expected 400, got %d", resp.StatusCode)
		}
		t.Logf("limit=abc → %d OK", resp.StatusCode)
	})
}
