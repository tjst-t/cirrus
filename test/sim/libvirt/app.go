// Package libvirtsim provides the libvirt RPC protocol simulator.
//
// Two modes of operation:
//   - Multi-host mode (Server): One process manages multiple hosts.
//     Used by the unified cirrus-sim binary for local development.
//   - Single-host mode (HostInstance): One process per host.
//     Used inside worker containers in docker-compose environments.
package libvirtsim

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/handler"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/netns"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/rpc"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

// Server is the multi-host libvirt-sim server instance.
// It manages multiple hosts, each with its own RPC listener.
type Server struct {
	httpServer *http.Server
	rpcServer  *rpc.Server
	store      *state.Store
	logger     *slog.Logger
}

// New creates a new multi-host libvirt-sim Server.
func New(port string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	store := state.NewStore()
	rpcServer := rpc.NewServer(store, logger)
	mgmt := handler.NewManagement(store, rpcServer, logger)

	mux := http.NewServeMux()
	mgmt.RegisterRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", port),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		rpcServer: rpcServer,
		store:     store,
		logger:    logger,
	}
}

// SeedHost registers a host and starts its RPC listener.
func (s *Server) SeedHost(ctx context.Context, cfg HostConfig) error {
	host := &state.Host{
		HostID:             cfg.HostID,
		LibvirtPort:        cfg.LibvirtPort,
		CPUModel:           cfg.CPUModel,
		CPUSockets:         cfg.CPUSockets,
		CoresPerSocket:     cfg.CoresPerSocket,
		ThreadsPerCore:     cfg.ThreadsPerCore,
		MemoryMB:           cfg.MemoryMB,
		CPUOvercommitRatio: cfg.CPUOvercommitRatio,
		MemOvercommitRatio: cfg.MemOvercommitRatio,
	}
	if err := s.store.AddHost(host); err != nil {
		return fmt.Errorf("add host %s: %w", cfg.HostID, err)
	}
	if err := s.rpcServer.StartListener(ctx, cfg.HostID, cfg.LibvirtPort); err != nil {
		_ = s.store.RemoveHost(cfg.HostID)
		return fmt.Errorf("start listener for host %s: %w", cfg.HostID, err)
	}
	return nil
}

// Start starts the management API server in a goroutine.
func (s *Server) Start() {
	go func() {
		s.logger.Info("libvirt-sim starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("libvirt-sim server failed", "error", err)
		}
	}()
}

// Shutdown gracefully shuts down the management API and all RPC listeners.
func (s *Server) Shutdown(ctx context.Context) error {
	s.rpcServer.StopAll()
	s.rpcServer.Wait()
	return s.httpServer.Shutdown(ctx)
}

// HostConfig holds the configuration for seeding a host.
type HostConfig struct {
	HostID             string
	LibvirtPort        int
	CPUModel           string
	CPUSockets         int
	CoresPerSocket     int
	ThreadsPerCore     int
	MemoryMB           int64
	CPUOvercommitRatio float64
	MemOvercommitRatio float64
}

// HostInstance is a single-host libvirt-sim instance.
// Each worker container runs one HostInstance managing exactly one host.
// It manages namespace/veth operations for VM simulation.
type HostInstance struct {
	httpServer *http.Server
	rpcServer  *rpc.Server
	store      *state.Store
	hostID     string
	logger     *slog.Logger
}

// HostInstanceConfig holds the configuration for a single-host instance.
type HostInstanceConfig struct {
	HostID             string
	LibvirtPort        int
	MgmtPort           string
	CPUModel           string
	CPUSockets         int
	CoresPerSocket     int
	ThreadsPerCore     int
	MemoryMB           int64
	CPUOvercommitRatio float64
	MemOvercommitRatio float64
	OVSBridge          string // default: br-int
	EnableNetns        bool   // true in privileged containers
}

// NewHostInstance creates a single-host libvirt-sim instance.
func NewHostInstance(cfg HostInstanceConfig, logger *slog.Logger) *HostInstance {
	if logger == nil {
		logger = slog.Default()
	}

	store := state.NewStore()
	rpcServer := rpc.NewServer(store, logger)

	// Configure netns manager
	var nsMgr netns.Manager
	if cfg.EnableNetns {
		nsMgr = netns.NewLinuxManager(cfg.OVSBridge, logger)
	} else {
		nsMgr = netns.NewNoopManager(logger)
	}
	rpcServer.SetNetnsManager(nsMgr)

	mgmt := handler.NewManagement(store, rpcServer, logger)

	mux := http.NewServeMux()
	mgmt.RegisterRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Pre-register the single host
	host := &state.Host{
		HostID:             cfg.HostID,
		LibvirtPort:        cfg.LibvirtPort,
		CPUModel:           cfg.CPUModel,
		CPUSockets:         cfg.CPUSockets,
		CoresPerSocket:     cfg.CoresPerSocket,
		ThreadsPerCore:     cfg.ThreadsPerCore,
		MemoryMB:           cfg.MemoryMB,
		CPUOvercommitRatio: cfg.CPUOvercommitRatio,
		MemOvercommitRatio: cfg.MemOvercommitRatio,
	}
	if host.CPUOvercommitRatio == 0 {
		host.CPUOvercommitRatio = 4.0
	}
	if host.MemOvercommitRatio == 0 {
		host.MemOvercommitRatio = 1.5
	}
	if err := store.AddHost(host); err != nil {
		logger.Error("failed to pre-register host", "host_id", cfg.HostID, "error", err)
	}

	return &HostInstance{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", cfg.MgmtPort),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		rpcServer: rpcServer,
		store:     store,
		hostID:    cfg.HostID,
		logger:    logger,
	}
}

// Start starts both the management API and the libvirt RPC listener.
func (h *HostInstance) Start() {
	ctx := context.Background()

	// Start the libvirt RPC listener for this host
	host, err := h.store.GetHost(h.hostID)
	if err != nil {
		h.logger.Error("host not found in store", "host_id", h.hostID, "error", err)
		return
	}
	if err := h.rpcServer.StartListener(ctx, h.hostID, host.LibvirtPort); err != nil {
		h.logger.Error("failed to start RPC listener", "host_id", h.hostID, "error", err)
		return
	}

	// Start the management API
	go func() {
		h.logger.Info("libvirtd-sim (single-host) starting",
			"host_id", h.hostID,
			"mgmt_addr", h.httpServer.Addr,
			"libvirt_port", host.LibvirtPort,
		)
		if err := h.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error("management API failed", "error", err)
		}
	}()
}

// Shutdown gracefully shuts down the instance.
func (h *HostInstance) Shutdown(ctx context.Context) error {
	h.rpcServer.StopAll()
	h.rpcServer.Wait()
	return h.httpServer.Shutdown(ctx)
}
