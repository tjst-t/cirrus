package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tjst-t/cirrus/internal/agent"
	"github.com/tjst-t/cirrus/internal/api"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/compute/libvirt"
	computestub "github.com/tjst-t/cirrus/internal/compute/stub"
	"github.com/tjst-t/cirrus/internal/config"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/network/ovs"
	networkstub "github.com/tjst-t/cirrus/internal/network/stub"
	"github.com/tjst-t/cirrus/internal/scheduler"
	"github.com/tjst-t/cirrus/internal/state"
	"github.com/tjst-t/cirrus/internal/storage"
	localstore "github.com/tjst-t/cirrus/internal/storage/local"
	storagestub "github.com/tjst-t/cirrus/internal/storage/stub"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: cirrus <controller|worker> [--config=cirrus.yaml]\n")
		os.Exit(1)
	}

	role := os.Args[1]
	configPath := "cirrus.yaml"
	for _, arg := range os.Args[2:] {
		if len(arg) > 9 && arg[:9] == "--config=" {
			configPath = arg[9:]
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	cfg.Role = role

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	switch role {
	case "controller":
		if err := runController(ctx, cfg, logger); err != nil {
			logger.Error("controller failed", "error", err)
			os.Exit(1)
		}
	case "worker":
		if err := runWorker(ctx, cfg, logger); err != nil {
			logger.Error("worker failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown role: %s (use controller or worker)\n", role)
		os.Exit(1)
	}

	<-sigCh
	logger.Info("shutting down...")
	cancel()
}

func runController(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	db, err := state.NewDB(ctx, cfg.Controller.DB)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	logger.Info("database migrated successfully")

	sched := scheduler.New(db)
	handler := api.New(db, sched, logger)

	server := &http.Server{
		Addr:    cfg.Listen,
		Handler: handler.Router(),
	}

	go func() {
		logger.Info("controller API listening", "addr", cfg.Listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("API server error", "error", err)
		}
	}()

	return nil
}

func runWorker(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	var (
		computeDriver  compute.Driver
		networkProvider network.Provider
		storageBackend storage.Backend
	)

	if cfg.Compute.Driver == "stub" {
		logger.Info("using stub backends")
		computeDriver = computestub.New(logger)
		networkProvider = networkstub.New(logger)
		storageBackend = storagestub.New(logger)
	} else {
		computeDriver = libvirt.New(cfg.Compute.URI, "")
		networkProvider = ovs.New(cfg.Network.Bridge, cfg.Network.LocalIP)
		storageBackend = localstore.New(cfg.Storage.DiskDir, cfg.Storage.ImageDir)
	}

	// Initialize bridge
	if err := networkProvider.InitBridge(ctx); err != nil {
		logger.Warn("init bridge failed (may already exist)", "error", err)
	}

	// Create agent
	agentSrv := agent.New(computeDriver, networkProvider, storageBackend, logger)

	// Register with controller
	go func() {
		for {
			if err := registerWorker(cfg, logger); err != nil {
				logger.Error("register with controller failed", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break
		}
		// Start heartbeat loop
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sendHeartbeat(cfg, logger)
			}
		}
	}()

	// Start gRPC server
	go func() {
		if err := agentSrv.Serve(cfg.Listen); err != nil {
			logger.Error("agent server failed", "error", err)
		}
	}()

	return nil
}

func registerWorker(cfg *config.Config, logger *slog.Logger) error {
	addr := cfg.Listen
	if cfg.Advertise != "" {
		addr = cfg.Advertise
	}
	body, _ := json.Marshal(map[string]any{
		"name":    cfg.Worker.Name,
		"address": addr,
		"vcpus":   cfg.Worker.VCPUs,
		"ram_mb":  cfg.Worker.RamMB,
		"disk_gb": cfg.Worker.DiskGB,
	})
	url := fmt.Sprintf("http://%s/api/v1/workers/register", cfg.Worker.ControllerAddr)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register returned %d", resp.StatusCode)
	}
	logger.Info("registered with controller", "name", cfg.Worker.Name)
	return nil
}

func sendHeartbeat(cfg *config.Config, logger *slog.Logger) {
	body, _ := json.Marshal(map[string]string{"name": cfg.Worker.Name})
	url := fmt.Sprintf("http://%s/api/v1/workers/heartbeat", cfg.Worker.ControllerAddr)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Warn("heartbeat failed", "error", err)
		return
	}
	resp.Body.Close()
}
