// Package storagesim provides the Cirrus Storage API simulator.
package storagesim

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

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
}

// New creates a new storage-sim Server.
func New(port string, logger *slog.Logger) *Server {
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
func (s *Server) Start() {
	go func() {
		s.logger.Info("storage-sim starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("storage-sim server failed", "error", err)
		}
	}()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
