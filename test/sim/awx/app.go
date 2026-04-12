// Package awxsim provides the AWX REST API simulator.
package awxsim

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/test/sim/awx/internal/handler"
	"github.com/tjst-t/cirrus/test/sim/awx/internal/state"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
)

// Server is the awx-sim server instance.
type Server struct {
	httpServer *http.Server
	store      *state.Store
	logger     *slog.Logger
	dbDSN      string // empty = no persistence
}

// New creates a new awx-sim Server without database persistence.
func New(port string, logger *slog.Logger) *Server {
	return NewWithDB(port, "", logger)
}

// NewWithDB creates a new awx-sim Server backed by PostgreSQL persistence.
// dsn is a postgres connection string; pass empty string to run without persistence.
func NewWithDB(port, dsn string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	store := state.NewStore()
	h := handler.NewHandler(store)
	faultEngine := fault.New()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	faultMw := fault.Middleware(faultEngine, "awx-sim")

	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", port),
			Handler:           faultMw(mux),
			ReadHeaderTimeout: 10 * time.Second,
		},
		store:  store,
		logger: logger,
		dbDSN:  dsn,
	}
}

// Start starts the server in a goroutine.
// If a DSN was provided, the DB is initialized and state is loaded before
// the HTTP server begins accepting connections.
func (s *Server) Start() {
	go func() {
		if s.dbDSN != "" {
			if err := s.initDB(); err != nil {
				s.logger.Error("awx-sim: DB init failed, running without persistence", "error", err)
			}
		}
		s.logger.Info("awx-sim starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("awx-sim server failed", "error", err)
		}
	}()
}

// initDB connects to PostgreSQL with retries, creates the schema, and loads
// persisted state into the in-memory store.
func (s *Server) initDB() error {
	ctx := context.Background()

	var pool *pgxpool.Pool
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		p, err := pgxpool.New(ctx, s.dbDSN)
		if err != nil {
			lastErr = err
			continue
		}
		if err := p.Ping(ctx); err != nil {
			p.Close()
			lastErr = err
			continue
		}
		pool = p
		break
	}
	if pool == nil {
		return fmt.Errorf("awx-sim: connect to postgres: %w", lastErr)
	}

	if err := state.SetupSchema(ctx, pool); err != nil {
		pool.Close()
		return err
	}

	s.store.SetDB(pool)

	if err := s.store.LoadFromDB(ctx); err != nil {
		return err
	}

	s.logger.Info("awx-sim: DB persistence enabled", "dsn", s.dbDSN)
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
