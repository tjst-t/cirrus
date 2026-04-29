package api_test

// drs_handler_acceptance_test.go — acceptance test for S025-2 DRS admin endpoints.
//
// Spins up just enough state to call the handler and verifies the run + status
// round-trip using the same fakeDRSRunner used in the unit tests.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	controllerdrs "github.com/tjst-t/cirrus/internal/controller/drs"
	"github.com/tjst-t/cirrus/internal/scheduler"
)

// TestAC_S025_2_DRSAdminEndpoints verifies the DRS admin run+status round-trip.
func TestAC_S025_2_DRSAdminEndpoints(t *testing.T) {
	azID := uuid.New()
	expectedReport := &controllerdrs.RunReport{
		StartedAt:  time.Now().Add(-2 * time.Second),
		FinishedAt: time.Now().Add(-time.Second),
		AZResults: []scheduler.DRSResult{
			{
				AZID:           azID,
				StddevBefore:   0.25,
				EvaluatedHosts: 3,
				PlannedMoves: []scheduler.MigrationPlan{
					{
						VMID:       uuid.New(),
						TenantID:   uuid.New(),
						SrcHostID:  uuid.New(),
						DestHostID: uuid.New(),
						AZID:       azID,
					},
				},
			},
		},
		Successes: 1,
		Failures:  0,
	}

	runner := &fakeDRSRunner{
		runReport:  expectedReport,
		lastReport: nil, // initially nil
	}

	handler := drsTestRouter(runner, true, 300)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("run returns 200 with report", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/v1/admin/drs/run", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)

		if body["successes"] != float64(1) {
			t.Errorf("expected successes=1, got %v", body["successes"])
		}
		azResults := body["az_results"].([]any)
		if len(azResults) != 1 {
			t.Fatalf("expected 1 az_result, got %d", len(azResults))
		}
		az := azResults[0].(map[string]any)
		if az["az_id"] != azID.String() {
			t.Errorf("expected az_id=%s, got %v", azID, az["az_id"])
		}
		if az["planned_count"] != float64(1) {
			t.Errorf("expected planned_count=1, got %v", az["planned_count"])
		}
		if body["duration_ms"] == nil {
			t.Error("expected duration_ms field")
		}
	})

	// Simulate runner storing lastReport after RunOnce (as the real runner does)
	runner.lastReport = expectedReport

	t.Run("status returns enabled and interval", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/v1/admin/drs/status", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)

		if body["enabled"] != true {
			t.Errorf("expected enabled=true, got %v", body["enabled"])
		}
		if body["interval_seconds"] != float64(300) {
			t.Errorf("expected interval_seconds=300, got %v", body["interval_seconds"])
		}
		if body["last_report"] == nil {
			t.Fatal("expected last_report to be non-null after run")
		}
		report := body["last_report"].(map[string]any)
		if report["successes"] != float64(1) {
			t.Errorf("expected successes=1 in last_report, got %v", report["successes"])
		}
	})

	t.Run("status returns null last_report before any run", func(t *testing.T) {
		// Use a fresh runner with no last report.
		freshRunner := &fakeDRSRunner{lastReport: nil}
		freshHandler := drsTestRouter(freshRunner, false, 600)
		freshSrv := httptest.NewServer(freshHandler)
		defer freshSrv.Close()

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, freshSrv.URL+"/api/v1/admin/drs/status", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)

		if body["last_report"] != nil {
			t.Errorf("expected last_report=null, got %v", body["last_report"])
		}
		if body["enabled"] != false {
			t.Errorf("expected enabled=false, got %v", body["enabled"])
		}
	})

	t.Run("run returns 409 when already in progress", func(t *testing.T) {
		busyRunner := &fakeDRSRunner{}
		busyRunner.inFlight.Store(1)
		busyHandler := drsTestRouter(busyRunner, true, 300)
		busySrv := httptest.NewServer(busyHandler)
		defer busySrv.Close()

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, busySrv.URL+"/api/v1/admin/drs/run", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.StatusCode)
		}
	})
}
