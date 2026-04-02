// Package handler implements the management REST API for libvirt-sim.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/rpc"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
	simxml "github.com/tjst-t/cirrus/test/sim/libvirt/internal/xml"
)

// Management handles the /sim/ REST API endpoints.
type Management struct {
	store  *state.Store
	server *rpc.Server
	logger *slog.Logger
}

// NewManagement creates a new management API handler.
func NewManagement(store *state.Store, server *rpc.Server, logger *slog.Logger) *Management {
	return &Management{
		store:  store,
		server: server,
		logger: logger,
	}
}

// RegisterRoutes registers all management API routes on the given mux.
func (m *Management) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sim/hosts", m.handleCreateHost)
	mux.HandleFunc("GET /sim/hosts", m.handleListHosts)
	mux.HandleFunc("GET /sim/hosts/{host_id}", m.handleGetHost)
	mux.HandleFunc("PUT /sim/hosts/{host_id}/state", m.handleUpdateHostState)
	mux.HandleFunc("POST /sim/hosts/{host_id}/config", m.handleUpdateHostConfig)
	mux.HandleFunc("GET /sim/stats", m.handleGetStats)
	mux.HandleFunc("POST /sim/reset", m.handleReset)
	mux.HandleFunc("POST /sim/config/migration", m.handleUpdateMigrationConfig)
	mux.HandleFunc("GET /sim/config/migration", m.handleGetMigrationConfig)
	mux.HandleFunc("GET /sim/domains", m.handleListAllDomains)
	mux.HandleFunc("GET /sim/hosts/{host_id}/domains", m.handleListHostDomains)

	// Domain lifecycle management (used by LibvirtDriver for VM create/delete/start/stop)
	mux.HandleFunc("POST /sim/hosts/{host_id}/domains", m.handleDefineDomain)
	mux.HandleFunc("DELETE /sim/hosts/{host_id}/domains/{uuid}", m.handleUndefineDomain)
	mux.HandleFunc("POST /sim/hosts/{host_id}/domains/{uuid}/start", m.handleStartDomain)
	mux.HandleFunc("POST /sim/hosts/{host_id}/domains/{uuid}/stop", m.handleStopDomain)
	mux.HandleFunc("POST /sim/hosts/{host_id}/domains/{uuid}/destroy", m.handleDestroyDomain)
}

// CreateHostRequest is the request body for POST /sim/hosts.
type CreateHostRequest struct {
	HostID             string           `json:"host_id"`
	LibvirtPort        int              `json:"libvirt_port"`
	CPUModel           string           `json:"cpu_model"`
	CPUSockets         int              `json:"cpu_sockets"`
	CoresPerSocket     int              `json:"cores_per_socket"`
	ThreadsPerCore     int              `json:"threads_per_core"`
	MemoryMB           int64            `json:"memory_mb"`
	CPUOvercommitRatio float64          `json:"cpu_overcommit_ratio"`
	MemOvercommitRatio float64          `json:"memory_overcommit_ratio"`
	NUMATopology       []state.NUMANode `json:"numa_topology,omitempty"`
	GPUs               []state.GPU      `json:"gpus,omitempty"`
}

func (m *Management) handleCreateHost(w http.ResponseWriter, r *http.Request) {
	var req CreateHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.HostID == "" {
		m.writeError(w, http.StatusBadRequest, "host_id is required")
		return
	}
	if req.LibvirtPort == 0 {
		m.writeError(w, http.StatusBadRequest, "libvirt_port is required")
		return
	}

	host := &state.Host{
		HostID:             req.HostID,
		LibvirtPort:        req.LibvirtPort,
		CPUModel:           req.CPUModel,
		CPUSockets:         req.CPUSockets,
		CoresPerSocket:     req.CoresPerSocket,
		ThreadsPerCore:     req.ThreadsPerCore,
		MemoryMB:           req.MemoryMB,
		CPUOvercommitRatio: req.CPUOvercommitRatio,
		MemOvercommitRatio: req.MemOvercommitRatio,
		NUMATopology:       req.NUMATopology,
		GPUs:               req.GPUs,
	}

	if err := m.store.AddHost(host); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "already") {
			status = http.StatusConflict
		}
		m.writeError(w, status, err.Error())
		return
	}

	// Start TCP listener
	ctx := context.Background()
	if err := m.server.StartListener(ctx, host.HostID, host.LibvirtPort); err != nil {
		// Rollback host registration
		_ = m.store.RemoveHost(host.HostID)
		m.writeError(w, http.StatusInternalServerError, fmt.Sprintf("start listener: %v", err))
		return
	}

	m.logger.Info("host registered", "host_id", host.HostID, "port", host.LibvirtPort)
	m.writeJSON(w, http.StatusCreated, host)
}

func (m *Management) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts := m.store.ListHosts()
	infos := make([]state.HostInfo, 0, len(hosts))
	for _, h := range hosts {
		infos = append(infos, h.Info())
	}
	m.writeJSON(w, http.StatusOK, infos)
}

func (m *Management) handleGetHost(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	host, err := m.store.GetHost(hostID)
	if err != nil {
		m.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	m.writeJSON(w, http.StatusOK, host.Info())
}

// UpdateHostStateRequest is the request body for PUT /sim/hosts/{host_id}/state.
type UpdateHostStateRequest struct {
	State state.HostState `json:"state"`
}

func (m *Management) handleUpdateHostState(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	host, err := m.store.GetHost(hostID)
	if err != nil {
		m.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateHostStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	switch req.State {
	case state.HostStateOnline, state.HostStateOffline, state.HostStateMaintenance:
		host.State = req.State
	default:
		m.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid state: %s", req.State))
		return
	}

	m.logger.Info("host state updated", "host_id", hostID, "state", req.State)
	m.writeJSON(w, http.StatusOK, host.Info())
}

// UpdateHostConfigRequest is the request body for POST /sim/hosts/{host_id}/config.
type UpdateHostConfigRequest struct {
	CPUOvercommitRatio *float64 `json:"cpu_overcommit_ratio,omitempty"`
	MemOvercommitRatio *float64 `json:"memory_overcommit_ratio,omitempty"`
}

func (m *Management) handleUpdateHostConfig(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	host, err := m.store.GetHost(hostID)
	if err != nil {
		m.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateHostConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.CPUOvercommitRatio != nil {
		host.CPUOvercommitRatio = *req.CPUOvercommitRatio
	}
	if req.MemOvercommitRatio != nil {
		host.MemOvercommitRatio = *req.MemOvercommitRatio
	}

	m.logger.Info("host config updated", "host_id", hostID)
	m.writeJSON(w, http.StatusOK, host.Info())
}

func (m *Management) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := m.store.GetStats()
	m.writeJSON(w, http.StatusOK, stats)
}

func (m *Management) handleReset(w http.ResponseWriter, r *http.Request) {
	m.server.StopAll()
	m.store.Reset()
	m.logger.Info("all state reset")
	m.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (m *Management) handleGetMigrationConfig(w http.ResponseWriter, r *http.Request) {
	cfg := m.store.GetMigrationConfig()
	m.writeJSON(w, http.StatusOK, cfg)
}

func (m *Management) handleUpdateMigrationConfig(w http.ResponseWriter, r *http.Request) {
	var cfg state.MigrationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		m.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	m.store.SetMigrationConfig(cfg)
	m.logger.Info("migration config updated",
		"prepare_ms", cfg.PrepareDurationMs,
		"base_transfer_ms", cfg.BaseTransferDurationMs,
		"per_gb_ms", cfg.PerGBMemoryMs,
		"finish_ms", cfg.FinishDurationMs)
	m.writeJSON(w, http.StatusOK, cfg)
}

// DomainInfo is the JSON representation of a domain for the management API.
type DomainInfo struct {
	Name         string `json:"name"`
	UUID         string `json:"uuid"`
	State        int32  `json:"state"`
	VCPUs        int    `json:"vcpus"`
	MemoryKiB    int64  `json:"memory_kib"`
	HostID       string `json:"host_id"`
	InterfaceIDs []string `json:"interface_ids,omitempty"`
}

func (m *Management) handleListAllDomains(w http.ResponseWriter, r *http.Request) {
	hosts := m.store.ListHosts()
	var domains []DomainInfo
	for _, h := range hosts {
		doms, err := m.store.ListDomains(h.HostID)
		if err != nil {
			continue
		}
		for _, d := range doms {
			domains = append(domains, DomainInfo{
				Name:         d.Name,
				UUID:         d.UUIDString(),
				State:        int32(d.State),
				VCPUs:        d.VCPUs,
				MemoryKiB:    d.MemoryKiB,
				HostID:       h.HostID,
				InterfaceIDs: d.InterfaceIDs,
			})
		}
	}
	if domains == nil {
		domains = []DomainInfo{}
	}
	m.writeJSON(w, http.StatusOK, domains)
}

func (m *Management) handleListHostDomains(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	doms, err := m.store.ListDomains(hostID)
	if err != nil {
		m.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	domains := make([]DomainInfo, 0, len(doms))
	for _, d := range doms {
		domains = append(domains, DomainInfo{
			Name:         d.Name,
			UUID:         d.UUIDString(),
			State:        int32(d.State),
			VCPUs:        d.VCPUs,
			MemoryKiB:    d.MemoryKiB,
			HostID:       hostID,
			InterfaceIDs: d.InterfaceIDs,
		})
	}
	m.writeJSON(w, http.StatusOK, domains)
}

// DefineDomainRequest is the request body for POST /sim/hosts/{host_id}/domains.
type DefineDomainRequest struct {
	XML string `json:"xml"` // libvirt domain XML
}

func (m *Management) handleDefineDomain(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	var req DefineDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.XML == "" {
		m.writeError(w, http.StatusBadRequest, "xml is required")
		return
	}

	parsed, err := simxml.ParseDomainXML(req.XML)
	if err != nil {
		m.writeError(w, http.StatusBadRequest, "invalid domain xml: "+err.Error())
		return
	}

	d := state.NewDomainFromXML(parsed, req.XML)
	if err := m.store.DefineDomain(hostID, d); err != nil {
		m.writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := m.store.StartDomain(hostID, d.UUIDString()); err != nil {
		m.writeError(w, http.StatusInternalServerError, "start domain: "+err.Error())
		return
	}
	dom, _ := m.store.GetDomain(hostID, d.UUIDString())
	m.writeJSON(w, http.StatusCreated, DomainInfo{
		Name:         dom.Name,
		UUID:         dom.UUIDString(),
		State:        int32(dom.State),
		VCPUs:        dom.VCPUs,
		MemoryKiB:    dom.MemoryKiB,
		HostID:       hostID,
		InterfaceIDs: dom.InterfaceIDs,
	})
}

func (m *Management) handleUndefineDomain(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	domUUID := r.PathValue("uuid")
	// Destroy first if running, then undefine.
	_ = m.store.DestroyDomain(hostID, domUUID)
	if err := m.store.UndefineDomain(hostID, domUUID); err != nil {
		m.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Management) handleStartDomain(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	domUUID := r.PathValue("uuid")
	if err := m.store.StartDomain(hostID, domUUID); err != nil {
		m.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Management) handleStopDomain(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	domUUID := r.PathValue("uuid")
	if err := m.store.ShutdownDomain(hostID, domUUID); err != nil {
		m.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Management) handleDestroyDomain(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	domUUID := r.PathValue("uuid")
	if err := m.store.DestroyDomain(hostID, domUUID); err != nil {
		m.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Management) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		m.logger.Error("failed to write JSON response", "error", err)
	}
}

func (m *Management) writeError(w http.ResponseWriter, status int, msg string) {
	m.writeJSON(w, status, map[string]string{"error": msg})
}
