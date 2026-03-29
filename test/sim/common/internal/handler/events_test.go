package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/eventlog"
)

func seedEvents(el *eventlog.EventLog) {
	ctx := context.Background()
	events := []eventlog.Event{
		{Simulator: "libvirt-sim", HostID: "host-001", Operation: "domain_create", Result: "success", DurationMs: 10},
		{Simulator: "libvirt-sim", HostID: "host-002", Operation: "domain_delete", Result: "success", DurationMs: 5},
		{Simulator: "storage-sim", HostID: "host-001", Operation: "volume_create", Result: "failure", DurationMs: 20},
		{Simulator: "storage-sim", HostID: "host-003", Operation: "volume_delete", Result: "success", DurationMs: 8},
		{Simulator: "libvirt-sim", HostID: "host-001", Operation: "domain_start", Result: "success", DurationMs: 3},
	}
	for _, e := range events {
		el.Record(ctx, e)
	}
}

func TestGetEvents(t *testing.T) {
	el := eventlog.New()
	seedEvents(el)
	h := NewEventsHandler(el)

	mux := http.NewServeMux()
	h.Register(mux)

	tests := []struct {
		name           string
		query          string
		expectedTotal  int
		expectedLen    int
		expectedLimit  int
		expectedOffset int
		expectedStatus int
	}{
		{
			name:           "get all events",
			query:          "",
			expectedTotal:  5,
			expectedLen:    5,
			expectedLimit:  100,
			expectedOffset: 0,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "filter by simulator",
			query:          "?simulator=libvirt-sim",
			expectedTotal:  3,
			expectedLen:    3,
			expectedLimit:  100,
			expectedOffset: 0,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "filter by host_id",
			query:          "?host_id=host-001",
			expectedTotal:  3,
			expectedLen:    3,
			expectedLimit:  100,
			expectedOffset: 0,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "filter by operation",
			query:          "?operation=domain_create",
			expectedTotal:  1,
			expectedLen:    1,
			expectedLimit:  100,
			expectedOffset: 0,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "with limit",
			query:          "?limit=2",
			expectedTotal:  5,
			expectedLen:    2,
			expectedLimit:  2,
			expectedOffset: 0,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "with limit and offset",
			query:          "?limit=2&offset=3",
			expectedTotal:  5,
			expectedLen:    2,
			expectedLimit:  2,
			expectedOffset: 3,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "offset beyond total",
			query:          "?offset=10",
			expectedTotal:  5,
			expectedLen:    0,
			expectedLimit:  100,
			expectedOffset: 10,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/events"+tt.query, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			var resp eventsResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Total != tt.expectedTotal {
				t.Errorf("expected total %d, got %d", tt.expectedTotal, resp.Total)
			}
			if len(resp.Events) != tt.expectedLen {
				t.Errorf("expected %d events, got %d", tt.expectedLen, len(resp.Events))
			}
			if resp.Limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, resp.Limit)
			}
			if resp.Offset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, resp.Offset)
			}
		})
	}
}

func TestDeleteEvents(t *testing.T) {
	el := eventlog.New()
	seedEvents(el)
	h := NewEventsHandler(el)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}

	// Verify events are cleared
	req = httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp eventsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 events after delete, got %d", resp.Total)
	}
}

func TestInvalidLimitOffset(t *testing.T) {
	el := eventlog.New()
	h := NewEventsHandler(el)

	mux := http.NewServeMux()
	h.Register(mux)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
	}{
		{
			name:           "invalid limit",
			query:          "?limit=abc",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid offset",
			query:          "?offset=abc",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "negative limit",
			query:          "?limit=-1",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "negative offset",
			query:          "?offset=-1",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/events"+tt.query, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	el := eventlog.New()
	h := NewEventsHandler(el)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}
