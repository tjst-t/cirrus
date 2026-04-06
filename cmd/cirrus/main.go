package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/agent"
	"github.com/tjst-t/cirrus/internal/api"
	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/blockdev"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/config"
	"github.com/tjst-t/cirrus/internal/controller"
	"github.com/tjst-t/cirrus/internal/controller/reconcile"
	"github.com/tjst-t/cirrus/internal/jobqueue"
	"github.com/tjst-t/cirrus/internal/scheduler"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/hypervisor"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/quota"
	"github.com/tjst-t/cirrus/internal/state"
	"github.com/tjst-t/cirrus/internal/storage"
	iscsistorage "github.com/tjst-t/cirrus/internal/storage/driver/iscsi"
	rbdstorage "github.com/tjst-t/cirrus/internal/storage/driver/rbd"
	simstorage "github.com/tjst-t/cirrus/internal/storage/driver/sim"
	"github.com/tjst-t/cirrus/internal/topology"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "cirrus",
		Short: "Cirrus IaaS platform",
	}
	rootCmd.AddCommand(newControllerCmd())
	rootCmd.AddCommand(newWorkerCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newControllerCmd() *cobra.Command {
	cfg := &config.ControllerConfig{}
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Start the Cirrus controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runController(cfg)
		},
	}
	f := cmd.Flags()
	f.IntVar(&cfg.APIPort, "api-port", 0, "HTTP API listen port (required)")
	f.IntVar(&cfg.GRPCPort, "grpc-port", 0, "gRPC listen port (required)")
	f.StringVar(&cfg.DBDSN, "db-dsn", "", "PostgreSQL connection string")
	f.StringVar(&cfg.StorageEndpoint, "storage-endpoint", "", "Storage sim/backend endpoint")
	f.StringVar(&cfg.AWXEndpoint, "awx-endpoint", "", "AWX endpoint")
	f.StringVar(&cfg.NetBoxEndpoint, "netbox-endpoint", "", "NetBox endpoint")
	f.StringVar(&cfg.AuthTokens, "auth-tokens", "", "Static auth tokens (token1=externalid1,token2=externalid2)")
	f.StringVar(&cfg.RegistrationToken, "registration-token", "", "Shared secret for worker self-registration")
	f.StringVar(&cfg.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	f.BoolVar(&cfg.Debug, "debug", false, "Include internal error details in API 500 responses (development only)")
	f.IntVar(&cfg.ReconcileInterval, "reconcile-interval", 300, "Active reconcile loop interval in seconds")
	f.IntVar(&cfg.ReconcileNetworkInterval, "reconcile-network-interval", 0, "Network reconcile interval in seconds (0 = use --reconcile-interval)")
	f.IntVar(&cfg.ReconcileStorageInterval, "reconcile-storage-interval", 0, "Storage reconcile interval in seconds (0 = use --reconcile-interval)")
	f.BoolVar(&cfg.ReconcileEnabled, "reconcile-enabled", true, "Enable active reconcile loops")
	f.BoolVar(&cfg.AutoHealEnabled, "auto-heal-enabled", true, "Enable auto-heal actions on drift detection")
	f.IntVar(&cfg.UnexpectedPresentThreshold, "unexpected-present-threshold", 3, "Consecutive detections before unexpected_present escalation")
	f.IntVar(&cfg.DriftEventRetentionDays, "drift-event-retention-days", 90, "Days to retain drift_events records")
	f.StringVar(&cfg.SecretsKey, "secrets-key", "", "Base64-encoded 32-byte AES-GCM key for encrypting VPN secrets (required for VPN egress)")
	return cmd
}

func newWorkerCmd() *cobra.Command {
	cfg := &config.WorkerConfig{}
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start a Cirrus worker",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorker(cfg)
		},
	}
	f := cmd.Flags()
	f.StringVar(&cfg.Controller, "controller", "localhost:9090", "Controller gRPC address")
	f.StringVar(&cfg.HostID, "host-id", "", "Host ID for this worker (deprecated: use --registration-token)")
	f.IntVar(&cfg.GRPCPort, "grpc-port", 0, "Port for the worker-side WorkerService gRPC server (0 = disabled)")
	f.StringVar(&cfg.WorkerGRPCAddr, "worker-grpc-addr", "", "Address (host:port) advertised to controller for WorkerService (auto-derived if empty)")
	f.StringVar(&cfg.LibvirtURI, "libvirt-uri", "", "Libvirt connection URI (tcp://host:port)")
	f.StringVar(&cfg.LibvirtSimMgmtAddr, "libvirt-sim-mgmt-addr", "", "libvirtd-sim HTTP management API base URL (e.g. http://localhost:8100)")
	f.StringVar(&cfg.RegistrationToken, "registration-token", "", "Registration token for self-registration")
	f.IntVar(&cfg.HeartbeatInterval, "heartbeat-interval", 10, "Heartbeat interval in seconds")
	f.StringVar(&cfg.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	f.StringVar(&cfg.StorageDomains, "storage-domains", "", "Comma-separated storage domains to join")
	f.StringVar(&cfg.Location, "location", "", "Location in the topology tree (name or ID)")
	f.StringVar(&cfg.FabricIP, "fabric-ip", "", "IP for Geneve tunnel endpoints (auto-detected if empty)")
	f.StringVar(&cfg.GatewayUplinkPort, "gw-uplink-port", "", "Physical uplink port for Direct Connect VLAN trunk (GW-role hosts only)")
	return cmd
}

func runController(cfg *config.ControllerConfig) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: config.ParseLogLevel(cfg.LogLevel)}))

	// Database
	pool, err := state.Connect(ctx, cfg.DBDSN)
	if err != nil {
		return fmt.Errorf("controller: db connect: %w", err)
	}
	defer state.Close(pool)
	logger.Info("database connected")

	if err := state.RunMigrations(cfg.DBDSN); err != nil {
		return fmt.Errorf("controller: migrations: %w", err)
	}
	logger.Info("migrations applied")

	// Job queue store (needed for recovery before dispatcher starts).
	jobQueue := jobqueue.NewStore(pool)

	// Recover any jobs that were left in status=running from a previous crash.
	if err := jobQueue.RecoverAllRunningJobs(ctx, logger); err != nil {
		return fmt.Errorf("controller: job recovery: %w", err)
	}

	// Identity service
	identitySvc := identity.NewStore(pool)

	// Authentication
	tokenMap := config.ParseAuthTokens(cfg.AuthTokens)
	authn := identity.NewStaticTokenAuth(tokenMap, identitySvc)

	// Bootstrap users from static tokens (dev convenience)
	if tokenMap != nil {
		var externalIDs []string
		for _, eid := range tokenMap {
			externalIDs = append(externalIDs, eid)
		}
		identity.BootstrapUsers(ctx, identitySvc, externalIDs, logger)
	}

	// Authorization
	authz := identity.NewRBACAuthorizer(identitySvc)

	// Host service
	hostSvc := host.NewStore(pool)

	// Topology service
	topologySvc := topology.NewStore(pool)

	// Availability Zone service
	azSvc := az.NewStore(pool)

	// Quota service
	quotaSvc := quota.NewStore(pool)

	// Storage service
	storageDrivers := storage.DriverRegistry{
		"sim": func(endpoint string, backendID string, driverCfg map[string]any) storage.Driver {
			// Resolve sim:// scheme to the real storage-sim HTTP endpoint.
			if strings.HasPrefix(endpoint, "sim://") && cfg.StorageEndpoint != "" {
				endpoint = cfg.StorageEndpoint
			}
			// Use sim_backend_id from driver_config if present (sim uses its own ID scheme).
			if simID, ok := driverCfg["sim_backend_id"].(string); ok && simID != "" {
				backendID = simID
			}
			return simstorage.New(endpoint, backendID)
		},
		"iscsi": func(endpoint, backendID string, cfg map[string]any) storage.Driver {
			return iscsistorage.New(endpoint, backendID, cfg)
		},
		"rbd": func(endpoint, backendID string, cfg map[string]any) storage.Driver {
			return rbdstorage.New(endpoint, backendID, cfg)
		},
	}
	storageStore := storage.NewStore(pool)

	dispatcher := jobqueue.NewDispatcher(jobQueue, 4, logger)

	storageSvc := storage.NewService(storageStore, storageDrivers, quotaSvc, jobQueue, logger)

	// Flavor service
	flavorSvc := flavor.NewService(pool)

	// SecretsKey: optional — required only when creating VPN egress types.
	// If absent, the store returns an error when vpn_ipsec or vpn_wireguard is requested.
	var secretsKey []byte
	if cfg.SecretsKey == "" {
		logger.Warn("secrets_key not configured; VPN egress types will be unavailable")
	} else {
		var err error
		secretsKey, err = base64.StdEncoding.DecodeString(cfg.SecretsKey)
		if err != nil {
			return fmt.Errorf("controller: decode secrets-key: %w", err)
		}
	}

	// Network service
	networkSvc := network.NewStore(pool, logger, quotaSvc).WithSecretsKey(secretsKey)

	// Scheduler
	sched := scheduler.New(hostSvc, storageSvc, topologySvc)

	// Worker client pool (controller → worker gRPC)
	workerPool := controller.NewWorkerClientPool()
	defer workerPool.Close()

	// Compute orchestrator
	computeSvc := compute.NewOrchestrator(pool, flavorSvc, networkSvc, storageSvc, sched, workerPool, quotaSvc, jobQueue, logger)

	// Register job handlers with the dispatcher.
	// Both interfaces now declare RegisterHandlers, so these compile-time assertions
	// fail loudly if the concrete types are ever removed from the interface.
	computeSvc.RegisterHandlers(dispatcher)
	storageSvc.RegisterHandlers(dispatcher)

	// HTTP API
	router := api.NewRouter(pool, logger, authn, authz, identitySvc, hostSvc, topologySvc, networkSvc, azSvc, storageSvc, flavorSvc, computeSvc, quotaSvc, jobQueue, cfg.Debug)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.APIPort),
		Handler: router,
	}

	// Network state controller
	stateCtrl := network.NewStateController(pool, logger).WithSecretsKey(secretsKey)
	networkStateSrv := network.NewGRPCStateServer(stateCtrl, logger, cfg.RegistrationToken)

	// Reconcile intervals
	reconcileInterval := time.Duration(cfg.ReconcileInterval) * time.Second
	if reconcileInterval <= 0 {
		reconcileInterval = 5 * time.Minute
	}
	netInterval := time.Duration(cfg.ReconcileNetworkInterval) * time.Second
	if netInterval <= 0 {
		netInterval = reconcileInterval
	}
	storageInterval := time.Duration(cfg.ReconcileStorageInterval) * time.Second
	if storageInterval <= 0 {
		storageInterval = reconcileInterval
	}

	// DriftHandler (wires VMHealer + NetworkHealer)
	driftHandler := reconcile.NewDriftHandler(reconcile.DriftHandlerConfig{
		Pool:            pool,
		Logger:          logger,
		AutoHealEnabled: cfg.AutoHealEnabled,
		DedupTTL:        reconcileInterval * 2,
		VMHealer:        computeSvc,
		NetworkHealer:   networkStateSrv,
	})

	// HeartbeatReconciler
	hbReconciler := reconcile.NewHeartbeatReconciler(pool, driftHandler, logger)

	// gRPC
	grpcSrv := controller.NewGRPCServer(logger, hostSvc, topologySvc, networkStateSrv, cfg.RegistrationToken, hbReconciler)
	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("controller: grpc listen: %w", err)
	}

	logger.Info("controller starting",
		"api_port", cfg.APIPort,
		"grpc_port", cfg.GRPCPort,
		"reconcile_enabled", cfg.ReconcileEnabled,
		"auto_heal_enabled", cfg.AutoHealEnabled,
	)

	g, gCtx := errgroup.WithContext(ctx)

	// Job dispatcher
	g.Go(func() error {
		dispatcher.Start(gCtx)
		return nil
	})

	g.Go(func() error {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := grpcSrv.Serve(grpcLis); err != nil && err != grpc.ErrServerStopped {
			return fmt.Errorf("grpc: %w", err)
		}
		return nil
	})

	// Network reconciler
	if cfg.ReconcileEnabled {
		netReconciler := reconcile.NewNetworkReconciler(stateCtrl, hostSvc, driftHandler, logger, netInterval).WithPool(pool)
		g.Go(func() error {
			return netReconciler.Run(gCtx)
		})

		// Storage reconciler
		storageReconciler := reconcile.NewStorageReconciler(storageSvc, driftHandler, logger, storageInterval)
		g.Go(func() error {
			return storageReconciler.Run(gCtx)
		})
	}

	// Heartbeat monitor
	faultyHandler := controller.NewHostFaultyHandler(pool, logger)
	heartbeatMonitor := controller.NewHeartbeatMonitor(pool, hostSvc, faultyHandler, logger, 30*time.Second)
	g.Go(func() error {
		return heartbeatMonitor.Run(gCtx)
	})

	g.Go(func() error {
		<-gCtx.Done()
		logger.Info("shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		grpcSrv.GracefulStop()
		return httpSrv.Shutdown(shutdownCtx)
	})

	return g.Wait()
}

func runWorker(cfg *config.WorkerConfig) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: config.ParseLogLevel(cfg.LogLevel)}))

	// Hypervisor driver
	var driver hypervisor.Driver
	if cfg.LibvirtURI != "" {
		driver = hypervisor.NewLibvirtDriver(cfg.LibvirtURI, cfg.LibvirtSimMgmtAddr)
		if err := driver.Connect(ctx); err != nil {
			logger.Warn("libvirt connection check failed", "uri", cfg.LibvirtURI, "error", err)
		} else {
			logger.Info("libvirt connection OK", "uri", cfg.LibvirtURI)
		}
	}

	// Connect to controller
	ag, err := agent.New(cfg.Controller, cfg.HostID, logger, driver)
	if err != nil {
		return fmt.Errorf("worker: agent init: %w", err)
	}
	defer ag.Close()

	// Self-registration: if registration token is provided, register before heartbeat
	if cfg.RegistrationToken != "" {
		var topo *agent.TopologyDeclaration
		if cfg.StorageDomains != "" || cfg.Location != "" {
			topo = &agent.TopologyDeclaration{
				Location: cfg.Location,
			}
			if cfg.StorageDomains != "" {
				for _, sd := range strings.Split(cfg.StorageDomains, ",") {
					sd = strings.TrimSpace(sd)
					if sd != "" {
						topo.StorageDomains = append(topo.StorageDomains, sd)
					}
				}
			}
		}
		if err := ag.Register(ctx, cfg.RegistrationToken, cfg.LibvirtURI, cfg.FabricIP, cfg.WorkerGRPCAddr, topo); err != nil {
			return fmt.Errorf("worker: registration failed: %w", err)
		}
	}

	// Wire the resolved host ID into the libvirt driver so it can call
	// host-scoped management API endpoints.
	if libvirtDrv, ok := driver.(*hypervisor.LibvirtDriver); ok {
		libvirtDrv.SetHostID(ag.HostID())
	}

	logger.Info("worker starting", "host_id", ag.HostID(), "controller", cfg.Controller)

	interval := time.Duration(cfg.HeartbeatInterval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	// Create network agent (uses shared gRPC connection)
	netAgent := ag.CreateNetworkAgent(cfg.Controller, cfg.RegistrationToken, logger)

	g, gCtx := errgroup.WithContext(ctx)

	// Heartbeat loop
	g.Go(func() error {
		ag.RunHeartbeat(gCtx, interval)
		return nil
	})

	// Network agent loop
	if netAgent != nil {
		g.Go(func() error {
			return netAgent.Run(gCtx)
		})
	}

	// Worker gRPC server (WorkerService: called by controller to create/delete VMs)
	if cfg.GRPCPort > 0 {
		blockMgr := blockdev.New(logger)
		workerSrv := agent.NewWorkerServer(driver, blockMgr, logger)
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
		if err != nil {
			return fmt.Errorf("worker: grpc listen: %w", err)
		}
		g.Go(func() error {
			return agent.StartGRPCServer(gCtx, lis, workerSrv, logger)
		})
	}

	return g.Wait()
}
