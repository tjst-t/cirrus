package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tjst-t/cirrus/test/sim/awx/internal/state"
)

func setupHandler() (*Handler, *http.ServeMux) {
	store := state.NewStore()
	h := NewHandler(store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestCreateAndListTemplates(t *testing.T) {
	_, mux := setupHandler()

	body := `{"name":"host-provision","description":"Provision host","expected_duration_ms":30000,"failure_rate":0.0}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v2/job_templates/", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v2/job_templates/", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if count := resp["count"].(float64); count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestGetTemplate(t *testing.T) {
	_, mux := setupHandler()

	body := `{"name":"t1","description":"","expected_duration_ms":100,"failure_rate":0}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v2/job_templates/", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v2/job_templates/1", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v2/job_templates/999", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestLaunchAndGetJob(t *testing.T) {
	_, mux := setupHandler()

	body := `{"name":"fast","description":"","expected_duration_ms":10,"failure_rate":0}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v2/job_templates/", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	body = `{"extra_vars":{"host_id":"host-1"}}`
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/v2/job_templates/1/launch/", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("launch status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var job map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v2/jobs/1/", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("get job status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCancelJob(t *testing.T) {
	_, mux := setupHandler()

	body := `{"name":"long","description":"","expected_duration_ms":60000,"failure_rate":0}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v2/job_templates/", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/v2/job_templates/1/launch/", bytes.NewBufferString(`{}`))
	mux.ServeHTTP(w, r)

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/v2/jobs/1/cancel/", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Cancel again should conflict
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/v2/jobs/1/cancel/", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Errorf("second cancel status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestCallbackConfig(t *testing.T) {
	_, mux := setupHandler()

	body := `{"enabled":true,"callback_url":"http://localhost/cb","auth_token":"tok"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sim/config/callback", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/sim/config/callback", nil)
	mux.ServeHTTP(w, r)

	var cfg state.CallbackConfig
	if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled {
		t.Error("expected callback enabled")
	}
}

func TestStatsAndReset(t *testing.T) {
	_, mux := setupHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/sim/stats", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/sim/reset", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want %d", w.Code, http.StatusOK)
	}
}
