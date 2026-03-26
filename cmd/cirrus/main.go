package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/agent"
	"github.com/tjst-t/cirrus/internal/api"
	"github.com/tjst-t/cirrus/internal/config"
	"github.com/tjst-t/cirrus/internal/controller"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/hypervisor"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network/ovn"
	"github.com/tjst-t/cirrus/internal/state"
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
	f.IntVar(&cfg.APIPort, "api-port", 8080, "HTTP API listen port")
	f.IntVar(&cfg.GRPCPort, "grpc-port", 9090, "gRPC listen port")
	f.StringVar(&cfg.DBDSN, "db-dsn", "", "PostgreSQL connection string")
	f.StringVar(&cfg.OVNNB, "ovn-nb", "", "OVN Northbound DB address (tcp:host:port)")
	f.StringVar(&cfg.StorageEndpoint, "storage-endpoint", "", "Storage sim/backend endpoint")
	f.StringVar(&cfg.AWXEndpoint, "awx-endpoint", "", "AWX endpoint")
	f.StringVar(&cfg.NetBoxEndpoint, "netbox-endpoint", "", "NetBox endpoint")
	f.StringVar(&cfg.AuthTokens, "auth-tokens", "", "Static auth tokens (token1=externalid1,token2=externalid2)")
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
	f.StringVar(&cfg.HostID, "host-id", "", "Host ID for this worker")
	f.StringVar(&cfg.LibvirtURI, "libvirt-uri", "", "Libvirt connection URI (tcp://host:port)")
	return cmd
}

func runController(cfg *config.ControllerConfig) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

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

	// OVN connection check (non-blocking)
	if cfg.OVNNB != "" {
		if err := ovn.CheckConnection(ctx, cfg.OVNNB); err != nil {
			logger.Warn("OVN NB connection check failed", "addr", cfg.OVNNB, "error", err)
		} else {
			logger.Info("OVN NB connection OK", "addr", cfg.OVNNB)
		}
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

	// HTTP API
	router := api.NewRouter(pool, logger, authn, authz, identitySvc, hostSvc)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.APIPort),
		Handler: router,
	}

	// gRPC
	grpcSrv := controller.NewGRPCServer(logger, hostSvc)
	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("controller: grpc listen: %w", err)
	}

	logger.Info("controller starting",
		"api_port", cfg.APIPort,
		"grpc_port", cfg.GRPCPort,
	)

	g, gCtx := errgroup.WithContext(ctx)

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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Hypervisor driver
	var driver hypervisor.Driver
	if cfg.LibvirtURI != "" {
		driver = hypervisor.NewLibvirtDriver(cfg.LibvirtURI)
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

	logger.Info("worker starting", "host_id", cfg.HostID, "controller", cfg.Controller)

	// Run heartbeat loop (blocks until ctx cancelled)
	ag.RunHeartbeat(ctx, 10*time.Second)

	return nil
}
