package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/snapshot"
)

// SnapshotHandler handles state snapshot API requests.
type SnapshotHandler struct {
	mgr *snapshot.Manager
}

// NewSnapshotHandler creates a new SnapshotHandler.
func NewSnapshotHandler(mgr *snapshot.Manager) *SnapshotHandler {
	return &SnapshotHandler{mgr: mgr}
}

// RegisterRoutes registers snapshot routes on the given mux.
func (h *SnapshotHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/state/snapshot", h.takeSnapshot)
	mux.HandleFunc("POST /api/v1/state/restore/{id}", h.restoreSnapshot)
	mux.HandleFunc("GET /api/v1/state/snapshots", h.listSnapshots)
	mux.HandleFunc("DELETE /api/v1/state/snapshots/{id}", h.deleteSnapshot)
}

func (h *SnapshotHandler) takeSnapshot(w http.ResponseWriter, r *http.Request) {
	snap, err := h.mgr.TakeSnapshot(r.Context())
	if err != nil {
		writeSnapshotError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSnapshotJSON(w, http.StatusCreated, map[string]string{
		"snapshot_id": snap.ID,
		"created_at":  snap.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *SnapshotHandler) restoreSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.mgr.RestoreSnapshot(r.Context(), id); err != nil {
		writeSnapshotError(w, http.StatusNotFound, err.Error())
		return
	}
	writeSnapshotJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

func (h *SnapshotHandler) listSnapshots(w http.ResponseWriter, r *http.Request) {
	list := h.mgr.ListSnapshots(r.Context())
	writeSnapshotJSON(w, http.StatusOK, list)
}

func (h *SnapshotHandler) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.mgr.DeleteSnapshot(r.Context(), id); err != nil {
		writeSnapshotError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeSnapshotJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to write response", "error", err)
	}
}

func writeSnapshotError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		slog.Warn("failed to write error response", "error", err)
	}
}
