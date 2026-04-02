// cmd/cirrus-iscsi-server is a lightweight HTTP management API that wraps
// tgtd/tgtadm to provide iSCSI target lifecycle operations for Cirrus storage
// integration tests.
//
// API:
//
//	POST   /volumes                       – allocate backing file + tgt target + LUN
//	DELETE /volumes/{id}                  – delete LUN, target, and backing file
//	POST   /volumes/{id}/export           – bind initiator IP, return portal/IQN/LUN
//	DELETE /volumes/{id}/export           – unbind initiator IP
//	GET    /volumes/{id}                  – get target info
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	volumeDir  = "/var/lib/cirrus-iscsi"
	iqnPrefix  = "iqn.2024-01.io.cirrus:volume"
	defaultLUN = 1
)

type targetState struct {
	TID      int
	IQN      string
	FilePath string
	SizeGB   int64
	BoundIPs map[string]struct{}
}

var (
	mu      sync.Mutex
	targets = map[string]*targetState{} // volume_id → state
	tidSeq  atomic.Int64
)

var (
	portalIP = envOrDefault("PORTAL_IP", "127.0.0.1")
	listenOn = envOrDefault("LISTEN_ADDR", ":8080")
	logger   = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
)

func main() {
	if err := os.MkdirAll(volumeDir, 0o755); err != nil {
		logger.Error("mkdir volume dir", "err", err)
		os.Exit(1)
	}

	if err := startTgtd(); err != nil {
		logger.Error("start tgtd", "err", err)
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

	logger.Info("cirrus-iscsi-server ready", "addr", listenOn, "portal", portalIP)
	if err := http.ListenAndServe(listenOn, mux); err != nil {
		logger.Error("http server", "err", err)
		os.Exit(1)
	}
}

// startTgtd launches tgtd if not already running and waits for it to be ready.
func startTgtd() error {
	// Check if tgtd is already running.
	if err := tgtadm("--op", "show", "--mode", "system"); err == nil {
		logger.Info("tgtd already running")
		return nil
	}

	cmd := exec.Command("tgtd") // tgtd daemonizes by default
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start tgtd: %w", err)
	}
	go cmd.Wait() // reap child process to avoid zombies

	// Wait up to 5 s for tgtd to become reachable.
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := tgtadm("--op", "show", "--mode", "system"); err == nil {
			logger.Info("tgtd started")
			return nil
		}
	}
	return fmt.Errorf("tgtd did not become ready in time")
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

	if _, exists := targets[req.VolumeID]; exists {
		http.Error(w, "volume already exists", http.StatusConflict)
		return
	}

	tid := int(tidSeq.Add(1))
	iqn := fmt.Sprintf("%s.%s", iqnPrefix, req.VolumeID)
	filePath := filepath.Join(volumeDir, req.VolumeID)

	// Create sparse backing file.
	if out, err := exec.Command("truncate", "-s", fmt.Sprintf("%dG", req.SizeGB), filePath).CombinedOutput(); err != nil {
		logger.Error("truncate", "err", err, "out", string(out))
		http.Error(w, "create backing file failed", http.StatusInternalServerError)
		return
	}

	// Create iSCSI target.
	if err := tgtadm("--lld", "iscsi", "--op", "new", "--mode", "target",
		"--tid", strconv.Itoa(tid), "--targetname", iqn); err != nil {
		_ = os.Remove(filePath)
		logger.Error("tgt new target", "err", err)
		http.Error(w, "create target failed", http.StatusInternalServerError)
		return
	}

	// Add LUN.
	if err := tgtadm("--lld", "iscsi", "--op", "new", "--mode", "logicalunit",
		"--tid", strconv.Itoa(tid), "--lun", strconv.Itoa(defaultLUN), "-b", filePath); err != nil {
		_ = tgtadm("--lld", "iscsi", "--op", "delete", "--mode", "target", "--tid", strconv.Itoa(tid))
		_ = os.Remove(filePath)
		logger.Error("tgt add lun", "err", err)
		http.Error(w, "add LUN failed", http.StatusInternalServerError)
		return
	}

	targets[req.VolumeID] = &targetState{
		TID:      tid,
		IQN:      iqn,
		FilePath: filePath,
		SizeGB:   req.SizeGB,
		BoundIPs: make(map[string]struct{}),
	}

	logger.Info("volume created", "volume_id", req.VolumeID, "tid", tid, "iqn", iqn)
	writeJSON(w, http.StatusCreated, map[string]any{
		"volume_id": req.VolumeID,
		"tid":       tid,
		"iqn":       iqn,
	})
}

func handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	mu.Lock()
	defer mu.Unlock()

	ts, ok := targets[id]
	if !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	// Remove LUN then target.
	_ = tgtadm("--lld", "iscsi", "--op", "delete", "--mode", "logicalunit",
		"--tid", strconv.Itoa(ts.TID), "--lun", strconv.Itoa(defaultLUN))
	_ = tgtadm("--lld", "iscsi", "--op", "delete", "--mode", "target",
		"--tid", strconv.Itoa(ts.TID))
	_ = os.Remove(ts.FilePath)

	delete(targets, id)
	logger.Info("volume deleted", "volume_id", id)
	w.WriteHeader(http.StatusNoContent)
}

func handleExportVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		InitiatorIP string `json:"initiator_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.InitiatorIP == "" {
		http.Error(w, "initiator_ip required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	ts, ok := targets[id]
	if !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	if err := tgtadm("--lld", "iscsi", "--op", "bind", "--mode", "target",
		"--tid", strconv.Itoa(ts.TID), "-I", req.InitiatorIP); err != nil {
		logger.Error("tgt bind", "err", err)
		http.Error(w, "bind initiator failed", http.StatusInternalServerError)
		return
	}

	ts.BoundIPs[req.InitiatorIP] = struct{}{}
	portal := fmt.Sprintf("%s:3260", portalIP)

	logger.Info("volume exported", "volume_id", id, "initiator", req.InitiatorIP)
	writeJSON(w, http.StatusOK, map[string]any{
		"target": ts.IQN,
		"portal": portal,
		"lun":    defaultLUN,
	})
}

func handleUnexportVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		InitiatorIP string `json:"initiator_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.InitiatorIP == "" {
		http.Error(w, "initiator_ip required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	ts, ok := targets[id]
	if !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	if err := tgtadm("--lld", "iscsi", "--op", "unbind", "--mode", "target",
		"--tid", strconv.Itoa(ts.TID), "-I", req.InitiatorIP); err != nil {
		logger.Error("tgt unbind", "err", err)
		http.Error(w, "unbind initiator failed", http.StatusInternalServerError)
		return
	}

	delete(ts.BoundIPs, req.InitiatorIP)
	logger.Info("volume unexported", "volume_id", id, "initiator", req.InitiatorIP)
	w.WriteHeader(http.StatusNoContent)
}

func handleGetVolume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	mu.Lock()
	ts, ok := targets[id]
	mu.Unlock()

	if !ok {
		http.Error(w, "volume not found", http.StatusNotFound)
		return
	}

	ips := make([]string, 0, len(ts.BoundIPs))
	for ip := range ts.BoundIPs {
		ips = append(ips, ip)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"volume_id":   id,
		"tid":         ts.TID,
		"iqn":         ts.IQN,
		"size_gb":     ts.SizeGB,
		"bound_ips":   ips,
		"portal":      fmt.Sprintf("%s:3260", portalIP),
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func tgtadm(args ...string) error {
	out, err := exec.Command("tgtadm", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tgtadm %s: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return nil
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
