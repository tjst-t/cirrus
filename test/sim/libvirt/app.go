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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
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
	dbDSN      string // empty = no persistence
}

// New creates a new multi-host libvirt-sim Server without database persistence.
func New(port string, logger *slog.Logger) *Server {
	return NewWithDB(port, "", logger)
}

// NewWithDB creates a new multi-host libvirt-sim Server backed by PostgreSQL persistence.
// dsn is a postgres connection string; pass empty string to run without persistence.
func NewWithDB(port, dsn string, logger *slog.Logger) *Server {
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
		dbDSN:     dsn,
	}
}

// SeedHost registers a host and starts its RPC listener.
// If the host already exists (e.g. restored from DB), it is silently skipped.
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
	err := s.store.AddHost(host)
	if err != nil {
		if isHostExists(err) {
			s.logger.Info("libvirt-sim: host already registered, skipping seed", "host_id", cfg.HostID)
			return nil
		}
		return fmt.Errorf("add host %s: %w", cfg.HostID, err)
	}
	if err := s.rpcServer.StartListener(ctx, cfg.HostID, cfg.LibvirtPort); err != nil {
		_ = s.store.RemoveHost(cfg.HostID)
		return fmt.Errorf("start listener for host %s: %w", cfg.HostID, err)
	}
	return nil
}

// isHostExists reports whether the error indicates the host already exists.
func isHostExists(err error) bool {
	return errors.Is(err, state.ErrHostExists)
}

// Start starts the management API server in a goroutine.
// If a DSN was provided, the DB is initialized and state is loaded before
// the HTTP server begins accepting connections. RPC listeners are restarted
// for all hosts that were restored from the database.
func (s *Server) Start() {
	go func() {
		if s.dbDSN != "" {
			if err := s.initDB(); err != nil {
				s.logger.Error("libvirt-sim: DB init failed, running without persistence", "error", err)
			}
		}
		s.logger.Info("libvirt-sim starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("libvirt-sim server failed", "error", err)
		}
	}()
}

// initDB connects to PostgreSQL with retries, creates the schema, and loads
// persisted state into the in-memory store. RPC listeners are restarted for
// each host restored from the database.
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
		return fmt.Errorf("libvirt-sim: connect to postgres: %w", lastErr)
	}

	if err := state.SetupSchema(ctx, pool); err != nil {
		pool.Close()
		return err
	}

	s.store.SetDB(pool)

	if err := s.store.LoadFromDB(ctx); err != nil {
		return err
	}

	// Restart RPC listeners for hosts restored from DB.
	for _, h := range s.store.ListHosts() {
		if err := s.rpcServer.StartListener(ctx, h.HostID, h.LibvirtPort); err != nil {
			s.logger.Error("libvirt-sim: restart RPC listener for restored host",
				"host_id", h.HostID, "port", h.LibvirtPort, "error", err)
		} else {
			s.logger.Info("libvirt-sim: restored host RPC listener",
				"host_id", h.HostID, "port", h.LibvirtPort)
		}
	}

	s.logger.Info("libvirt-sim: DB persistence enabled", "dsn", s.dbDSN)
	return nil
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
	hostCfg    HostInstanceConfig
	dbDSN      string // empty = no persistence
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
	DBDSN              string // optional postgres DSN for state persistence
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
	rpcServer.SetFaultEngine(fault.New())

	mgmt := handler.NewManagement(store, rpcServer, logger)

	mux := http.NewServeMux()
	mgmt.RegisterRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Enable single-host fallback: unknown host_id in URL resolves to the
	// single managed host. This lets the worker use its controller-assigned
	// UUID in management API calls without re-registering the host.
	mgmt.SetSingleHostID(cfg.HostID)

	if cfg.CPUOvercommitRatio == 0 {
		cfg.CPUOvercommitRatio = 4.0
	}
	if cfg.MemOvercommitRatio == 0 {
		cfg.MemOvercommitRatio = 1.5
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
		hostCfg:   cfg,
		dbDSN:     cfg.DBDSN,
		logger:    logger,
	}
}

// initHostDB connects to PostgreSQL, creates the schema, loads persisted
// state, and sets up DB-backed persistence for this HostInstance.
func (h *HostInstance) initHostDB(ctx context.Context) error {
	var pool *pgxpool.Pool
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		p, err := pgxpool.New(ctx, h.dbDSN)
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
		return fmt.Errorf("libvirtd-sim: connect to postgres: %w", lastErr)
	}

	if err := state.SetupSchema(ctx, pool); err != nil {
		pool.Close()
		return err
	}

	h.store.SetDB(pool)

	if err := h.store.LoadFromDB(ctx); err != nil {
		return err
	}

	h.logger.Info("libvirtd-sim: DB persistence enabled", "host_id", h.hostID, "dsn", h.dbDSN)
	return nil
}

// Start starts both the management API and the libvirt RPC listener.
// If a DBDSN was configured, DB persistence is initialized first and any
// previously persisted domains are restored before the RPC listener begins.
func (h *HostInstance) Start() {
	ctx := context.Background()

	// Initialise DB persistence (with retries) if a DSN was provided.
	if h.dbDSN != "" {
		if err := h.initHostDB(ctx); err != nil {
			h.logger.Error("libvirtd-sim: DB init failed, running without persistence", "error", err)
		}
	}

	// Ensure the host record exists in the store.  On a fresh start the host
	// is not in the DB yet; on a restart it will have been restored by
	// LoadFromDB above, in which case AddHost returns ErrHostExists — that is
	// expected and safe to ignore.
	if _, err := h.store.GetHost(h.hostID); err != nil {
		host := &state.Host{
			HostID:             h.hostCfg.HostID,
			LibvirtPort:        h.hostCfg.LibvirtPort,
			CPUModel:           h.hostCfg.CPUModel,
			CPUSockets:         h.hostCfg.CPUSockets,
			CoresPerSocket:     h.hostCfg.CoresPerSocket,
			ThreadsPerCore:     h.hostCfg.ThreadsPerCore,
			MemoryMB:           h.hostCfg.MemoryMB,
			CPUOvercommitRatio: h.hostCfg.CPUOvercommitRatio,
			MemOvercommitRatio: h.hostCfg.MemOvercommitRatio,
		}
		if addErr := h.store.AddHost(host); addErr != nil {
			h.logger.Error("failed to register host", "host_id", h.hostID, "error", addErr)
			return
		}
	}

	// Start the libvirt RPC listener for this host
	host, err := h.store.GetHost(h.hostID)
	if err != nil {
		h.logger.Error("host not found in store", "host_id", h.hostID, "error", err)
		return
	}

	domainCount := 0
	if domains, listErr := h.store.ListDomains(h.hostID); listErr == nil {
		domainCount = len(domains)
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
			"restored_domains", domainCount,
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
