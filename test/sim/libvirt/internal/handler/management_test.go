package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/rpc"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

func setupTestServer(t *testing.T) (*http.ServeMux, *state.Store) {
	t.Helper()
	store := state.NewStore()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	rpcServer := rpc.NewServer(store, logger)
	mgmt := NewManagement(store, rpcServer, logger)

	mux := http.NewServeMux()
	mgmt.RegisterRoutes(mux)
	return mux, store
}

func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestCreateHost(t *testing.T) {
	mux, _ := setupTestServer(t)

	tests := []struct {
		name       string
		body       CreateHostRequest
		wantStatus int
	}{
		{
			name: "valid host",
			body: CreateHostRequest{
				HostID:             "host-001",
				CPUModel:           "Test CPU",
				CPUSockets:         2,
				CoresPerSocket:     4,
				ThreadsPerCore:     2,
				MemoryMB:           32768,
				CPUOvercommitRatio: 4.0,
				MemOvercommitRatio: 1.5,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing host_id",
			body: CreateHostRequest{
				LibvirtPort: 16510,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing port",
			body: CreateHostRequest{
				HostID: "host-002",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantStatus == http.StatusCreated {
				tt.body.LibvirtPort = findFreePort(t)
			}

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/sim/hosts", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListHosts(t *testing.T) {
	mux, store := setupTestServer(t)

	host := &state.Host{
		HostID:         "host-001",
		LibvirtPort:    16509,
		CPUModel:       "Test CPU",
		CPUSockets:     2,
		CoresPerSocket: 4,
		ThreadsPerCore: 2,
		MemoryMB:       32768,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sim/hosts", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var hosts []state.HostInfo
	if err := json.NewDecoder(rec.Body).Decode(&hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Errorf("hosts count = %d, want 1", len(hosts))
	}
}

func TestGetHost(t *testing.T) {
	mux, store := setupTestServer(t)

	host := &state.Host{
		HostID:         "host-001",
		LibvirtPort:    16509,
		CPUModel:       "Test CPU",
		CPUSockets:     2,
		CoresPerSocket: 4,
		ThreadsPerCore: 2,
		MemoryMB:       32768,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		hostID     string
		wantStatus int
	}{
		{"existing host", "host-001", http.StatusOK},
		{"non-existent host", "host-999", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/sim/hosts/"+tt.hostID, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestUpdateHostState(t *testing.T) {
	mux, store := setupTestServer(t)

	host := &state.Host{
		HostID:      "host-001",
		LibvirtPort: 16509,
		MemoryMB:    32768,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		hostState  string
		wantStatus int
	}{
		{"online", "online", http.StatusOK},
		{"offline", "offline", http.StatusOK},
		{"maintenance", "maintenance", http.StatusOK},
		{"invalid", "invalid", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(UpdateHostStateRequest{State: state.HostState(tt.hostState)})
			req := httptest.NewRequest(http.MethodPut, "/sim/hosts/host-001/state", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGetStats(t *testing.T) {
	mux, store := setupTestServer(t)

	host := &state.Host{
		HostID:             "host-001",
		LibvirtPort:        16509,
		CPUSockets:         2,
		CoresPerSocket:     4,
		ThreadsPerCore:     2,
		MemoryMB:           32768,
		CPUOvercommitRatio: 4.0,
		MemOvercommitRatio: 1.5,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sim/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var stats state.Stats
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.TotalHosts != 1 {
		t.Errorf("total_hosts = %d, want 1", stats.TotalHosts)
	}
}

func TestReset(t *testing.T) {
	mux, store := setupTestServer(t)

	host := &state.Host{
		HostID:      "host-001",
		LibvirtPort: 16509,
		MemoryMB:    32768,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sim/reset", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	hosts := store.ListHosts()
	if len(hosts) != 0 {
		t.Errorf("hosts after reset = %d, want 0", len(hosts))
	}
}

func TestGetMigrationConfig(t *testing.T) {
	mux, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/sim/config/migration", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var cfg state.MigrationConfig
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.PrepareDurationMs != 500 {
		t.Errorf("prepare_duration_ms = %d, want 500", cfg.PrepareDurationMs)
	}
}

func TestUpdateMigrationConfig(t *testing.T) {
	mux, store := setupTestServer(t)

	newCfg := state.MigrationConfig{
		PrepareDurationMs:      100,
		BaseTransferDurationMs: 500,
		PerGBMemoryMs:          200,
		FinishDurationMs:       50,
	}
	body, _ := json.Marshal(newCfg)
	req := httptest.NewRequest(http.MethodPost, "/sim/config/migration", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	got := store.GetMigrationConfig()
	if got != newCfg {
		t.Errorf("migration config = %+v, want %+v", got, newCfg)
	}
}

func TestUpdateMigrationConfigInvalidBody(t *testing.T) {
	mux, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/sim/config/migration", bytes.NewReader([]byte("invalid")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUpdateHostConfig(t *testing.T) {
	mux, store := setupTestServer(t)

	host := &state.Host{
		HostID:             "host-001",
		LibvirtPort:        16509,
		MemoryMB:           32768,
		CPUOvercommitRatio: 1.0,
		MemOvercommitRatio: 1.0,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	cpuRatio := 4.0
	body, _ := json.Marshal(UpdateHostConfigRequest{CPUOvercommitRatio: &cpuRatio})
	req := httptest.NewRequest(http.MethodPost, "/sim/hosts/host-001/config", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	h, _ := store.GetHost("host-001")
	if h.CPUOvercommitRatio != 4.0 {
		t.Errorf("cpu_overcommit_ratio = %f, want 4.0", h.CPUOvercommitRatio)
	}
}
