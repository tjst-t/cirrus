// Package main provides the unified cirrus-sim binary that starts all simulators in one process.
//
// OVN-sim and NetBox-sim are deprecated (OVN→VPC migration, NetBox is Phase 3).
//
// Usage:
//
//	cirrus-sim                    # start all with default ports
//	cirrus-sim -common=8000 ...  # override individual ports
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tjst-t/cirrus/test/sim/aggregator"
	awxsim "github.com/tjst-t/cirrus/test/sim/awx"
	common "github.com/tjst-t/cirrus/test/sim/common"
	libvirtsim "github.com/tjst-t/cirrus/test/sim/libvirt"
	pgsim "github.com/tjst-t/cirrus/test/sim/postgres"
	storagesim "github.com/tjst-t/cirrus/test/sim/storage"
	"github.com/tjst-t/cirrus/test/sim/webui"
)

// version is set at build time via -ldflags.
var version = "dev"

// Shutdowner is implemented by all simulator servers.
type Shutdowner interface {
	Start()
	Shutdown(ctx context.Context) error
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	commonPort := flag.String("common", envOrDefault("COMMON_PORT", "8000"), "common API port")
	libvirtPort := flag.String("libvirt", envOrDefault("LIBVIRT_SIM_PORT", "8100"), "libvirt-sim management port")
	awxPort := flag.String("awx", envOrDefault("AWX_SIM_PORT", "8300"), "awx-sim port")
	storagePort := flag.String("storage", envOrDefault("STORAGE_SIM_PORT", "8500"), "storage-sim port")
	dashboardPort := flag.String("dashboard", envOrDefault("DASHBOARD_PORT", "8080"), "dashboard web UI port")
	aggregatorPort := flag.String("aggregator", envOrDefault("AGGREGATOR_PORT", "8090"), "aggregator dashboard port")
	postgresPort := flag.String("postgres", envOrDefault("POSTGRES_PORT", "5432"), "embedded PostgreSQL port")
	postgresMgmtPort := flag.String("postgres-mgmt", envOrDefault("POSTGRES_MGMT_PORT", "8600"), "PostgreSQL management API port")
	envFile := flag.String("env", envOrDefault("CIRRUS_SIM_ENV", ""), "environment YAML file to seed on startup")
	flag.Parse()

	if *showVersion {
		fmt.Println("cirrus-sim", version)
		os.Exit(0)
	}

	webui.Version = version

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Build endpoint map for dashboard proxy
	endpoints := webui.Endpoints{
		"common":      fmt.Sprintf("http://localhost:%s", *commonPort),
		"libvirt-sim": fmt.Sprintf("http://localhost:%s", *libvirtPort),
		"awx-sim":     fmt.Sprintf("http://localhost:%s", *awxPort),
		"storage-sim": fmt.Sprintf("http://localhost:%s", *storagePort),
		"postgres":    fmt.Sprintf("http://localhost:%s", *postgresMgmtPort),
	}

	// Create simulator instances
	pgSim := pgsim.New(*postgresPort, *postgresMgmtPort, logger.With("sim", "postgres"))
	libvirtSim := libvirtsim.New(*libvirtPort, logger.With("sim", "libvirt-sim"))
	storageSim := storagesim.New(*storagePort, logger.With("sim", "storage-sim"))

	// Create aggregator with endpoints pointing to local simulators
	aggregator.Version = version
	aggEndpoints := aggregator.Endpoints{
		Workers:    []string{fmt.Sprintf("http://localhost:%s", *libvirtPort)},
		StorageSim: fmt.Sprintf("http://localhost:%s", *storagePort),
		AWXSim:     fmt.Sprintf("http://localhost:%s", *awxPort),
		CommonSim:  fmt.Sprintf("http://localhost:%s", *commonPort),
		PostgreSim: fmt.Sprintf("http://localhost:%s", *postgresMgmtPort),
	}

	sims := []struct {
		name string
		srv  Shutdowner
	}{
		{"postgres", pgSim},
		{"common", common.New(*commonPort, logger.With("sim", "common"))},
		{"libvirt-sim", libvirtSim},
		{"awx-sim", awxsim.New(*awxPort, logger.With("sim", "awx-sim"))},
		{"storage-sim", storageSim},
		{"dashboard", webui.New(*dashboardPort, endpoints, logger.With("sim", "dashboard"))},
		{"aggregator", aggregator.New(*aggregatorPort, aggEndpoints, logger.With("sim", "aggregator"))},
	}

	logger.Info("starting cirrus-sim (unified)",
		"postgres", *postgresPort,
		"common", *commonPort,
		"libvirt-sim", *libvirtPort,
		"awx-sim", *awxPort,
		"storage-sim", *storagePort,
		"dashboard", *dashboardPort,
		"aggregator", *aggregatorPort,
	)

	for _, s := range sims {
		s.srv.Start()
	}

	// Seed environment if specified
	if *envFile != "" {
		ctx := context.Background()
		if err := seedFromEnvFile(ctx, *envFile, libvirtSim, storageSim, logger); err != nil {
			logger.Error("environment seeding failed", "file", *envFile, "error", err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  cirrus-sim %s is running\n", version)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Dashboard (legacy)       http://localhost:%s\n", *dashboardPort)
	fmt.Fprintf(os.Stderr, "  Aggregator Dashboard     http://localhost:%s\n", *aggregatorPort)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  postgres                 %s\n", pgSim.ConnectionURL())
	fmt.Fprintf(os.Stderr, "  common (events/faults)   http://localhost:%s\n", *commonPort)
	fmt.Fprintf(os.Stderr, "  libvirt-sim (management) http://localhost:%s\n", *libvirtPort)
	fmt.Fprintf(os.Stderr, "  awx-sim                  http://localhost:%s\n", *awxPort)
	fmt.Fprintf(os.Stderr, "  storage-sim              http://localhost:%s\n", *storagePort)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	if *envFile != "" {
		fmt.Fprintf(os.Stderr, "  Environment              %s\n", *envFile)
		fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	}
	fmt.Fprintf(os.Stderr, "  Press Ctrl+C to stop\n\n")

	// Wait for signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutting down all simulators")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := len(sims) - 1; i >= 0; i-- {
		s := sims[i]
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown failed", "sim", s.name, "error", err)
		} else {
			logger.Info("stopped", "sim", s.name)
		}
	}
	logger.Info("cirrus-sim stopped")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
