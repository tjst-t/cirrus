// Package sim provides HTTP handlers for the simulator management API under /sim/.
package sim

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/tjst-t/cirrus/test/sim/storage/internal/state"
)

// ManagementHandler handles simulator management API requests under /sim/.
type ManagementHandler struct {
	store  *state.Store
	logger *slog.Logger
}

// NewManagementHandler creates a new ManagementHandler.
func NewManagementHandler(store *state.Store, logger *slog.Logger) *ManagementHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ManagementHandler{store: store, logger: logger}
}

// RegisterRoutes registers all management API routes on the given mux.
func (h *ManagementHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sim/backends", h.handleAddBackend)
	mux.HandleFunc("GET /sim/backends", h.handleListBackends)
	mux.HandleFunc("PUT /sim/backends/{id}/state", h.handleSetBackendState)
	mux.HandleFunc("POST /sim/config", h.handleSetConfig)
	mux.HandleFunc("GET /sim/stats", h.handleGetStats)
	mux.HandleFunc("POST /sim/reset", h.handleReset)
}

type addBackendRequest struct {
	BackendID          string   `json:"backend_id"`
	TotalCapacityGB    int64    `json:"total_capacity_gb"`
	TotalIOPS          int64    `json:"total_iops"`
	Capabilities       []string `json:"capabilities"`
	OverprovisionRatio float64  `json:"overprovision_ratio"`
}

func (h *ManagementHandler) handleAddBackend(w http.ResponseWriter, r *http.Request) {
	var req addBackendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	b := state.Backend{
		BackendID:          req.BackendID,
		TotalCapacityGB:    req.TotalCapacityGB,
		TotalIOPS:          req.TotalIOPS,
		Capabilities:       req.Capabilities,
		OverprovisionRatio: req.OverprovisionRatio,
	}

	if err := h.store.AddBackend(r.Context(), b); err != nil {
		code := http.StatusConflict
		if req.BackendID == "" {
			code = http.StatusBadRequest
		}
		writeError(w, code, err.Error())
		return
	}

	// Return the stored backend
	stored, _ := h.store.GetBackend(r.Context(), req.BackendID)
	writeJSON(w, http.StatusCreated, stored)
}

func (h *ManagementHandler) handleListBackends(w http.ResponseWriter, r *http.Request) {
	backends := h.store.ListBackends(r.Context())
	writeJSON(w, http.StatusOK, backends)
}

type setBackendStateRequest struct {
	State string `json:"state"`
}

func (h *ManagementHandler) handleSetBackendState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req setBackendStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	bs := state.BackendState(req.State)
	if err := h.store.SetBackendState(r.Context(), id, bs); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	b, _ := h.store.GetBackend(r.Context(), id)
	writeJSON(w, http.StatusOK, b)
}

func (h *ManagementHandler) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var cfg state.SimConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}
	h.store.SetConfig(r.Context(), cfg)
	writeJSON(w, http.StatusOK, cfg)
}

func (h *ManagementHandler) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := h.store.GetStats(r.Context())
	writeJSON(w, http.StatusOK, stats)
}

func (h *ManagementHandler) handleReset(w http.ResponseWriter, r *http.Request) {
	h.store.Reset(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
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
