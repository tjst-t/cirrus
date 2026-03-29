// Package awxsim provides the AWX REST API simulator.
package awxsim

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/tjst-t/cirrus/test/sim/awx/internal/handler"
	"github.com/tjst-t/cirrus/test/sim/awx/internal/state"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
)

// Server is the awx-sim server instance.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates a new awx-sim Server.
func New(port string, logger *slog.Logger) *Server {
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
		logger: logger,
	}
}

// Start starts the server in a goroutine.
func (s *Server) Start() {
	go func() {
		s.logger.Info("awx-sim starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("awx-sim server failed", "error", err)
		}
	}()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
