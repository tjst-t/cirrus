// Package common provides shared services: event log, fault injection, data generator, snapshot.
package common

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/tjst-t/cirrus/test/sim/common/internal/handler"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/datagen"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/eventlog"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/snapshot"
)

// Server is the common API server instance.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates a new common Server with its own fault engine.
func New(port string, logger *slog.Logger) *Server {
	return NewWithFaultEngine(port, fault.New(), logger)
}

// NewWithFaultEngine creates a new common Server using the provided fault engine.
// Use this to share a fault engine between multiple simulators.
func NewWithFaultEngine(port string, fe *fault.Engine, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	el := eventlog.New()
	gen := datagen.New()
	snapMgr := snapshot.NewManager()

	mux := http.NewServeMux()
	handler.NewEventsHandler(el).Register(mux)
	handler.NewFaultHandler(fe).RegisterRoutes(mux)
	handler.NewDatagenHandler(gen).RegisterRoutes(mux)
	handler.NewSnapshotHandler(snapMgr).RegisterRoutes(mux)

	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", port),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Start starts the server in a goroutine.
func (s *Server) Start() {
	go func() {
		s.logger.Info("common-api starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("common-api server failed", "error", err)
		}
	}()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
