package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/internal/state"
)

type handlers struct {
	pool *pgxpool.Pool
}

func (h *handlers) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var err error
	if h.pool != nil {
		err = state.HealthCheck(r.Context(), h.pool)
	} else {
		err = fmt.Errorf("database not connected")
	}
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
