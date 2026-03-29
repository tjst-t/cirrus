package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/datagen"
	libvirtsim "github.com/tjst-t/cirrus/test/sim/libvirt"
	storagesim "github.com/tjst-t/cirrus/test/sim/storage"
)

// portRange holds a start-end port range from portman.
type portRange struct {
	start int
	end   int
}

// getPortRange reads PORT_START/PORT_END env vars for a given name.
// Returns zero range if not set (fallback to defaults).
func getPortRange(envPrefix string) portRange {
	startStr := os.Getenv(envPrefix + "_PORT_START")
	endStr := os.Getenv(envPrefix + "_PORT_END")
	if startStr == "" || endStr == "" {
		return portRange{}
	}
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil {
		return portRange{}
	}
	return portRange{start: start, end: end}
}

// seedFromEnvFile loads an environment YAML file and seeds all simulators with the generated data.
func seedFromEnvFile(
	ctx context.Context,
	path string,
	libvirt *libvirtsim.Server,
	storage *storagesim.Server,
	logger *slog.Logger,
) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read environment file: %w", err)
	}

	// Parse YAML for environment definitions
	var envDef datagen.EnvironmentDef
	if err := yaml.Unmarshal(data, &envDef); err != nil {
		return fmt.Errorf("parse environment YAML: %w", err)
	}

	// Check for portman-managed port ranges
	libvirtRange := getPortRange("LIBVIRT_HOSTS")

	// Generate hosts and backends
	gen := datagen.New()
	opts := datagen.GenerateOptions{
		LibvirtBasePort: libvirtRange.start,
	}
	result, err := gen.GenerateWithOptions(ctx, data, opts)
	if err != nil {
		return fmt.Errorf("generate environment: %w", err)
	}

	logger.Info("seeding environment", "name", result.Name, "hosts", len(result.Hosts), "backends", len(result.Backends))

	if libvirtRange.start > 0 {
		logger.Info("using portman range for libvirt hosts", "start", libvirtRange.start, "end", libvirtRange.end)
	}

	// Seed libvirt-sim hosts
	for _, h := range result.Hosts {
		cfg := libvirtsim.HostConfig{
			HostID:             h.HostID,
			LibvirtPort:        h.LibvirtPort,
			CPUModel:           h.CPUModel,
			CPUSockets:         h.CPUSockets,
			CoresPerSocket:     h.CoresPerSocket,
			ThreadsPerCore:     h.ThreadsPerCore,
			MemoryMB:           int64(h.MemoryMB),
			CPUOvercommitRatio: 4.0,
			MemOvercommitRatio: 1.5,
		}
		if err := libvirt.SeedHost(ctx, cfg); err != nil {
			return fmt.Errorf("seed host %s: %w", h.HostID, err)
		}
	}
	logger.Info("seeded libvirt-sim", "hosts", len(result.Hosts))

	// Seed storage backends
	for _, b := range result.Backends {
		cfg := storagesim.BackendConfig{
			BackendID:          b.BackendID,
			TotalCapacityGB:    b.TotalCapacityGB,
			TotalIOPS:          b.TotalIOPS,
			Capabilities:       b.Capabilities,
			OverprovisionRatio: 1.5,
		}
		if err := storage.SeedBackend(cfg); err != nil {
			return fmt.Errorf("seed backend %s: %w", b.BackendID, err)
		}
	}
	if len(result.Backends) > 0 {
		logger.Info("seeded storage-sim", "backends", len(result.Backends))
	}

	logger.Info("environment seeding complete", "name", result.Name)
	return nil
}
