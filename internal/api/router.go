package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter creates the HTTP router with all middleware and routes.
func NewRouter(pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestID)
	r.Use(Recovery(logger))
	r.Use(Logger(logger))

	h := &handlers{pool: pool}
	r.Get("/healthz", h.healthz)

	return r
}
