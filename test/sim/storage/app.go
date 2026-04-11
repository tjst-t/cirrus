// Package storagesim provides the Cirrus Storage API simulator.
package storagesim

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
	"github.com/tjst-t/cirrus/test/sim/storage/internal/handler"
	"github.com/tjst-t/cirrus/test/sim/storage/internal/sim"
	"github.com/tjst-t/cirrus/test/sim/storage/internal/state"
)

// Server is the storage-sim server instance.
type Server struct {
	httpServer *http.Server
	store      *state.Store
	logger     *slog.Logger
	dbDSN      string // empty = no persistence
}

// New creates a new storage-sim Server without database persistence.
func New(port string, logger *slog.Logger) *Server {
	return NewWithDB(port, "", logger)
}

// NewWithDB creates a new storage-sim Server backed by PostgreSQL persistence.
// dsn is a postgres connection string; pass empty string to run without persistence.
func NewWithDB(port, dsn string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	store := state.NewStore(logger)
	faultEngine := fault.New()
	mux := http.NewServeMux()

	storageHandler := handler.NewStorageHandler(store, logger)
	storageHandler.RegisterRoutes(mux)

	mgmtHandler := sim.NewManagementHandler(store, logger)
	mgmtHandler.RegisterRoutes(mux)

	// Wrap with fault injection middleware
	faultMw := fault.Middleware(faultEngine, "storage-sim")

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

// SeedBackend registers a storage backend.
func (s *Server) SeedBackend(cfg BackendConfig) error {
	b := state.Backend{
		BackendID:          cfg.BackendID,
		TotalCapacityGB:    cfg.TotalCapacityGB,
		TotalIOPS:          cfg.TotalIOPS,
		Capabilities:       cfg.Capabilities,
		State:              state.BackendActive,
		OverprovisionRatio: cfg.OverprovisionRatio,
	}
	return s.store.AddBackend(context.Background(), b)
}

// BackendConfig holds the configuration for seeding a storage backend.
type BackendConfig struct {
	BackendID          string
	TotalCapacityGB    int64
	TotalIOPS          int64
	Capabilities       []string
	OverprovisionRatio float64
}

// Start starts the server in a goroutine. Call Shutdown to stop.
// If a DSN was provided, the DB is initialized and state is loaded before
// the HTTP server begins accepting connections.
func (s *Server) Start() {
	go func() {
		if s.dbDSN != "" {
			if err := s.initDB(); err != nil {
				s.logger.Error("storage-sim: DB init failed, running without persistence", "error", err)
			}
		}
		s.logger.Info("storage-sim starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("storage-sim server failed", "error", err)
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
		return fmt.Errorf("storage-sim: connect to postgres: %w", lastErr)
	}

	if err := state.SetupSchema(ctx, pool); err != nil {
		pool.Close()
		return err
	}

	s.store.SetDB(pool)

	if err := s.store.LoadFromDB(ctx); err != nil {
		return err
	}

	s.logger.Info("storage-sim: DB persistence enabled", "dsn", s.dbDSN)
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
