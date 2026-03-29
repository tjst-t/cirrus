package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
)

// FaultHandler handles fault injection API requests.
type FaultHandler struct {
	engine *fault.Engine
}

// NewFaultHandler creates a new FaultHandler.
func NewFaultHandler(engine *fault.Engine) *FaultHandler {
	return &FaultHandler{engine: engine}
}

// RegisterRoutes registers fault injection routes on the given mux.
func (h *FaultHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/faults", h.addRule)
	mux.HandleFunc("GET /api/v1/faults", h.getRules)
	mux.HandleFunc("DELETE /api/v1/faults/{id}", h.deleteRule)
	mux.HandleFunc("DELETE /api/v1/faults", h.clearRules)
}

func (h *FaultHandler) addRule(w http.ResponseWriter, r *http.Request) {
	var rule fault.FaultRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeFaultError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id := h.engine.AddRule(r.Context(), rule)
	rule.ID = id

	writeFaultJSON(w, http.StatusCreated, rule)
}

func (h *FaultHandler) getRules(w http.ResponseWriter, r *http.Request) {
	rules := h.engine.GetRules(r.Context())
	writeFaultJSON(w, http.StatusOK, rules)
}

func (h *FaultHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		// Try extracting from path manually
		id = strings.TrimPrefix(r.URL.Path, "/api/v1/faults/")
	}

	if err := h.engine.DeleteRule(r.Context(), id); err != nil {
		writeFaultError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FaultHandler) clearRules(w http.ResponseWriter, r *http.Request) {
	// Only clear if path is exactly /api/v1/faults (not /api/v1/faults/{id})
	if r.PathValue("id") != "" {
		h.deleteRule(w, r)
		return
	}

	h.engine.ClearRules(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func writeFaultJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to write response", "error", err)
	}
}

func writeFaultError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		slog.Warn("failed to write error response", "error", err)
	}
}
