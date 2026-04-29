package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/api"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/jobqueue"
)

// mockComputeSvc is a stub implementation of compute.Service for handler tests.
type mockComputeSvc struct {
	migrateErr        error
	migrateCalled     bool
	migrateCalledWith *uuid.UUID // the targetHostID passed to MigrateVM (nil = not called)
	migrateVMID       uuid.UUID
}

func (m *mockComputeSvc) RegisterHandlers(_ *jobqueue.Dispatcher) {}

func (m *mockComputeSvc) CreateVM(_ context.Context, _ compute.CreateVMSpec) (*compute.CreateVMResponse, error) {
	return nil, nil
}

func (m *mockComputeSvc) GetVM(_ context.Context, _, _ uuid.UUID) (*compute.VM, error) {
	return nil, compute.ErrNotFound
}

func (m *mockComputeSvc) ListVMsPage(_ context.Context, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]compute.VM, error) {
	return nil, nil
}

func (m *mockComputeSvc) DeleteVM(_ context.Context, _, _ uuid.UUID) (*compute.DeleteVMResponse, error) {
	return nil, nil
}

func (m *mockComputeSvc) StartVM(_ context.Context, _, _ uuid.UUID) error  { return nil }
func (m *mockComputeSvc) StopVM(_ context.Context, _, _ uuid.UUID) error   { return nil }
func (m *mockComputeSvc) ForceStopVM(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockComputeSvc) RebootVM(_ context.Context, _, _ uuid.UUID) error { return nil }

func (m *mockComputeSvc) RepairVM(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockComputeSvc) MigrateVM(_ context.Context, _, vmID uuid.UUID, targetHostID *uuid.UUID) error {
	m.migrateCalled = true
	m.migrateVMID = vmID
	m.migrateCalledWith = targetHostID
	return m.migrateErr
}

func (m *mockComputeSvc) FailoverVM(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockComputeSvc) ListVMsByHost(_ context.Context, _ uuid.UUID) ([]compute.VM, error) {
	return nil, nil
}

// vmTestRouter builds a router wired with the given compute service.
func vmTestRouter(svc compute.Service) http.Handler {
	return api.NewRouter(nil, slog.Default(), &testAuthn{}, &testAuthz{}, nil, nil, nil, nil, nil, nil, nil, svc, nil, nil, false)
}

// vmActionReq sends a JSON POST to /api/v1/vms/{vmID}/actions with optional X-Tenant-ID.
func vmActionReq(handler http.Handler, vmID uuid.UUID, body any, tenantID *uuid.UUID) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms/"+vmID.String()+"/actions", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	if tenantID != nil {
		req.Header.Set("X-Tenant-ID", tenantID.String())
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestVMAction_Migrate_NoTargetHost verifies that a migrate action without
// target_host_id is accepted and MigrateVM is called with nil targetHostID.
func TestVMAction_Migrate_NoTargetHost(t *testing.T) {
	svc := &mockComputeSvc{}
	r := vmTestRouter(svc)

	vmID := uuid.New()
	tid := uuid.New()

	w := vmActionReq(r, vmID, map[string]string{"action": "migrate"}, &tid)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body.String())
	}
	if !svc.migrateCalled {
		t.Fatal("expected MigrateVM to be called, but it was not")
	}
	if svc.migrateVMID != vmID {
		t.Fatalf("MigrateVM called with vmID %s, want %s", svc.migrateVMID, vmID)
	}
	if svc.migrateCalledWith != nil {
		t.Fatalf("expected targetHostID=nil, got %s", *svc.migrateCalledWith)
	}
}

// TestVMAction_Migrate_WithTargetHost verifies that target_host_id is correctly
// parsed and forwarded to MigrateVM.
func TestVMAction_Migrate_WithTargetHost(t *testing.T) {
	svc := &mockComputeSvc{}
	r := vmTestRouter(svc)

	vmID := uuid.New()
	hostID := uuid.New()
	tid := uuid.New()

	body := map[string]string{
		"action":         "migrate",
		"target_host_id": hostID.String(),
	}
	w := vmActionReq(r, vmID, body, &tid)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body.String())
	}
	if svc.migrateCalledWith == nil {
		t.Fatal("expected targetHostID to be set, got nil")
	}
	if *svc.migrateCalledWith != hostID {
		t.Fatalf("MigrateVM called with hostID %s, want %s", *svc.migrateCalledWith, hostID)
	}
}

// TestVMAction_Migrate_InvalidTargetHostID verifies that a non-UUID target_host_id → 400.
func TestVMAction_Migrate_InvalidTargetHostID(t *testing.T) {
	svc := &mockComputeSvc{}
	r := vmTestRouter(svc)

	vmID := uuid.New()
	tid := uuid.New()

	body := map[string]string{
		"action":         "migrate",
		"target_host_id": "not-a-uuid",
	}
	w := vmActionReq(r, vmID, body, &tid)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestVMAction_Migrate_VMNotRunning verifies that ErrConflict from MigrateVM
// results in a 409 invalid state response.
func TestVMAction_Migrate_VMNotRunning(t *testing.T) {
	svc := &mockComputeSvc{migrateErr: compute.ErrConflict}
	r := vmTestRouter(svc)

	vmID := uuid.New()
	tid := uuid.New()

	w := vmActionReq(r, vmID, map[string]string{"action": "migrate"}, &tid)

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != "ERR_INVALID_STATE" {
		t.Fatalf("want code ERR_INVALID_STATE, got %v", resp["code"])
	}
}

// TestVMAction_Migrate_NoTenant verifies that omitting X-Tenant-ID → 400.
func TestVMAction_Migrate_NoTenant(t *testing.T) {
	svc := &mockComputeSvc{}
	r := vmTestRouter(svc)

	vmID := uuid.New()
	w := vmActionReq(r, vmID, map[string]string{"action": "migrate"}, nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}
