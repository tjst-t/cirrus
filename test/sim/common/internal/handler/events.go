// Package handler provides HTTP handlers for the common event log API.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/eventlog"
)

const defaultLimit = 100

// EventsHandler handles HTTP requests for the event log API.
type EventsHandler struct {
	log *eventlog.EventLog
}

// NewEventsHandler creates a new EventsHandler with the given event log.
func NewEventsHandler(log *eventlog.EventLog) *EventsHandler {
	return &EventsHandler{log: log}
}

// Register registers the event log API routes on the given mux.
func (h *EventsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/events", h.handleEvents)
}

func (h *EventsHandler) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getEvents(w, r)
	case http.MethodDelete:
		h.deleteEvents(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type eventsResponse struct {
	Events []eventlog.Event `json:"events"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

func (h *EventsHandler) getEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	filter, limit, offset, err := parseFilterFromQuery(ctx, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("bad request: %s", err), http.StatusBadRequest)
		return
	}

	events, total := h.log.Query(ctx, filter)

	resp := eventsResponse{
		Events: events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}

	// Ensure events is never null in JSON
	if resp.Events == nil {
		resp.Events = []eventlog.Event{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(ctx, "failed to encode response", "error", err)
	}
}

func (h *EventsHandler) deleteEvents(w http.ResponseWriter, r *http.Request) {
	h.log.Clear(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// parseFilterFromQuery extracts filter, limit, and offset from query parameters.
func parseFilterFromQuery(ctx context.Context, r *http.Request) (eventlog.Filter, int, int, error) {
	q := r.URL.Query()

	limit := defaultLimit
	offset := 0

	if v := q.Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return eventlog.Filter{}, 0, 0, fmt.Errorf("invalid limit: %w", err)
		}
		if parsed < 0 {
			return eventlog.Filter{}, 0, 0, fmt.Errorf("invalid limit: must be non-negative")
		}
		limit = parsed
	}

	if v := q.Get("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return eventlog.Filter{}, 0, 0, fmt.Errorf("invalid offset: %w", err)
		}
		if parsed < 0 {
			return eventlog.Filter{}, 0, 0, fmt.Errorf("invalid offset: must be non-negative")
		}
		offset = parsed
	}

	var after time.Time
	if v := q.Get("after"); v != "" {
		parsed, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return eventlog.Filter{}, 0, 0, fmt.Errorf("invalid after timestamp: %w", err)
		}
		after = parsed
	}

	_ = ctx // ctx available for future use

	filter := eventlog.Filter{
		Simulator: q.Get("simulator"),
		HostID:    q.Get("host_id"),
		Operation: q.Get("operation"),
		After:     after,
		Limit:     limit,
		Offset:    offset,
	}

	return filter, limit, offset, nil
}
