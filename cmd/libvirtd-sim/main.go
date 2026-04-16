// Package main provides the standalone libvirtd-sim binary for single-host mode.
// Each worker container runs one instance of this binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	libvirtsim "github.com/tjst-t/cirrus/test/sim/libvirt"
)

func main() {
	hostID := flag.String("host-id", envOrDefault("HOST_ID", ""), "host identifier")
	libvirtPort := flag.Int("libvirt-port", intEnvOrDefault("LIBVIRT_PORT", 16509), "libvirt RPC port")
	mgmtPort := flag.String("mgmt-port", envOrDefault("LIBVIRTD_SIM_MGMT_PORT", "8100"), "management API port")
	cpuModel := flag.String("cpu-model", envOrDefault("CPU_MODEL", "Intel Xeon Gold 6348"), "CPU model name")
	cpuSockets := flag.Int("cpu-sockets", intEnvOrDefault("CPU_SOCKETS", 2), "CPU sockets")
	coresPerSocket := flag.Int("cores-per-socket", intEnvOrDefault("CORES_PER_SOCKET", 28), "cores per socket")
	threadsPerCore := flag.Int("threads-per-core", intEnvOrDefault("THREADS_PER_CORE", 2), "threads per core")
	memoryMB := flag.Int64("memory-mb", int64EnvOrDefault("MEMORY_MB", 524288), "memory in MB")
	enableNetns := flag.Bool("enable-netns", false, "enable network namespace operations (requires privileges)")
	ovsBridge := flag.String("ovs-bridge", envOrDefault("OVS_BRIDGE", "br-int"), "OVS bridge name for namespace connectivity")
	dbDSN := flag.String("db-dsn", envOrDefault("DB_DSN", ""), "postgres DSN for state persistence (empty = in-memory only)")
	flag.Parse()

	if *hostID == "" {
		fmt.Fprintln(os.Stderr, "error: --host-id is required")
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := libvirtsim.HostInstanceConfig{
		HostID:             *hostID,
		LibvirtPort:        *libvirtPort,
		MgmtPort:           *mgmtPort,
		CPUModel:           *cpuModel,
		CPUSockets:         *cpuSockets,
		CoresPerSocket:     *coresPerSocket,
		ThreadsPerCore:     *threadsPerCore,
		MemoryMB:           *memoryMB,
		CPUOvercommitRatio: 4.0,
		MemOvercommitRatio: 1.5,
		OVSBridge:          *ovsBridge,
		EnableNetns:        *enableNetns,
		DBDSN:              *dbDSN,
	}

	instance := libvirtsim.NewHostInstance(cfg, logger)
	instance.Start()

	logger.Info("libvirtd-sim (single-host) running",
		"host_id", *hostID,
		"libvirt_port", *libvirtPort,
		"mgmt_port", *mgmtPort,
		"enable_netns", *enableNetns,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := instance.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnvOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: invalid integer for %s=%q, using default %d\n", key, v, def)
			return def
		}
		return n
	}
	return def
}

func int64EnvOrDefault(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: invalid integer for %s=%q, using default %d\n", key, v, def)
			return def
		}
		return n
	}
	return def
}
