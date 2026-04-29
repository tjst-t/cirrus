package client_test

// drs_test.go — unit tests for the DRS client methods.
//
// Uses an httptest.Server to exercise DRSRun and DRSStatus without a real
// controller.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tjst-t/cirrus/internal/client"
)

func TestDRSRun_Success(t *testing.T) {
	want := &client.DRSRunReport{
		StartedAt:  "2026-01-01T00:00:00Z",
		FinishedAt: "2026-01-01T00:00:01Z",
		DurationMs: 1000,
		AZResults: []client.DRSAZResult{
			{AZID: "az-1", StddevBefore: 0.3, PlannedCount: 2, EvaluatedHosts: 4},
		},
		Successes: 2,
		Failures:  0,
		Errors:    []string{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/drs/run" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-token")
	got, err := c.DRSRun(context.Background())
	if err != nil {
		t.Fatalf("DRSRun: %v", err)
	}
	if got.Successes != want.Successes {
		t.Errorf("Successes: got %d, want %d", got.Successes, want.Successes)
	}
	if len(got.AZResults) != 1 {
		t.Fatalf("AZResults: got %d, want 1", len(got.AZResults))
	}
	if got.AZResults[0].AZID != "az-1" {
		t.Errorf("AZResults[0].AZID: got %q, want az-1", got.AZResults[0].AZID)
	}
}

func TestDRSRun_409Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "ERR_CONFLICT",
			"message": "drs run already in progress",
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-token")
	_, err := c.DRSRun(context.Background())
	if err == nil {
		t.Fatal("expected error for 409")
	}
}

func TestDRSStatus_NoLastReport(t *testing.T) {
	want := &client.DRSStatus{
		Enabled:         true,
		IntervalSeconds: 300,
		LastReport:      nil,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/drs/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-token")
	got, err := c.DRSStatus(context.Background())
	if err != nil {
		t.Fatalf("DRSStatus: %v", err)
	}
	if got.Enabled != true {
		t.Errorf("Enabled: got %v, want true", got.Enabled)
	}
	if got.IntervalSeconds != 300 {
		t.Errorf("IntervalSeconds: got %d, want 300", got.IntervalSeconds)
	}
	if got.LastReport != nil {
		t.Errorf("LastReport: expected nil, got %v", got.LastReport)
	}
}

func TestDRSStatus_WithLastReport(t *testing.T) {
	want := &client.DRSStatus{
		Enabled:         false,
		IntervalSeconds: 600,
		LastReport: &client.DRSRunReport{
			StartedAt:  "2026-01-01T00:00:00Z",
			FinishedAt: "2026-01-01T00:00:02Z",
			DurationMs: 2000,
			Successes:  3,
			Failures:   1,
			Errors:     []string{"migration failed"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-token")
	got, err := c.DRSStatus(context.Background())
	if err != nil {
		t.Fatalf("DRSStatus: %v", err)
	}
	if got.LastReport == nil {
		t.Fatal("expected non-nil LastReport")
	}
	if got.LastReport.Successes != 3 {
		t.Errorf("Successes: got %d, want 3", got.LastReport.Successes)
	}
	if got.LastReport.Failures != 1 {
		t.Errorf("Failures: got %d, want 1", got.LastReport.Failures)
	}
}
