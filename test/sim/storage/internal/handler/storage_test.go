package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/test/sim/storage/internal/state"
)

func setupTestServer() (*http.ServeMux, *state.Store) {
	store := state.NewStore(nil)
	h := NewStorageHandler(store, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, store
}

func addBackend(t *testing.T, store *state.Store, id string) {
	t.Helper()
	err := store.AddBackend(context.Background(), state.Backend{
		BackendID:          id,
		TotalCapacityGB:    10000,
		TotalIOPS:          500000,
		Capabilities:       []string{"ssd", "snapshot"},
		OverprovisionRatio: 2.0,
	})
	if err != nil {
		t.Fatalf("add backend: %v", err)
	}
}

func TestHandleBackendInfo(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")

	tests := []struct {
		name       string
		backendID  string
		wantStatus int
	}{
		{name: "success", backendID: "b1", wantStatus: http.StatusOK},
		{name: "missing header", backendID: "", wantStatus: http.StatusBadRequest},
		{name: "not found", backendID: "missing", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/backend/info", nil)
			if tt.backendID != "" {
				req.Header.Set("X-Backend-Id", tt.backendID)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleBackendHealth(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")

	req := httptest.NewRequest("GET", "/api/v1/backend/health", nil)
	req.Header.Set("X-Backend-Id", "b1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["healthy"] != true {
		t.Errorf("healthy = %v, want true", resp["healthy"])
	}
}

func TestHandleCreateVolume(t *testing.T) {
	tests := []struct {
		name       string
		backendID  string
		body       map[string]any
		wantStatus int
	}{
		{
			name:      "success",
			backendID: "b1",
			body: map[string]any{
				"volume_id":        "vol-001",
				"size_gb":          100,
				"thin_provisioned": true,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing backend header",
			backendID:  "",
			body:       map[string]any{"volume_id": "vol-002", "size_gb": 100},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "backend not found",
			backendID: "missing",
			body: map[string]any{
				"volume_id": "vol-003",
				"size_gb":   100,
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, store := setupTestServer()
			addBackend(t, store, "b1")

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/v1/volumes", bytes.NewReader(body))
			if tt.backendID != "" {
				req.Header.Set("X-Backend-Id", tt.backendID)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantStatus == http.StatusCreated {
				var vol state.Volume
				if err := json.NewDecoder(w.Body).Decode(&vol); err != nil {
					t.Fatal(err)
				}
				if vol.State != state.VolumeAvailable {
					t.Errorf("state = %s, want available", vol.State)
				}
				if vol.ConsumedGB != 0 {
					t.Errorf("consumed_gb = %d, want 0", vol.ConsumedGB)
				}
			}
		})
	}
}

func TestHandleGetVolume(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")
	if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "vol-1", BackendID: "b1", SizeGB: 50, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		volumeID   string
		wantStatus int
	}{
		{name: "found", volumeID: "vol-1", wantStatus: http.StatusOK},
		{name: "not found", volumeID: "missing", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/volumes/"+tt.volumeID, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleListVolumes(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")
	if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "v2", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExportVolume(context.Background(), "v1", "host-1", "rbd"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{name: "all", query: "", wantCount: 2},
		{name: "in_use", query: "?state=in_use", wantCount: 1},
		{name: "available", query: "?state=available", wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/volumes"+tt.query, nil)
			req.Header.Set("X-Backend-Id", "b1")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			var vols []state.Volume
			if err := json.NewDecoder(w.Body).Decode(&vols); err != nil {
				t.Fatal(err)
			}
			if len(vols) != tt.wantCount {
				t.Errorf("got %d volumes, want %d", len(vols), tt.wantCount)
			}
		})
	}
}

func TestHandleDeleteVolume(t *testing.T) {
	tests := []struct {
		name       string
		volumeID   string
		exported   bool
		wantStatus int
	}{
		{name: "success", volumeID: "v1", exported: false, wantStatus: http.StatusNoContent},
		{name: "in use", volumeID: "v1", exported: true, wantStatus: http.StatusNotAcceptable},
		{name: "not found", volumeID: "missing", exported: false, wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, store := setupTestServer()
			addBackend(t, store, "b1")
			if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}
			if tt.exported {
				if _, err := store.ExportVolume(context.Background(), "v1", "host-1", "rbd"); err != nil {
					t.Fatal(err)
				}
			}

			req := httptest.NewRequest("DELETE", "/api/v1/volumes/"+tt.volumeID, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandleDeleteVolumeWithSnapshots(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")
	if _, err := store.CreateVolume(context.Background(), state.Volume{
		VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true,
		Snapshots: []string{"snap-1"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/volumes/v1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestHandleExtendVolume(t *testing.T) {
	tests := []struct {
		name       string
		newSizeGB  int64
		wantStatus int
	}{
		{name: "success", newSizeGB: 200, wantStatus: http.StatusOK},
		{name: "shrink", newSizeGB: 50, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, store := setupTestServer()
			addBackend(t, store, "b1")
			if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}

			body, _ := json.Marshal(map[string]any{"new_size_gb": tt.newSizeGB})
			req := httptest.NewRequest("PUT", "/api/v1/volumes/v1/extend", bytes.NewReader(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleExportUnexport(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")
	if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}

	// Export
	body, _ := json.Marshal(map[string]string{"host_id": "host-1", "protocol": "rbd"})
	req := httptest.NewRequest("POST", "/api/v1/volumes/v1/export", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("export status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var vol state.Volume
	if err := json.NewDecoder(w.Body).Decode(&vol); err != nil {
		t.Fatal(err)
	}
	if vol.State != state.VolumeInUse {
		t.Errorf("state = %s, want in_use", vol.State)
	}

	// Unexport
	req = httptest.NewRequest("DELETE", "/api/v1/volumes/v1/export", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexport status = %d, want %d", w.Code, http.StatusOK)
	}

	if err := json.NewDecoder(w.Body).Decode(&vol); err != nil {
		t.Fatal(err)
	}
	if vol.State != state.VolumeAvailable {
		t.Errorf("state = %s, want available", vol.State)
	}
}

func TestHandleCreateSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		volumeID   string
		body       map[string]any
		wantStatus int
	}{
		{
			name:     "success",
			volumeID: "vol-1",
			body: map[string]any{
				"snapshot_id": "snap-001",
				"metadata":    map[string]string{"description": "before-upgrade"},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:     "volume not found",
			volumeID: "missing",
			body: map[string]any{
				"snapshot_id": "snap-002",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:     "empty snapshot id",
			volumeID: "vol-1",
			body: map[string]any{
				"snapshot_id": "",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, store := setupTestServer()
			addBackend(t, store, "b1")
			if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "vol-1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/v1/volumes/"+tt.volumeID+"/snapshots", bytes.NewReader(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantStatus == http.StatusCreated {
				var snap state.Snapshot
				if err := json.NewDecoder(w.Body).Decode(&snap); err != nil {
					t.Fatal(err)
				}
				if snap.SnapshotID != "snap-001" {
					t.Errorf("snapshot_id = %s, want snap-001", snap.SnapshotID)
				}
				if snap.VolumeID != "vol-1" {
					t.Errorf("volume_id = %s, want vol-1", snap.VolumeID)
				}
				if snap.SizeGB != 100 {
					t.Errorf("size_gb = %d, want 100", snap.SizeGB)
				}
				if snap.State != "available" {
					t.Errorf("state = %s, want available", snap.State)
				}
			}
		})
	}
}

func TestHandleListSnapshots(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")
	ctx := context.Background()
	if _, err := store.CreateVolume(ctx, state.Volume{VolumeID: "vol-1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSnapshot(ctx, "vol-1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSnapshot(ctx, "vol-1", "snap-002", nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/v1/volumes/vol-1/snapshots", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var snaps []state.Snapshot
	if err := json.NewDecoder(w.Body).Decode(&snaps); err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Errorf("got %d snapshots, want 2", len(snaps))
	}
}

func TestHandleDeleteSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		snapshotID string
		hasClones  bool
		wantStatus int
	}{
		{name: "success", snapshotID: "snap-001", hasClones: false, wantStatus: http.StatusNoContent},
		{name: "not found", snapshotID: "missing", hasClones: false, wantStatus: http.StatusNotFound},
		{name: "has clones", snapshotID: "snap-001", hasClones: true, wantStatus: http.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, store := setupTestServer()
			addBackend(t, store, "b1")
			ctx := context.Background()
			if _, err := store.CreateVolume(ctx, state.Volume{VolumeID: "vol-1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateSnapshot(ctx, "vol-1", "snap-001", nil); err != nil {
				t.Fatal(err)
			}
			if tt.hasClones {
				if _, err := store.CloneFromSnapshot(ctx, "snap-001", "clone-001", nil); err != nil {
					t.Fatal(err)
				}
			}

			req := httptest.NewRequest("DELETE", "/api/v1/snapshots/"+tt.snapshotID, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandleCloneFromSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		snapshotID string
		body       map[string]any
		wantStatus int
	}{
		{
			name:       "success",
			snapshotID: "snap-001",
			body:       map[string]any{"volume_id": "clone-001", "metadata": map[string]string{"tenant": "t1"}},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "snapshot not found",
			snapshotID: "missing",
			body:       map[string]any{"volume_id": "clone-002"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, store := setupTestServer()
			addBackend(t, store, "b1")
			ctx := context.Background()
			if _, err := store.CreateVolume(ctx, state.Volume{VolumeID: "vol-1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateSnapshot(ctx, "vol-1", "snap-001", nil); err != nil {
				t.Fatal(err)
			}

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/v1/snapshots/"+tt.snapshotID+"/clone", bytes.NewReader(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantStatus == http.StatusCreated {
				var vol state.Volume
				if err := json.NewDecoder(w.Body).Decode(&vol); err != nil {
					t.Fatal(err)
				}
				if vol.ParentSnapshotID != "snap-001" {
					t.Errorf("parent_snapshot_id = %s, want snap-001", vol.ParentSnapshotID)
				}
				if vol.VolumeID != "clone-001" {
					t.Errorf("volume_id = %s, want clone-001", vol.VolumeID)
				}
			}
		})
	}
}

func TestHandleFlatten(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")
	ctx := context.Background()

	// Set fast flatten for test
	store.SetConfig(ctx, state.SimConfig{DefaultLatencyMs: 2, FlattenPerGBMs: 1})

	if _, err := store.CreateVolume(ctx, state.Volume{VolumeID: "vol-1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSnapshot(ctx, "vol-1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CloneFromSnapshot(ctx, "snap-001", "clone-001", nil); err != nil {
		t.Fatal(err)
	}

	// Start flatten
	req := httptest.NewRequest("POST", "/api/v1/volumes/clone-001/flatten", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var flattenResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&flattenResp); err != nil {
		t.Fatal(err)
	}
	opID, ok := flattenResp["operation_id"].(string)
	if !ok || opID == "" {
		t.Fatal("operation_id not returned")
	}
	if flattenResp["state"] != string(state.OperationRunning) {
		t.Errorf("state = %v, want running", flattenResp["state"])
	}

	// Wait for completion and check operation
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req = httptest.NewRequest("GET", "/api/v1/operations/"+opID, nil)
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("get operation status = %d", w.Code)
		}

		var op state.Operation
		if err := json.NewDecoder(w.Body).Decode(&op); err != nil {
			t.Fatal(err)
		}
		if op.State == state.OperationCompleted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify volume after flatten
	req = httptest.NewRequest("GET", "/api/v1/volumes/clone-001", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var vol state.Volume
	if err := json.NewDecoder(w.Body).Decode(&vol); err != nil {
		t.Fatal(err)
	}
	if vol.ParentSnapshotID != "" {
		t.Errorf("parent_snapshot_id = %s, want empty", vol.ParentSnapshotID)
	}
	if vol.ConsumedGB != vol.SizeGB {
		t.Errorf("consumed_gb = %d, want %d", vol.ConsumedGB, vol.SizeGB)
	}
}

func TestHandleFlattenNoParent(t *testing.T) {
	mux, store := setupTestServer()
	addBackend(t, store, "b1")
	if _, err := store.CreateVolume(context.Background(), state.Volume{VolumeID: "vol-1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/v1/volumes/vol-1/flatten", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleGetOperationNotFound(t *testing.T) {
	mux, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/operations/missing", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCapacityExceeded(t *testing.T) {
	mux, store := setupTestServer()
	// Small backend: 100GB, ratio 1.0
	if err := store.AddBackend(context.Background(), state.Backend{
		BackendID:          "small",
		TotalCapacityGB:    100,
		TotalIOPS:          1000,
		Capabilities:       []string{"ssd"},
		OverprovisionRatio: 1.0,
	}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{
		"volume_id":        "big-vol",
		"size_gb":          200,
		"thin_provisioned": true,
	})
	req := httptest.NewRequest("POST", "/api/v1/volumes", bytes.NewReader(body))
	req.Header.Set("X-Backend-Id", "small")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInsufficientStorage {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInsufficientStorage)
	}
}
