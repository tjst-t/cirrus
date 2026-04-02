package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"github.com/tjst-t/cirrus/internal/api"
)

func TestHealthz_NoDB(t *testing.T) {
	// With a nil pool, healthz should return 503
	router := api.NewRouter(nil, slog.Default(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "error" {
		t.Fatalf("expected status=error, got %s", body["status"])
	}
}
