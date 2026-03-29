package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
)

func setupFaultMux() *http.ServeMux {
	engine := fault.New()
	h := NewFaultHandler(engine)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestAddAndGetFaultRules(t *testing.T) {
	mux := setupFaultMux()

	body := `{"target":{"simulator":"libvirt-sim"},"trigger":{"type":"probabilistic","probability":0.5},"fault":{"type":"error","error_code":-1,"error_message":"fail"}}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/faults", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var created fault.FaultRule
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Get all rules
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v1/faults", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d", w.Code)
	}

	var rules []fault.FaultRule
	if err := json.NewDecoder(w.Body).Decode(&rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Errorf("rule count = %d, want 1", len(rules))
	}
}

func TestDeleteFaultRule(t *testing.T) {
	mux := setupFaultMux()

	body := `{"trigger":{"type":"probabilistic","probability":1.0},"fault":{"type":"error"}}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/faults", bytes.NewBufferString(body))
	mux.ServeHTTP(w, r)

	var created fault.FaultRule
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodDelete, "/api/v1/faults/"+created.ID, nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify empty
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v1/faults", nil)
	mux.ServeHTTP(w, r)

	var rules []fault.FaultRule
	if err := json.NewDecoder(w.Body).Decode(&rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("rule count = %d, want 0", len(rules))
	}
}

func TestClearFaultRules(t *testing.T) {
	mux := setupFaultMux()

	body := `{"trigger":{"type":"probabilistic","probability":1.0},"fault":{"type":"error"}}`
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/faults", bytes.NewBufferString(body))
		mux.ServeHTTP(w, r)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/faults", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("clear status = %d, want %d", w.Code, http.StatusNoContent)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v1/faults", nil)
	mux.ServeHTTP(w, r)

	var rules []fault.FaultRule
	if err := json.NewDecoder(w.Body).Decode(&rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("rule count = %d, want 0", len(rules))
	}
}

func TestDeleteNonexistentRule(t *testing.T) {
	mux := setupFaultMux()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/faults/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
