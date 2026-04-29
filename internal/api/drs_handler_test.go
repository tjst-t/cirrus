package api_test

// drs_handler_test.go — table-driven HTTP tests for the DRS admin endpoints.
//
// Uses a fakeDRSRunner satisfying api.DRSRunner — no database, no real runner.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/api"
	controllerdrs "github.com/tjst-t/cirrus/internal/controller/drs"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/scheduler"
)

// fakeDRSRunner satisfies api.DRSRunner for tests.
type fakeDRSRunner struct {
	inFlight   atomic.Int32
	lastReport *controllerdrs.RunReport
	runErr     error
	runReport  *controllerdrs.RunReport
}

func (f *fakeDRSRunner) IsRunning() bool         { return f.inFlight.Load() == 1 }
func (f *fakeDRSRunner) TryAcquire() bool        { return f.inFlight.CompareAndSwap(0, 1) }
func (f *fakeDRSRunner) Release()                { f.inFlight.Store(0) }
func (f *fakeDRSRunner) LastReport() *controllerdrs.RunReport { return f.lastReport }
func (f *fakeDRSRunner) RunOnce(_ context.Context) (*controllerdrs.RunReport, error) {
	return f.runReport, f.runErr
}

// drsDenyAllAuthz always returns Deny (for 403 tests on DRS endpoints).
type drsDenyAllAuthz struct{}

func (a *drsDenyAllAuthz) Authorize(_ context.Context, _ *identity.User, _ identity.Action, _ identity.Resource) (identity.Decision, error) {
	return identity.Deny, nil
}

// drsTestRouter creates a router with the given DRS runner.
// testAuthz (defined in topology_handler_test.go) always returns Allow.
func drsTestRouter(runner api.DRSRunner, enabled bool, intervalSecs int) http.Handler {
	return api.NewRouter(
		nil, slog.Default(), &testAuthn{}, &testAuthz{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		false,
		api.NewRouterOptions{
			DRSRunner:       runner,
			DRSEnabled:      enabled,
			DRSIntervalSecs: intervalSecs,
		},
	)
}

// drsTestRouterDenyAuthz creates a router that always denies authorization.
func drsTestRouterDenyAuthz(runner api.DRSRunner) http.Handler {
	return api.NewRouter(
		nil, slog.Default(), &testAuthn{}, &drsDenyAllAuthz{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		false,
		api.NewRouterOptions{DRSRunner: runner},
	)
}

// --- POST /api/v1/admin/drs/run ---

func TestDRSRun_Returns200WithReport(t *testing.T) {
	azID := uuid.New()
	runner := &fakeDRSRunner{
		runReport: &controllerdrs.RunReport{
			StartedAt:  time.Now().Add(-time.Second),
			FinishedAt: time.Now(),
			AZResults: []scheduler.DRSResult{
				{AZID: azID, StddevBefore: 0.3, EvaluatedHosts: 3},
			},
			Successes: 1,
			Failures:  0,
		},
	}
	h := drsTestRouter(runner, true, 300)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/drs/run", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["successes"] != float64(1) {
		t.Errorf("expected successes=1, got %v", body["successes"])
	}
	azResults, ok := body["az_results"].([]any)
	if !ok || len(azResults) != 1 {
		t.Errorf("expected 1 az_result, got %v", body["az_results"])
	}
}

func TestDRSRun_Returns409WhenInProgress(t *testing.T) {
	runner := &fakeDRSRunner{}
	runner.inFlight.Store(1) // simulate in-progress run
	h := drsTestRouter(runner, true, 300)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/drs/run", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}

	var errBody map[string]any
	json.NewDecoder(w.Body).Decode(&errBody)
	if errBody["message"] != "drs run already in progress" {
		t.Errorf("unexpected error message: %v", errBody["message"])
	}
}

func TestDRSRun_Returns403ForNonAdmin(t *testing.T) {
	runner := &fakeDRSRunner{}
	h := drsTestRouterDenyAuthz(runner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/drs/run", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDRSRun_Returns401WhenNoToken(t *testing.T) {
	runner := &fakeDRSRunner{}
	h := drsTestRouter(runner, true, 300)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/drs/run", nil)
	// No Authorization header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// --- GET /api/v1/admin/drs/status ---

func TestDRSStatus_Returns200WithNullLastReport(t *testing.T) {
	runner := &fakeDRSRunner{lastReport: nil}
	h := drsTestRouter(runner, true, 300)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/drs/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["last_report"] != nil {
		t.Errorf("expected last_report=null, got %v", body["last_report"])
	}
	if body["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", body["enabled"])
	}
	if body["interval_seconds"] != float64(300) {
		t.Errorf("expected interval_seconds=300, got %v", body["interval_seconds"])
	}
}

func TestDRSStatus_Returns200WithLastReport(t *testing.T) {
	azID := uuid.New()
	runner := &fakeDRSRunner{
		lastReport: &controllerdrs.RunReport{
			StartedAt:  time.Now().Add(-2 * time.Second),
			FinishedAt: time.Now().Add(-time.Second),
			AZResults: []scheduler.DRSResult{
				{AZID: azID, StddevBefore: 0.2, EvaluatedHosts: 2},
			},
			Successes: 2,
			Failures:  0,
		},
	}
	h := drsTestRouter(runner, false, 600)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/drs/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["last_report"] == nil {
		t.Fatal("expected non-null last_report")
	}
	report := body["last_report"].(map[string]any)
	if report["successes"] != float64(2) {
		t.Errorf("expected successes=2, got %v", report["successes"])
	}
	if body["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", body["enabled"])
	}
	if body["interval_seconds"] != float64(600) {
		t.Errorf("expected interval_seconds=600, got %v", body["interval_seconds"])
	}
}

func TestDRSStatus_Returns403ForNonAdmin(t *testing.T) {
	runner := &fakeDRSRunner{}
	h := drsTestRouterDenyAuthz(runner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/drs/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDRSStatus_Returns401WhenNoToken(t *testing.T) {
	runner := &fakeDRSRunner{}
	h := drsTestRouter(runner, true, 300)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/drs/status", nil)
	// No Authorization header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// --- Round-trip: run then status ---

func TestDRSRunStatus_RoundTrip(t *testing.T) {
	azID := uuid.New()
	report := &controllerdrs.RunReport{
		StartedAt:  time.Now().Add(-time.Second),
		FinishedAt: time.Now(),
		AZResults: []scheduler.DRSResult{
			{AZID: azID, StddevBefore: 0.25, EvaluatedHosts: 4},
		},
		Successes: 3,
		Failures:  1,
		Errors:    []string{"migration failed for vm-xyz"},
	}
	runner := &fakeDRSRunner{runReport: report}
	h := drsTestRouter(runner, true, 300)

	// POST /run
	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/drs/run", nil)
	runReq.Header.Set("Authorization", "Bearer test-token")
	runW := httptest.NewRecorder()
	h.ServeHTTP(runW, runReq)
	if runW.Code != http.StatusOK {
		t.Fatalf("run: expected 200, got %d", runW.Code)
	}

	// Simulate the runner storing lastReport after RunOnce
	runner.lastReport = report

	// GET /status
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/drs/status", nil)
	statusReq.Header.Set("Authorization", "Bearer test-token")
	statusW := httptest.NewRecorder()
	h.ServeHTTP(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", statusW.Code)
	}

	var statusBody map[string]any
	json.NewDecoder(statusW.Body).Decode(&statusBody)
	if statusBody["last_report"] == nil {
		t.Fatal("expected last_report to be populated after run")
	}
}
