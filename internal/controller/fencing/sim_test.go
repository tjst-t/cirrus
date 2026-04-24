package fencing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSimFencingAgent_Fence_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"off"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	agent := NewSimFencingAgent(srv.URL, 5*time.Second)
	hostID := uuid.New()

	if err := agent.Fence(context.Background(), hostID); err != nil {
		t.Errorf("Fence() returned unexpected error: %v", err)
	}
}

func TestSimFencingAgent_Fence_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	agent := NewSimFencingAgent(srv.URL, 5*time.Second)
	hostID := uuid.New()

	err := agent.Fence(context.Background(), hostID)
	if err == nil {
		t.Fatal("Fence() expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not contain status code 500", err.Error())
	}
}

func TestSimFencingAgent_Fence_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// hang forever — the client timeout will fire first
		select {
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	agent := NewSimFencingAgent(srv.URL, 50*time.Millisecond)
	hostID := uuid.New()

	err := agent.Fence(context.Background(), hostID)
	if err == nil {
		t.Fatal("Fence() expected timeout error, got nil")
	}
}
