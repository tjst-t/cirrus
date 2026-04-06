//go:build integration

package integration

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

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/compute"
)

// simLibvirtURLs returns all libvirt-sim management API base URLs.
// Reads LIBVIRT_SIM_URLS (comma-separated) for multiple worker sims, or
// LIBVIRT_SIM_URL for a single URL; defaults to http://localhost:8100.
func simLibvirtURLs() []string {
	if v := os.Getenv("LIBVIRT_SIM_URLS"); v != "" {
		var urls []string
		for _, u := range strings.Split(v, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				urls = append(urls, u)
			}
		}
		if len(urls) > 0 {
			return urls
		}
	}
	if u := os.Getenv("LIBVIRT_SIM_URL"); u != "" {
		return []string{u}
	}
	return []string{"http://localhost:8100"}
}

// simFindAndDestroyDomain searches all libvirt-sim instances for a domain with the given
// UUID and destroys it. Each worker container runs its own sim instance, so we check all
// known sim URLs. The cirrus host UUID does not map 1:1 to the sim host ID.
func simFindAndDestroyDomain(t *testing.T, _ string, domainUUID string) {
	t.Helper()
	simURLs := simLibvirtURLs()
	for _, simURL := range simURLs {
		if simTryDestroyDomain(t, simURL, domainUUID) {
			return
		}
	}
	t.Fatalf("sim: domain %s not found on any sim instance (checked %v)", domainUUID, simURLs)
}

// simTryDestroyDomain searches one sim instance for the domain and destroys it if found.
// Returns true if the domain was found and destroyed.
func simTryDestroyDomain(t *testing.T, simURL, domainUUID string) bool {
	t.Helper()
	// List all sim hosts in this instance.
	resp, err := http.Get(simURL + "/sim/hosts")
	if err != nil {
		return false // sim instance unreachable
	}
	defer resp.Body.Close()
	var hosts []struct {
		HostID string `json:"host_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hosts); err != nil {
		return false
	}
	for _, h := range hosts {
		domainsURL := fmt.Sprintf("%s/sim/hosts/%s/domains", simURL, h.HostID)
		dr, err := http.Get(domainsURL)
		if err != nil {
			continue
		}
		var domains []struct {
			UUID string `json:"uuid"`
		}
		_ = json.NewDecoder(dr.Body).Decode(&domains)
		dr.Body.Close()
		for _, d := range domains {
			if strings.EqualFold(d.UUID, domainUUID) {
				destroyURL := fmt.Sprintf("%s/sim/hosts/%s/domains/%s/destroy", simURL, h.HostID, domainUUID)
				req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, destroyURL, nil)
				dr2, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("sim destroy: %v", err)
				}
				dr2.Body.Close()
				if dr2.StatusCode != http.StatusNoContent {
					t.Fatalf("sim destroy: unexpected status %d", dr2.StatusCode)
				}
				t.Logf("sim: force-destroyed domain %s on sim=%s host=%s", domainUUID, simURL, h.HostID)
				return true
			}
		}
	}
	return false
}

// simListHostDomains returns the domain list for a host from libvirt-sim.
type simDomainInfo struct {
	UUID   string `json:"uuid"`
	HostID string `json:"host_id"`
	State  int32  `json:"state"`
}

// waitForDriftEvent polls drift_events until an event matching the given criteria appears,
// or until timeout.
func waitForDriftEvent(t *testing.T, env *TestEnv, resourceID, eventType string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		err := env.DB.QueryRow(context.Background(),
			`SELECT count(*) FROM drift_events
			 WHERE resource_id = $1 AND type = $2`,
			resourceID, eventType,
		).Scan(&count)
		if err != nil {
			t.Fatalf("query drift_events: %v", err)
		}
		if count > 0 {
			t.Logf("drift event detected: resource_id=%s type=%s", resourceID, eventType)
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timeout: no drift_event with resource_id=%s type=%s after %s", resourceID, eventType, timeout)
}

// TestReconcileHeartbeat_DriftEvent verifies that when a VM is force-stopped
// externally via cirrus-sim, HeartbeatReconciler detects the state mismatch and
// records a drift_event.
//
// Prerequisites (environment variables):
//
//	CIRRUS_ENDPOINT    — controller API base URL
//	CIRRUS_TOKEN       — bearer token
//	CIRRUS_TENANT_ID   — tenant UUID
//	TEST_DB_DSN        — direct DB DSN (default: postgres://cirrus:cirrus@localhost:5432/cirrus)
//	LIBVIRT_SIM_URL    — libvirt-sim management API URL (default: http://localhost:8100)
func TestReconcileHeartbeat_DriftEvent(t *testing.T) {
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping reconcile heartbeat integration test")
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

	env := NewTestEnv(t)
	c := client.New(endpoint, token)
	ctx := context.Background()

	// Step 1: find a flavor and create a VM.
	flavors, err := c.ListFlavors(ctx)
	if err != nil || len(flavors) == 0 {
		t.Skipf("no flavors available: %v", err)
	}

	vmName := fmt.Sprintf("test-reconcile-hb-%d", time.Now().Unix())
	vm, err := c.CreateVM(ctx, tenantID, client.CreateVMRequest{
		Name:     vmName,
		FlavorID: flavors[0].ID.String(),
	})
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	t.Logf("created VM %s (%s)", vm.Name, vm.ID)

	t.Cleanup(func() {
		_ = c.VMAction(ctx, tenantID, vm.ID, "force-stop")
		time.Sleep(2 * time.Second)
		_ = c.DeleteVM(ctx, tenantID, vm.ID)
	})

	// Step 2: wait for VM to reach running state.
	waitForVMStatus(t, c, ctx, tenantID, vm.ID, compute.VMStatusRunning, 60*time.Second)
	t.Logf("VM is running: %s", vm.ID)

	// Step 3: discover host_id from DB.
	var hostID uuid.UUID
	err = env.DB.QueryRow(ctx,
		`SELECT host_id FROM vms WHERE id = $1`, vm.ID,
	).Scan(&hostID)
	if err != nil {
		t.Fatalf("get host_id from DB: %v", err)
	}
	t.Logf("VM host_id: %s", hostID)

	// Step 4: force-destroy the domain via cirrus-sim (bypasses Cirrus API).
	// The DB still thinks the VM is "running".
	simFindAndDestroyDomain(t, "", vm.ID.String())
	t.Logf("domain destroyed in sim; DB still shows running")

	// Step 5: wait for HeartbeatReconciler to fire a drift event.
	// The reconciler runs on each heartbeat (typically every few seconds in dev).
	// We look for either state_mismatch or expected_missing.
	t.Logf("waiting for drift event (up to 30s)...")
	deadline := time.Now().Add(30 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		var count int
		err := env.DB.QueryRow(ctx,
			`SELECT count(*) FROM drift_events
			 WHERE resource_id = $1
			   AND type IN ('state_mismatch', 'expected_missing')
			   AND resource = 'vm'`,
			vm.ID.String(),
		).Scan(&count)
		if err != nil {
			t.Fatalf("query drift_events: %v", err)
		}
		if count > 0 {
			found = true
			t.Logf("drift event recorded for VM %s", vm.ID)
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !found {
		t.Fatalf("timeout: no drift event for VM %s after 30s", vm.ID)
	}

	// Step 6: verify the drift_events row has the expected fields.
	var layer, evType, severity, detectedBy string
	err = env.DB.QueryRow(ctx,
		`SELECT layer, type, severity, detected_by
		 FROM drift_events
		 WHERE resource_id = $1
		   AND type IN ('state_mismatch', 'expected_missing')
		   AND resource = 'vm'
		 ORDER BY created_at DESC LIMIT 1`,
		vm.ID.String(),
	).Scan(&layer, &evType, &severity, &detectedBy)
	if err != nil {
		t.Fatalf("scan drift_event: %v", err)
	}
	t.Logf("drift_event: layer=%s type=%s severity=%s detected_by=%s", layer, evType, severity, detectedBy)

	if layer != "compute" {
		t.Errorf("expected layer=compute, got %s", layer)
	}
	if detectedBy != "heartbeat_reconciler" {
		t.Errorf("expected detected_by=heartbeat_reconciler, got %s", detectedBy)
	}

	// Step 7: wait for VM status to transition to error (auto-heal).
	// Note: auto-heal only applies for expected_missing or crashed states.
	// For shutoff (state_mismatch), the action may be alert-only, so we check
	// with a shorter timeout and treat non-error as acceptable.
	t.Logf("checking if VM transitions to error state (auto-heal)...")
	errorDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(errorDeadline) {
		currentVM, err := c.GetVM(ctx, tenantID, vm.ID)
		if err != nil {
			t.Logf("get vm (may be healing): %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if currentVM.Status == compute.VMStatusError {
			t.Logf("VM transitioned to error state (auto-heal confirmed)")
			return
		}
		time.Sleep(2 * time.Second)
	}

	// If not in error, log current status — drift detection is the primary assertion.
	currentVM, _ := c.GetVM(ctx, tenantID, vm.ID)
	if currentVM != nil {
		t.Logf("VM final status: %s (drift detection passed; auto-heal may be alert-only for shutoff)", currentVM.Status)
	}
}

// simForceStopRequest is used to call internal sim APIs.
func simPostJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("simPostJSON marshal: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = &bytes.Buffer{}
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, reqBody)
	if err != nil {
		t.Fatalf("simPostJSON create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("simPostJSON do: %v", err)
	}
	return resp
}
