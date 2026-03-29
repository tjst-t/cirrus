// Package handler provides HTTP handlers for the Cirrus Storage API.
package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/tjst-t/cirrus/test/sim/storage/internal/state"
)

// StorageHandler handles Cirrus Storage API requests under /api/v1/.
type StorageHandler struct {
	store  *state.Store
	logger *slog.Logger
}

// NewStorageHandler creates a new StorageHandler.
func NewStorageHandler(store *state.Store, logger *slog.Logger) *StorageHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &StorageHandler{store: store, logger: logger}
}

// RegisterRoutes registers all storage API routes on the given mux.
func (h *StorageHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/backend/info", h.handleBackendInfo)
	mux.HandleFunc("GET /api/v1/backend/health", h.handleBackendHealth)
	mux.HandleFunc("POST /api/v1/volumes", h.handleCreateVolume)
	mux.HandleFunc("GET /api/v1/volumes/{volume_id}", h.handleGetVolume)
	mux.HandleFunc("GET /api/v1/volumes", h.handleListVolumes)
	mux.HandleFunc("DELETE /api/v1/volumes/{volume_id}", h.handleDeleteVolume)
	mux.HandleFunc("PUT /api/v1/volumes/{volume_id}/extend", h.handleExtendVolume)
	mux.HandleFunc("POST /api/v1/volumes/{volume_id}/export", h.handleExportVolume)
	mux.HandleFunc("DELETE /api/v1/volumes/{volume_id}/export", h.handleUnexportVolume)

	// Snapshot routes
	mux.HandleFunc("POST /api/v1/volumes/{volume_id}/snapshots", h.handleCreateSnapshot)
	mux.HandleFunc("GET /api/v1/volumes/{volume_id}/snapshots", h.handleListSnapshots)
	mux.HandleFunc("DELETE /api/v1/snapshots/{snapshot_id}", h.handleDeleteSnapshot)

	// Clone routes
	mux.HandleFunc("POST /api/v1/snapshots/{snapshot_id}/clone", h.handleCloneFromSnapshot)

	// Flatten routes
	mux.HandleFunc("POST /api/v1/volumes/{volume_id}/flatten", h.handleStartFlatten)
	mux.HandleFunc("GET /api/v1/operations/{operation_id}", h.handleGetOperation)

	// Migration routes
	mux.HandleFunc("POST /api/v1/migrations", h.handleStartMigration)
	mux.HandleFunc("GET /api/v1/migrations/{migration_id}", h.handleGetMigration)
	mux.HandleFunc("DELETE /api/v1/migrations/{migration_id}", h.handleCancelMigration)
}

func (h *StorageHandler) handleBackendInfo(w http.ResponseWriter, r *http.Request) {
	backendID := r.Header.Get("X-Backend-Id")
	if backendID == "" {
		writeError(w, http.StatusBadRequest, "X-Backend-Id header is required")
		return
	}

	b, err := h.store.GetBackend(r.Context(), backendID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("backend not found: %s", backendID))
		return
	}

	writeJSON(w, http.StatusOK, b)
}

func (h *StorageHandler) handleBackendHealth(w http.ResponseWriter, r *http.Request) {
	backendID := r.Header.Get("X-Backend-Id")
	if backendID == "" {
		writeError(w, http.StatusBadRequest, "X-Backend-Id header is required")
		return
	}

	_, err := h.store.GetBackend(r.Context(), backendID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("backend not found: %s", backendID))
		return
	}

	cfg := h.store.GetConfig(r.Context())

	resp := map[string]any{
		"healthy":    true,
		"latency_ms": cfg.DefaultLatencyMs,
		"last_check": time.Now().UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

type createVolumeRequest struct {
	VolumeID        string            `json:"volume_id"`
	SizeGB          int64             `json:"size_gb"`
	ThinProvisioned bool              `json:"thin_provisioned"`
	QoSPolicy       *state.QoSPolicy  `json:"qos_policy,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

func (h *StorageHandler) handleCreateVolume(w http.ResponseWriter, r *http.Request) {
	backendID := r.Header.Get("X-Backend-Id")
	if backendID == "" {
		writeError(w, http.StatusBadRequest, "X-Backend-Id header is required")
		return
	}

	var req createVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	vol := state.Volume{
		VolumeID:        req.VolumeID,
		BackendID:       backendID,
		SizeGB:          req.SizeGB,
		ThinProvisioned: req.ThinProvisioned,
		QoSPolicy:       req.QoSPolicy,
		Metadata:        req.Metadata,
	}

	created, err := h.store.CreateVolume(r.Context(), vol)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *StorageHandler) handleGetVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")
	v, err := h.store.GetVolume(r.Context(), volumeID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *StorageHandler) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	backendID := r.Header.Get("X-Backend-Id")
	stateFilter := state.VolumeState(r.URL.Query().Get("state"))

	volumes := h.store.ListVolumes(r.Context(), backendID, stateFilter)
	writeJSON(w, http.StatusOK, volumes)
}

func (h *StorageHandler) handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")
	err := h.store.DeleteVolume(r.Context(), volumeID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type extendVolumeRequest struct {
	NewSizeGB int64 `json:"new_size_gb"`
}

func (h *StorageHandler) handleExtendVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")

	var req extendVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	v, err := h.store.ExtendVolume(r.Context(), volumeID, req.NewSizeGB)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type exportVolumeRequest struct {
	HostID   string `json:"host_id"`
	Protocol string `json:"protocol"`
}

func (h *StorageHandler) handleExportVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")

	var req exportVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	v, err := h.store.ExportVolume(r.Context(), volumeID, req.HostID, req.Protocol)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *StorageHandler) handleUnexportVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")

	v, err := h.store.UnexportVolume(r.Context(), volumeID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type createSnapshotRequest struct {
	SnapshotID string            `json:"snapshot_id"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func (h *StorageHandler) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")

	var req createSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	snap, err := h.store.CreateSnapshot(r.Context(), volumeID, req.SnapshotID, req.Metadata)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, snap)
}

func (h *StorageHandler) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")

	snaps, err := h.store.ListSnapshots(r.Context(), volumeID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, snaps)
}

func (h *StorageHandler) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID := r.PathValue("snapshot_id")

	err := h.store.DeleteSnapshot(r.Context(), snapshotID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type cloneFromSnapshotRequest struct {
	VolumeID string            `json:"volume_id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (h *StorageHandler) handleCloneFromSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID := r.PathValue("snapshot_id")

	var req cloneFromSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	vol, err := h.store.CloneFromSnapshot(r.Context(), snapshotID, req.VolumeID, req.Metadata)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, vol)
}

func (h *StorageHandler) handleStartFlatten(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volume_id")

	op, err := h.store.StartFlatten(r.Context(), volumeID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"operation_id":         op.OperationID,
		"state":               op.State,
		"progress_percent":    op.ProgressPercent,
		"estimated_duration_ms": op.EstimatedRemainingMs,
	})
}

func (h *StorageHandler) handleGetOperation(w http.ResponseWriter, r *http.Request) {
	opID := r.PathValue("operation_id")

	op, err := h.store.GetOperation(r.Context(), opID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, op)
}

type startMigrationRequest struct {
	VolumeID      string `json:"volume_id"`
	DestBackendURL string `json:"dest_backend_url"`
	DestBackendID string `json:"dest_backend_id"`
}

func (h *StorageHandler) handleStartMigration(w http.ResponseWriter, r *http.Request) {
	var req startMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	mig, err := h.store.StartMigration(r.Context(), req.VolumeID, req.DestBackendID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, mig)
}

func (h *StorageHandler) handleGetMigration(w http.ResponseWriter, r *http.Request) {
	migID := r.PathValue("migration_id")

	mig, err := h.store.GetMigration(r.Context(), migID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, mig)
}

func (h *StorageHandler) handleCancelMigration(w http.ResponseWriter, r *http.Request) {
	migID := r.PathValue("migration_id")

	err := h.store.CancelMigration(r.Context(), migID)
	if err != nil {
		code := errorToHTTPStatus(err)
		writeError(w, code, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// errorToHTTPStatus maps state errors to HTTP status codes.
func errorToHTTPStatus(err error) int {
	switch {
	case containsErr(err, state.ErrVolumeNotFound), containsErr(err, state.ErrBackendNotFound):
		return http.StatusNotFound
	case containsErr(err, state.ErrSnapshotNotFound), containsErr(err, state.ErrOperationNotFound), containsErr(err, state.ErrMigrationNotFound):
		return http.StatusNotFound
	case containsErr(err, state.ErrVolumeInUse):
		return http.StatusNotAcceptable // 406
	case containsErr(err, state.ErrVolumeHasSnapshots):
		return http.StatusConflict // 409
	case containsErr(err, state.ErrSnapshotHasClones):
		return http.StatusConflict // 409
	case containsErr(err, state.ErrVolumeAlreadyExported), containsErr(err, state.ErrVolumeNotExported):
		return http.StatusConflict
	case containsErr(err, state.ErrShrinkNotAllowed), containsErr(err, state.ErrVolumeNoParent):
		return http.StatusBadRequest
	case containsErr(err, state.ErrEmptySnapshotID):
		return http.StatusBadRequest
	case containsErr(err, state.ErrInsufficientCapacity):
		return http.StatusInsufficientStorage // 507
	case containsErr(err, state.ErrBackendNotActive):
		return http.StatusServiceUnavailable // 503
	case containsErr(err, state.ErrVolumeExists), containsErr(err, state.ErrBackendExists):
		return http.StatusConflict
	case containsErr(err, state.ErrSnapshotExists):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func containsErr(err, target error) bool {
	for e := err; e != nil; e = unwrapSingle(e) {
		if e == target {
			return true
		}
	}
	return false
}

func unwrapSingle(err error) error {
	u, ok := err.(interface{ Unwrap() error })
	if !ok {
		return nil
	}
	return u.Unwrap()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().Error("failed to encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		slog.Default().Error("failed to encode error response", "error", err)
	}
}
