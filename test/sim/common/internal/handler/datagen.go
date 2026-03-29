package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/datagen"
)

// DatagenHandler handles data generator API requests.
type DatagenHandler struct {
	gen *datagen.Generator
}

// NewDatagenHandler creates a new DatagenHandler.
func NewDatagenHandler(gen *datagen.Generator) *DatagenHandler {
	return &DatagenHandler{gen: gen}
}

// RegisterRoutes registers data generator routes on the given mux.
func (h *DatagenHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/generate", h.generate)
	mux.HandleFunc("GET /api/v1/generate/status", h.getStatus)
	mux.HandleFunc("POST /api/v1/generate/reset", h.reset)
}

func (h *DatagenHandler) generate(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeDatagenError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	result, err := h.gen.Generate(r.Context(), data)
	if err != nil {
		writeDatagenError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeDatagenJSON(w, http.StatusOK, result)
}

func (h *DatagenHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	status := h.gen.GetStatus(r.Context())
	writeDatagenJSON(w, http.StatusOK, status)
}

func (h *DatagenHandler) reset(w http.ResponseWriter, r *http.Request) {
	h.gen.Reset(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func writeDatagenJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to write response", "error", err)
	}
}

func writeDatagenError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		slog.Warn("failed to write error response", "error", err)
	}
}
