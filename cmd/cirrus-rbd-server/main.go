// cmd/cirrus-rbd-server is a lightweight HTTP management API that wraps
// rbd/ceph CLI tools to provide Ceph RBD lifecycle operations for Cirrus
// storage integration tests.
//
// API:
//
//	POST   /volumes                 – rbd create image in pool
//	DELETE /volumes/{id}            – rbd rm image
//	POST   /volumes/{id}/export     – create client auth key, return keyring+monitor
//	DELETE /volumes/{id}/export     – remove client auth key
//	GET    /volumes/{id}            – get image info
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const defaultPool = "cirrus"

type imageState struct {
	VolumeID string
	SizeGB   int64
	Clients  map[string]struct{} // client IDs with active keyrings
}

var (
	mu     sync.Mutex
	images = map[string]*imageState{}
)

var (
	pool      = envOrDefault("CEPH_POOL", defaultPool)
	monitorIP = envOrDefault("MONITOR_IP", "127.0.0.1")
	listenOn  = envOrDefault("LISTEN_ADDR", ":8080")
	logger    = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
)

func main() {
	// Ensure the Ceph pool exists.
	if err := ensurePool(); err != nil {
		logger.Error("ensure pool", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /volumes", handleCreateVolume)
	mux.HandleFunc("DELETE /volumes/{id}", handleDeleteVolume)
	mux.HandleFunc("POST /volumes/{id}/export", handleExportVolume)
	mux.HandleFunc("DELETE /volumes/{id}/export", handleUnexportVolume)
	mux.HandleFunc("GET /volumes/{id}", handleGetVolume)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger.Info("cirrus-rbd-server ready", "addr", listenOn, "pool", pool, "monitor", monitorIP)
	if err := http.ListenAndServe(listenOn, mux); err != nil {
		logger.Error("http server", "err", err)
		os.Exit(1)
	}
}

func ensurePool() error {
	// Check if pool exists first.
	if err := ceph("osd", "pool", "ls"); err != nil {
		return fmt.Errorf("ceph not reachable: %w", err)
	}
	out, _ := exec.Command("ceph", "osd", "pool", "ls").Output()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == pool {
			logger.Info("pool exists", "pool", pool)
			return nil
		}
	}
	if err := ceph("osd", "pool", "create", pool, "32"); err != nil {
		return fmt.Errorf("create pool: %w", err)
	}
	if err := ceph("osd", "pool", "application", "enable", pool, "rbd"); err != nil {
		return fmt.Errorf("enable rbd on pool: %w", err)
	}
	logger.Info("pool created", "pool", pool)
	return nil
}

// ─── handlers ────────────────────────────────────────────────────────────────

func handleCreateVolume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VolumeID string `json:"volume_id"`
		SizeGB   int64  `json:"size_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.VolumeID == "" || req.SizeGB <= 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := images[req.VolumeID]; exists {
		http.Error(w, "volume already exists", http.StatusConflict)
		return
	}

	sizeMB := fmt.Sprintf("%dM", req.SizeGB*1024)
	if err := rbd("create", "--pool", pool, "--size", sizeMB, req.VolumeID); err != nil {
		logger.Error("rbd create", "err", err)
		http.Error(w, "rbd create failed", http.StatusInternalServerError)
		return
	}

	images[req.VolumeID] = &imageState{
		VolumeID: req.VolumeID,
		SizeGB:   req.SizeGB,
		Clients:  make(map[string]struct{}),
	}

	logger.Info("volume created", "volume_id", req.VolumeID, "size_gb", req.SizeGB)
	writeJSON(w, http.StatusCreated, map[string]any{
		"volume_id": req.VolumeID,
		"pool":      pool,
		"size_gb":   req.SizeGB,
	})
}

func handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	mu.Lock()
	defer mu.Unlock()

	if _, ok := images[id]; !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	if err := rbd("rm", pool+"/"+id); err != nil {
		logger.Error("rbd rm", "err", err)
		http.Error(w, "rbd rm failed", http.StatusInternalServerError)
		return
	}

	delete(images, id)
	logger.Info("volume deleted", "volume_id", id)
	w.WriteHeader(http.StatusNoContent)
}

func handleExportVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		ClientID string `json:"client_id"` // host UUID
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClientID == "" {
		http.Error(w, "client_id required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := images[id]; !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	cephClientID := fmt.Sprintf("client.cirrus.%s", req.ClientID)
	// Grant read/write access on this specific image.
	capsOSD := fmt.Sprintf("profile rbd pool=%s", pool)
	keyring, err := cephAuthGetOrCreate(cephClientID,
		"mon", "profile rbd",
		"osd", capsOSD,
		"mgr", "profile rbd",
	)
	if err != nil {
		logger.Error("ceph auth", "err", err)
		http.Error(w, "create auth key failed", http.StatusInternalServerError)
		return
	}

	images[id].Clients[req.ClientID] = struct{}{}

	logger.Info("volume exported", "volume_id", id, "client", req.ClientID)
	writeJSON(w, http.StatusOK, map[string]any{
		"monitor":   fmt.Sprintf("%s:6789", monitorIP),
		"pool":      pool,
		"image":     id,
		"keyring":   keyring,
		"client_id": cephClientID,
	})
}

func handleUnexportVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClientID == "" {
		http.Error(w, "client_id required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	img, ok := images[id]
	if !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	cephClientID := fmt.Sprintf("client.cirrus.%s", req.ClientID)
	if err := ceph("auth", "del", cephClientID); err != nil {
		logger.Error("ceph auth del", "err", err)
		http.Error(w, "delete auth key failed", http.StatusInternalServerError)
		return
	}

	delete(img.Clients, req.ClientID)
	logger.Info("volume unexported", "volume_id", id, "client", req.ClientID)
	w.WriteHeader(http.StatusNoContent)
}

func handleGetVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	mu.Lock()
	img, ok := images[id]
	mu.Unlock()

	if !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	clients := make([]string, 0, len(img.Clients))
	for c := range img.Clients {
		clients = append(clients, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"volume_id": id,
		"pool":      pool,
		"size_gb":   img.SizeGB,
		"monitor":   fmt.Sprintf("%s:6789", monitorIP),
		"clients":   clients,
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func rbd(args ...string) error {
	out, err := exec.Command("rbd", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("rbd %s: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

func ceph(args ...string) error {
	out, err := exec.Command("ceph", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ceph %s: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// cephAuthGetOrCreate creates or retrieves a Ceph client keyring and returns
// the base64-encoded key string.
func cephAuthGetOrCreate(entity string, caps ...string) (string, error) {
	args := append([]string{"auth", "get-or-create", entity, "--format", "json"}, caps...)
	out, err := exec.Command("ceph", args...).Output()
	if err != nil {
		return "", fmt.Errorf("ceph auth get-or-create %s: %w", entity, err)
	}
	var result []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result) == 0 {
		return "", fmt.Errorf("parse ceph auth output: %w", err)
	}
	return result[0].Key, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
