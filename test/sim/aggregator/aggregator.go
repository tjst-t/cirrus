// Package aggregator provides a process that aggregates state from distributed
// simulator instances (libvirtd-sim per worker, storage-sim, awx-sim).
//
// It exposes:
//   - Aggregated REST API (/sim/overview, /sim/hosts, /sim/events, /sim/faults)
//   - Dashboard Web UI
package aggregator

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

//go:embed static
var staticFiles embed.FS

// Version is set at build time.
var Version = "dev"

// Endpoints maps simulator names to their base URLs.
type Endpoints struct {
	Workers    []string // libvirtd-sim management API URLs (one per worker)
	StorageSim string
	AWXSim     string
	CommonSim  string
	PostgreSim string
}

// Server is the aggregator server.
type Server struct {
	httpServer *http.Server
	endpoints  Endpoints
	logger     *slog.Logger

	mu    sync.RWMutex
	cache *AggregatedState
}

// AggregatedState holds the latest polled state from all simulators.
type AggregatedState struct {
	Hosts       []HostInfo      `json:"hosts"`
	Domains     []DomainInfo    `json:"domains"`
	StorageStats json.RawMessage `json:"storage_stats,omitempty"`
	AWXStats     json.RawMessage `json:"awx_stats,omitempty"`
	PostgreStats json.RawMessage `json:"postgres_stats,omitempty"`
	CommonStats  json.RawMessage `json:"common_stats,omitempty"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// HostInfo represents an aggregated host.
type HostInfo struct {
	HostID             string  `json:"host_id"`
	LibvirtPort        int     `json:"libvirt_port"`
	State              string  `json:"state"`
	CPUModel           string  `json:"cpu_model,omitempty"`
	CPUSockets         int     `json:"cpu_sockets"`
	CoresPerSocket     int     `json:"cores_per_socket"`
	ThreadsPerCore     int     `json:"threads_per_core"`
	MemoryMB           int64   `json:"memory_mb"`
	UsedVCPUs          int     `json:"used_vcpus"`
	UsedMemoryMB       int64   `json:"used_memory_mb"`
	DomainCount        int     `json:"domain_count"`
	CPUOvercommitRatio float64 `json:"cpu_overcommit_ratio"`
	MemOvercommitRatio float64 `json:"memory_overcommit_ratio"`
	WorkerURL          string  `json:"worker_url"`
}

// DomainInfo represents a VM domain.
type DomainInfo struct {
	Name         string   `json:"name"`
	UUID         string   `json:"uuid"`
	State        int32    `json:"state"`
	VCPUs        int      `json:"vcpus"`
	MemoryKiB    int64    `json:"memory_kib"`
	HostID       string   `json:"host_id"`
	InterfaceIDs []string `json:"interface_ids,omitempty"`
}

// Overview is returned by GET /sim/overview.
type Overview struct {
	TotalHosts     int             `json:"total_hosts"`
	OnlineHosts    int             `json:"online_hosts"`
	TotalDomains   int             `json:"total_domains"`
	RunningDomains int             `json:"running_domains"`
	TotalVCPUsUsed int             `json:"total_vcpus_used"`
	TotalMemUsedMB int64           `json:"total_memory_used_mb"`
	Workers        int             `json:"workers"`
	StorageStats   json.RawMessage `json:"storage_stats,omitempty"`
	AWXStats       json.RawMessage `json:"awx_stats,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// New creates a new aggregator Server.
func New(port string, endpoints Endpoints, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		endpoints: endpoints,
		logger:    logger,
		cache:     &AggregatedState{},
	}

	mux := http.NewServeMux()

	// Static files for dashboard
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServer(http.FS(staticFS)))

	// API endpoints
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("GET /sim/overview", s.handleOverview)
	mux.HandleFunc("GET /sim/hosts", s.handleListHosts)
	mux.HandleFunc("GET /sim/hosts/{host_id}/domains", s.handleListHostDomains)
	mux.HandleFunc("GET /sim/domains", s.handleListAllDomains)
	mux.HandleFunc("GET /sim/events", s.handleEvents)
	mux.HandleFunc("GET /sim/faults", s.handleFaults)

	// Fault injection proxy (POST/DELETE to common API)
	mux.HandleFunc("POST /sim/faults", s.handleProxyFaultAdd)
	mux.HandleFunc("DELETE /sim/faults", s.handleProxyFaultClear)
	mux.HandleFunc("DELETE /sim/faults/{id}", s.handleProxyFaultDelete)

	// Snapshot proxy (common API)
	mux.HandleFunc("POST /sim/snapshots", s.handleProxySnapshotSave)
	mux.HandleFunc("GET /sim/snapshots", s.handleProxySnapshotList)
	mux.HandleFunc("POST /sim/snapshots/{id}/restore", s.handleProxySnapshotRestore)

	// Storage proxy
	mux.HandleFunc("GET /sim/storage/stats", s.handleProxyStorageStats)
	mux.HandleFunc("GET /sim/storage/backends", s.handleProxyStorageBackends)
	mux.HandleFunc("GET /sim/awx/stats", s.handleProxyAWXStats)
	mux.HandleFunc("GET /sim/postgres/stats", s.handleProxyPostgresStats)
	mux.HandleFunc("GET /sim/postgres/tables", s.handleProxyPostgresTables)
	mux.HandleFunc("GET /sim/postgres/tables/{name}", s.handleProxyPostgresTableData)
	mux.HandleFunc("GET /sim/postgres/tables/{name}/schema", s.handleProxyPostgresTableSchema)

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// Start starts the aggregator and begins polling.
func (s *Server) Start() {
	// Start polling loop
	go s.pollLoop()

	go func() {
		s.logger.Info("aggregator starting", "addr", s.httpServer.Addr, "workers", len(s.endpoints.Workers))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("aggregator server failed", "error", err)
		}
	}()
}

// Shutdown gracefully shuts down the aggregator.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) pollLoop() {
	for {
		s.poll()
		time.Sleep(3 * time.Second)
	}
}

func (s *Server) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex

	var allHosts []HostInfo
	var allDomains []DomainInfo

	// Poll each worker's libvirtd-sim
	for _, workerURL := range s.endpoints.Workers {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			// Fetch hosts
			var hosts []HostInfo
			if err := fetchJSON(ctx, url+"/sim/hosts", &hosts); err != nil {
				s.logger.Debug("failed to poll worker hosts", "url", url, "error", err)
				return
			}
			for i := range hosts {
				hosts[i].WorkerURL = url
			}

			// Fetch domains
			var domains []DomainInfo
			_ = fetchJSON(ctx, url+"/sim/domains", &domains)

			mu.Lock()
			allHosts = append(allHosts, hosts...)
			allDomains = append(allDomains, domains...)
			mu.Unlock()
		}(workerURL)
	}

	// Poll storage-sim
	var storageStats json.RawMessage
	if s.endpoints.StorageSim != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var raw json.RawMessage
			if err := fetchJSON(ctx, s.endpoints.StorageSim+"/sim/stats", &raw); err == nil {
				mu.Lock()
				storageStats = raw
				mu.Unlock()
			}
		}()
	}

	// Poll awx-sim
	var awxStats json.RawMessage
	if s.endpoints.AWXSim != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var raw json.RawMessage
			if err := fetchJSON(ctx, s.endpoints.AWXSim+"/sim/stats", &raw); err == nil {
				mu.Lock()
				awxStats = raw
				mu.Unlock()
			}
		}()
	}

	// Poll postgres
	var postgresStats json.RawMessage
	if s.endpoints.PostgreSim != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var raw json.RawMessage
			if err := fetchJSON(ctx, s.endpoints.PostgreSim+"/sim/stats", &raw); err == nil {
				mu.Lock()
				postgresStats = raw
				mu.Unlock()
			}
		}()
	}

	// Poll common
	var commonStats json.RawMessage
	if s.endpoints.CommonSim != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var raw json.RawMessage
			if err := fetchJSON(ctx, s.endpoints.CommonSim+"/api/v1/events?limit=1", &raw); err == nil {
				mu.Lock()
				commonStats = raw
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Sort hosts by ID for consistent output
	sort.Slice(allHosts, func(i, j int) bool {
		return allHosts[i].HostID < allHosts[j].HostID
	})

	s.mu.Lock()
	s.cache = &AggregatedState{
		Hosts:        allHosts,
		Domains:      allDomains,
		StorageStats: storageStats,
		AWXStats:     awxStats,
		PostgreStats: postgresStats,
		CommonStats:  commonStats,
		UpdatedAt:    time.Now(),
	}
	s.mu.Unlock()
}

// --- Handlers ---

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": Version})
}

func (s *Server) handleOverview(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	c := s.cache
	s.mu.RUnlock()

	overview := Overview{
		TotalHosts:   len(c.Hosts),
		Workers:      len(s.endpoints.Workers),
		StorageStats: c.StorageStats,
		AWXStats:     c.AWXStats,
		UpdatedAt:    c.UpdatedAt,
	}

	for _, h := range c.Hosts {
		if h.State == "" || h.State == "online" {
			overview.OnlineHosts++
		}
		overview.TotalVCPUsUsed += h.UsedVCPUs
		overview.TotalMemUsedMB += h.UsedMemoryMB
		overview.TotalDomains += h.DomainCount
	}

	for _, d := range c.Domains {
		if d.State == 1 { // running
			overview.RunningDomains++
		}
	}

	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleListHosts(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	hosts := s.cache.Hosts
	s.mu.RUnlock()

	if hosts == nil {
		hosts = []HostInfo{}
	}
	writeJSON(w, http.StatusOK, hosts)
}

func (s *Server) handleListHostDomains(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")

	s.mu.RLock()
	var domains []DomainInfo
	for _, d := range s.cache.Domains {
		if d.HostID == hostID {
			domains = append(domains, d)
		}
	}
	s.mu.RUnlock()

	if domains == nil {
		domains = []DomainInfo{}
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *Server) handleListAllDomains(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	domains := s.cache.Domains
	s.mu.RUnlock()

	if domains == nil {
		domains = []DomainInfo{}
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}, "total": 0})
		return
	}
	proxyGet(w, r, s.endpoints.CommonSim+"/api/v1/events")
}

func (s *Server) handleFaults(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	proxyGet(w, r, s.endpoints.CommonSim+"/api/v1/faults")
}

func (s *Server) handleProxyStorageStats(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.StorageSim == "" {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	proxyGet(w, r, s.endpoints.StorageSim+"/sim/stats")
}

func (s *Server) handleProxyAWXStats(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.AWXSim == "" {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	proxyGet(w, r, s.endpoints.AWXSim+"/sim/stats")
}

func (s *Server) handleProxyPostgresStats(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.PostgreSim == "" {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	proxyGet(w, r, s.endpoints.PostgreSim+"/sim/stats")
}

func (s *Server) handleProxyPostgresTables(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.PostgreSim == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	proxyGet(w, r, s.endpoints.PostgreSim+"/sim/tables")
}

func (s *Server) handleProxyPostgresTableData(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.endpoints.PostgreSim == "" {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	proxyGet(w, r, fmt.Sprintf("%s/sim/tables/%s?%s", s.endpoints.PostgreSim, name, r.URL.RawQuery))
}

func (s *Server) handleProxyPostgresTableSchema(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.endpoints.PostgreSim == "" {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	proxyGet(w, r, fmt.Sprintf("%s/sim/tables/%s/schema", s.endpoints.PostgreSim, name))
}

// --- Fault injection proxy ---

func (s *Server) handleProxyFaultAdd(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "common-sim not configured"})
		return
	}
	proxyRequest(w, r, s.endpoints.CommonSim+"/api/v1/faults")
}

func (s *Server) handleProxyFaultClear(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "common-sim not configured"})
		return
	}
	proxyRequest(w, r, s.endpoints.CommonSim+"/api/v1/faults")
}

func (s *Server) handleProxyFaultDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "common-sim not configured"})
		return
	}
	proxyRequest(w, r, fmt.Sprintf("%s/api/v1/faults/%s", s.endpoints.CommonSim, id))
}

// --- Snapshot proxy ---

func (s *Server) handleProxySnapshotSave(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "common-sim not configured"})
		return
	}
	proxyRequest(w, r, s.endpoints.CommonSim+"/api/v1/state/snapshot")
}

func (s *Server) handleProxySnapshotList(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	proxyGet(w, r, s.endpoints.CommonSim+"/api/v1/state/snapshots")
}

func (s *Server) handleProxySnapshotRestore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.endpoints.CommonSim == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "common-sim not configured"})
		return
	}
	proxyRequest(w, r, fmt.Sprintf("%s/api/v1/state/restore/%s", s.endpoints.CommonSim, id))
}

// --- Storage backends proxy ---

func (s *Server) handleProxyStorageBackends(w http.ResponseWriter, r *http.Request) {
	if s.endpoints.StorageSim == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	proxyGet(w, r, s.endpoints.StorageSim+"/sim/backends")
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fetchJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// proxyRequest forwards the original request (method + body) to the target URL.
func proxyRequest(w http.ResponseWriter, r *http.Request, url string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, r.Method, url, r.Body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func proxyGet(w http.ResponseWriter, _ *http.Request, url string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
