package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/datagen"
)

// runSeedCommand implements the "cirrus-sim seed" subcommand.
// It reads an env YAML, generates hosts/backends, and seeds the running
// cirrus-sim via HTTP management APIs. Idempotent: 409 responses are silently
// skipped so it's safe to run multiple times.
func runSeedCommand(args []string) {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	envFile := fs.String("env", envOrDefault("CIRRUS_SIM_ENV", ""), "environment YAML file to seed")
	libvirtURL := fs.String("libvirt-url", envOrDefault("LIBVIRT_SIM_URL", ""), "libvirt-sim management URL (e.g. http://localhost:8100)")
	storageURL := fs.String("storage-url", envOrDefault("STORAGE_SIM_URL", ""), "storage-sim management URL (e.g. http://localhost:8500)")
	fs.Parse(args) //nolint:errcheck

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *envFile == "" {
		fmt.Fprintln(os.Stderr, "cirrus-sim seed: -env is required")
		os.Exit(1)
	}
	if *libvirtURL == "" || *storageURL == "" {
		fmt.Fprintln(os.Stderr, "cirrus-sim seed: -libvirt-url and -storage-url are required")
		os.Exit(1)
	}

	data, err := os.ReadFile(*envFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read env file: %v\n", err)
		os.Exit(1)
	}

	var envDef datagen.EnvironmentDef
	if err := yaml.Unmarshal(data, &envDef); err != nil {
		fmt.Fprintf(os.Stderr, "parse env YAML: %v\n", err)
		os.Exit(1)
	}

	libvirtRange := getPortRange("LIBVIRT_HOSTS")
	ctx := context.Background()
	gen := datagen.New()
	opts := datagen.GenerateOptions{LibvirtBasePort: libvirtRange.start}
	result, err := gen.GenerateWithOptions(ctx, data, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate environment: %v\n", err)
		os.Exit(1)
	}

	logger.Info("seeding environment via HTTP API",
		"name", result.Name,
		"hosts", len(result.Hosts),
		"backends", len(result.Backends),
	)

	client := &http.Client{Timeout: 10 * time.Second}

	// Seed libvirt-sim hosts.
	for _, h := range result.Hosts {
		body := map[string]interface{}{
			"host_id":              h.HostID,
			"libvirt_port":         h.LibvirtPort,
			"cpu_model":            h.CPUModel,
			"cpu_sockets":          h.CPUSockets,
			"cores_per_socket":     h.CoresPerSocket,
			"threads_per_core":     h.ThreadsPerCore,
			"memory_mb":            h.MemoryMB,
			"cpu_overcommit_ratio": 4.0,
			"memory_overcommit_ratio": 1.5,
		}
		if err := postJSON(client, *libvirtURL+"/sim/hosts", body); err != nil {
			if isConflict(err) {
				logger.Info("host already registered, skipping", "host_id", h.HostID)
				continue
			}
			logger.Error("seed host", "host_id", h.HostID, "error", err)
			os.Exit(1)
		}
		logger.Info("seeded host", "host_id", h.HostID, "port", h.LibvirtPort)
	}

	// Seed storage-sim backends.
	for _, b := range result.Backends {
		caps, _ := json.Marshal(b.Capabilities)
		body := map[string]interface{}{
			"backend_id":          b.BackendID,
			"total_capacity_gb":   b.TotalCapacityGB,
			"total_iops":          b.TotalIOPS,
			"capabilities":        json.RawMessage(caps),
			"overprovision_ratio": 1.5,
		}
		if err := postJSON(client, *storageURL+"/sim/backends", body); err != nil {
			if isConflict(err) {
				logger.Info("backend already registered, skipping", "backend_id", b.BackendID)
				continue
			}
			logger.Error("seed backend", "backend_id", b.BackendID, "error", err)
			os.Exit(1)
		}
		logger.Info("seeded backend", "backend_id", b.BackendID, "capacity_gb", b.TotalCapacityGB)
	}

	logger.Info("environment seeding complete", "name", result.Name)
}

// postJSON sends a POST request with a JSON body and returns an error on non-2xx.
func postJSON(client *http.Client, url string, body interface{}) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return errConflict
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s: HTTP %d", url, resp.StatusCode)
	}
	return nil
}

// errConflict is a sentinel for 409 responses.
var errConflict = fmt.Errorf("conflict")

func isConflict(err error) bool {
	return err == errConflict
}
