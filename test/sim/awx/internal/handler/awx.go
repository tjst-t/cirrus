// Package handler provides HTTP handlers for the AWX simulator API.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/tjst-t/cirrus/test/sim/awx/internal/state"
)

// Handler provides AWX REST API handlers.
type Handler struct {
	store *state.Store
}

// NewHandler creates a new Handler backed by the given store.
func NewHandler(store *state.Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers all AWX API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/job_templates/", h.handleJobTemplates)
	mux.HandleFunc("/api/v2/jobs/", h.handleJobs)
	mux.HandleFunc("/sim/config/callback", h.handleCallback)
	mux.HandleFunc("/sim/config/callback/", h.handleCallback)
	mux.HandleFunc("/sim/stats", h.handleStats)
	mux.HandleFunc("/sim/stats/", h.handleStats)
	mux.HandleFunc("/sim/reset", h.handleReset)
	mux.HandleFunc("/sim/reset/", h.handleReset)
}

func (h *Handler) handleJobTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := trimTrailingSlash(r.URL.Path)

	// POST /api/v2/job_templates
	if path == "/api/v2/job_templates" && r.Method == http.MethodPost {
		h.createTemplate(ctx, w, r)
		return
	}

	// GET /api/v2/job_templates
	if path == "/api/v2/job_templates" && r.Method == http.MethodGet {
		h.listTemplates(ctx, w)
		return
	}

	// Check for /api/v2/job_templates/{id} or /api/v2/job_templates/{id}/launch
	rest := strings.TrimPrefix(path, "/api/v2/job_templates/")
	if rest == path {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// /api/v2/job_templates/{id}/launch
	if parts := strings.SplitN(rest, "/", 2); len(parts) == 2 && parts[1] == "launch" {
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid template id")
			return
		}
		if r.Method == http.MethodPost {
			h.launchJob(ctx, w, r, id)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// /api/v2/job_templates/{id}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template id")
		return
	}
	if r.Method == http.MethodGet {
		h.getTemplate(ctx, w, id)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (h *Handler) handleJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := trimTrailingSlash(r.URL.Path)

	rest := strings.TrimPrefix(path, "/api/v2/jobs/")
	if rest == path || rest == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// /api/v2/jobs/{id}/cancel
	if parts := strings.SplitN(rest, "/", 2); len(parts) == 2 && parts[1] == "cancel" {
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		if r.Method == http.MethodPost {
			h.cancelJob(ctx, w, id)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// /api/v2/jobs/{id}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	if r.Method == http.MethodGet {
		h.getJob(ctx, w, id)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (h *Handler) createTemplate(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name               string  `json:"name"`
		Description        string  `json:"description"`
		ExpectedDurationMs int64   `json:"expected_duration_ms"`
		FailureRate        float64 `json:"failure_rate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tmpl, err := h.store.CreateTemplate(ctx, req.Name, req.Description, req.ExpectedDurationMs, req.FailureRate)
	if err != nil {
		slog.WarnContext(ctx, "failed to create template", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, tmpl)
}

func (h *Handler) listTemplates(ctx context.Context, w http.ResponseWriter) {
	templates := h.store.ListTemplates(ctx)
	resp := map[string]interface{}{
		"count":   len(templates),
		"results": templates,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getTemplate(ctx context.Context, w http.ResponseWriter, id int64) {
	tmpl, err := h.store.GetTemplate(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tmpl)
}

func (h *Handler) launchJob(ctx context.Context, w http.ResponseWriter, r *http.Request, templateID int64) {
	var req struct {
		ExtraVars map[string]interface{} `json:"extra_vars"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	job, err := h.store.LaunchJob(ctx, templateID, req.ExtraVars)
	if err != nil {
		slog.WarnContext(ctx, "failed to launch job", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (h *Handler) getJob(ctx context.Context, w http.ResponseWriter, id int64) {
	job, err := h.store.GetJob(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) cancelJob(ctx context.Context, w http.ResponseWriter, id int64) {
	job, err := h.store.CancelJob(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "failed to cancel job", "error", err)
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method == http.MethodPost {
		var cfg state.CallbackConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		h.store.SetCallback(ctx, cfg)
		writeJSON(w, http.StatusOK, cfg)
		return
	}

	if r.Method == http.MethodGet {
		cfg := h.store.GetCallback(ctx)
		writeJSON(w, http.StatusOK, cfg)
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	stats := h.store.GetStats(r.Context())
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.store.Reset(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func trimTrailingSlash(path string) string {
	if len(path) > 1 && path[len(path)-1] == '/' {
		return path[:len(path)-1]
	}
	return path
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		slog.Warn("failed to write error response", "error", err)
	}
}
