package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tjst-t/cirrus/test/sim/storage/internal/state"
)

func setupTestServer() (*http.ServeMux, *state.Store) {
	store := state.NewStore(nil)
	h := NewManagementHandler(store, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, store
}

func TestHandleAddBackend(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]any{
				"backend_id":          "ceph-pool-ssd",
				"total_capacity_gb":   512000,
				"total_iops":          500000,
				"capabilities":        []string{"ssd", "snapshot"},
				"overprovision_ratio": 2.0,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "empty id",
			body:       map[string]any{"total_capacity_gb": 100},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, _ := setupTestServer()

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/sim/backends", bytes.NewReader(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandleAddBackendDuplicate(t *testing.T) {
	mux, _ := setupTestServer()

	body, _ := json.Marshal(map[string]any{
		"backend_id": "b1", "total_capacity_gb": 100, "total_iops": 1000,
	})

	req := httptest.NewRequest("POST", "/sim/backends", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first add: status = %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/sim/backends", bytes.NewReader(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("duplicate add: status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestHandleListBackends(t *testing.T) {
	mux, store := setupTestServer()
	if err := store.AddBackend(context.Background(), state.Backend{BackendID: "b1", TotalCapacityGB: 100, TotalIOPS: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddBackend(context.Background(), state.Backend{BackendID: "b2", TotalCapacityGB: 200, TotalIOPS: 2000}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/sim/backends", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var backends []state.Backend
	if err := json.NewDecoder(w.Body).Decode(&backends); err != nil {
		t.Fatal(err)
	}
	if len(backends) != 2 {
		t.Errorf("got %d backends, want 2", len(backends))
	}
}

func TestHandleSetBackendState(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		state      string
		wantStatus int
	}{
		{name: "draining", id: "b1", state: "draining", wantStatus: http.StatusOK},
		{name: "not found", id: "missing", state: "draining", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, store := setupTestServer()
			if err := store.AddBackend(context.Background(), state.Backend{BackendID: "b1", TotalCapacityGB: 100, TotalIOPS: 1000}); err != nil {
				t.Fatal(err)
			}

			body, _ := json.Marshal(map[string]string{"state": tt.state})
			req := httptest.NewRequest("PUT", "/sim/backends/"+tt.id+"/state", bytes.NewReader(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleGetStats(t *testing.T) {
	mux, store := setupTestServer()
	if err := store.AddBackend(context.Background(), state.Backend{BackendID: "b1", TotalCapacityGB: 1000, TotalIOPS: 1000, OverprovisionRatio: 2.0}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/sim/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var stats state.Stats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.BackendCount != 1 {
		t.Errorf("backend_count = %d, want 1", stats.BackendCount)
	}
	if stats.VolumeCount != 1 {
		t.Errorf("volume_count = %d, want 1", stats.VolumeCount)
	}
}

func TestHandleReset(t *testing.T) {
	mux, store := setupTestServer()
	if err := store.AddBackend(context.Background(), state.Backend{BackendID: "b1", TotalCapacityGB: 100, TotalIOPS: 1000}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/sim/reset", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	stats := store.GetStats(context.Background())
	if stats.BackendCount != 0 {
		t.Errorf("backend_count after reset = %d", stats.BackendCount)
	}
}

func TestHandleSetConfig(t *testing.T) {
	mux, store := setupTestServer()

	body, _ := json.Marshal(state.SimConfig{DefaultLatencyMs: 10})
	req := httptest.NewRequest("POST", "/sim/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	cfg := store.GetConfig(context.Background())
	if cfg.DefaultLatencyMs != 10 {
		t.Errorf("latency = %d, want 10", cfg.DefaultLatencyMs)
	}
}
