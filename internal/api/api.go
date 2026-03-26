package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/tjst-t/cirrus/internal/scheduler"
	"github.com/tjst-t/cirrus/internal/state"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

//go:embed static
var staticFiles embed.FS

type Handler struct {
	db        *state.DB
	scheduler *scheduler.Scheduler
	logger    *slog.Logger
	jobs      chan vmJob
}

type vmJob struct {
	VM    *state.VM
	Ports []state.Port
	Net   *state.Network
	Image *state.Image
}

func New(db *state.DB, sched *scheduler.Scheduler, logger *slog.Logger) *Handler {
	h := &Handler{
		db:        db,
		scheduler: sched,
		logger:    logger,
		jobs:      make(chan vmJob, 100),
	}
	// Start VM creation workers
	for i := 0; i < 5; i++ {
		go h.processJobs()
	}
	return h
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Serve WebUI
	staticFS, _ := fs.Sub(staticFiles, "static")
	r.Handle("/*", http.FileServer(http.FS(staticFS)))

	r.Route("/api/v1", func(r chi.Router) {
		// Admin endpoints (no auth for now)
		r.Post("/projects", h.createProject)
		r.Get("/projects", h.listProjects)
		r.Post("/projects/{projectID}/api-keys", h.createAPIKey)

		// Worker endpoints
		r.Get("/workers", h.listWorkers)
		r.Get("/workers/{workerID}", h.getWorker)
		r.Post("/workers/heartbeat", h.workerHeartbeat)
		r.Post("/workers/register", h.workerRegister)

		// Tenant endpoints (require API key auth)
		r.Group(func(r chi.Router) {
			r.Use(h.authMiddleware)
			r.Get("/images", h.listImages)
			r.Post("/images", h.createImage)

			r.Post("/networks", h.createNetwork)
			r.Get("/networks", h.listNetworks)
			r.Get("/networks/{networkID}", h.getNetwork)
			r.Delete("/networks/{networkID}", h.deleteNetwork)

			r.Post("/vms", h.createVM)
			r.Get("/vms", h.listVMs)
			r.Get("/vms/{vmID}", h.getVM)
			r.Delete("/vms/{vmID}", h.deleteVM)
			r.Post("/vms/{vmID}/actions", h.vmAction)

			r.Get("/ports", h.listPorts)
			r.Get("/ports/{portID}", h.getPort)
		})
	})

	return r
}

// Middleware

type contextKey string

const projectIDKey contextKey = "project_id"

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, "missing X-API-Key header")
			return
		}
		projectID, err := h.db.AuthenticateKey(r.Context(), apiKey)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid API key")
			return
		}
		ctx := context.WithValue(r.Context(), projectIDKey, projectID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getProjectID(r *http.Request) string {
	return r.Context().Value(projectIDKey).(string)
}

// Projects

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		QuotaVCPUs int    `json:"quota_vcpus"`
		QuotaRamMB int    `json:"quota_ram_mb"`
		QuotaVMs   int    `json:"quota_vms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	p := &state.Project{
		Name:       req.Name,
		QuotaVCPUs: req.QuotaVCPUs,
		QuotaRamMB: req.QuotaRamMB,
		QuotaVMs:   req.QuotaVMs,
	}
	if p.QuotaVCPUs == 0 {
		p.QuotaVCPUs = 20
	}
	if p.QuotaRamMB == 0 {
		p.QuotaRamMB = 51200
	}
	if p.QuotaVMs == 0 {
		p.QuotaVMs = 10
	}

	if err := h.db.CreateProject(r.Context(), p); err != nil {
		h.logger.Error("create project failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.db.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		req.Name = "default"
	}

	ak, err := h.db.CreateAPIKey(r.Context(), projectID, req.Name)
	if err != nil {
		h.logger.Error("create api key failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}
	writeJSON(w, http.StatusCreated, ak)
}

// Workers

func (h *Handler) listWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := h.db.ListWorkers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workers")
		return
	}
	writeJSON(w, http.StatusOK, workers)
}

func (h *Handler) getWorker(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "workerID")
	worker, err := h.db.GetWorker(r.Context(), workerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

func (h *Handler) workerRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Address   string `json:"address"`
		VCPUs     int    `json:"vcpus"`
		RamMB     int    `json:"ram_mb"`
		DiskGB    int    `json:"disk_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	worker := &state.Worker{
		Name:       req.Name,
		Address:    req.Address,
		TotalVCPUs: req.VCPUs,
		TotalRamMB: req.RamMB,
		TotalDiskGB: req.DiskGB,
		Status:     "online",
	}
	if err := h.db.UpsertWorker(r.Context(), worker); err != nil {
		h.logger.Error("register worker failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to register worker")
		return
	}

	// Set up tunnels to other workers
	h.setupTunnels(r.Context(), worker)

	writeJSON(w, http.StatusOK, worker)
}

func (h *Handler) workerHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.db.UpdateWorkerHeartbeat(r.Context(), req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) setupTunnels(ctx context.Context, newWorker *state.Worker) {
	workers, err := h.db.ListWorkers(ctx)
	if err != nil {
		h.logger.Error("list workers for tunnel setup", "error", err)
		return
	}

	for _, w := range workers {
		if w.ID == newWorker.ID {
			continue
		}
		// Tell existing worker about new worker
		go h.configureTunnel(ctx, w.Address, newWorker.Address, newWorker.Name)
		// Tell new worker about existing worker
		go h.configureTunnel(ctx, newWorker.Address, w.Address, w.Name)
	}
}

func (h *Handler) configureTunnel(ctx context.Context, workerAddr, peerAddr, peerName string) {
	conn, err := grpc.NewClient(workerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		h.logger.Error("connect to worker for tunnel", "worker", workerAddr, "error", err)
		return
	}
	defer conn.Close()

	client := pb.NewWorkerAgentClient(conn)
	resp, err := client.ConfigureTunnel(ctx, &pb.ConfigureTunnelRequest{
		PeerAddress: peerAddr,
		PeerName:    peerName,
	})
	if err != nil {
		h.logger.Error("configure tunnel failed", "worker", workerAddr, "peer", peerAddr, "error", err)
		return
	}
	if !resp.Success {
		h.logger.Error("configure tunnel returned error", "worker", workerAddr, "error", resp.Error)
	}
}

// Images

func (h *Handler) listImages(w http.ResponseWriter, r *http.Request) {
	images, err := h.db.ListImages(r.Context(), getProjectID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list images")
		return
	}
	writeJSON(w, http.StatusOK, images)
}

func (h *Handler) createImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Format == "" {
		req.Format = "qcow2"
	}

	projectID := getProjectID(r)
	img := &state.Image{
		Name:      req.Name,
		ProjectID: &projectID,
		Format:    req.Format,
		Status:    "active",
		Path:      "", // will be set after upload
	}
	if err := h.db.CreateImage(r.Context(), img); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create image")
		return
	}
	writeJSON(w, http.StatusCreated, img)
}

// Networks

func (h *Handler) createNetwork(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		CIDR string `json:"cidr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.CIDR == "" {
		writeError(w, http.StatusBadRequest, "name and cidr are required")
		return
	}

	// Validate CIDR
	_, _, err := net.ParseCIDR(req.CIDR)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid CIDR")
		return
	}

	// Calculate gateway (first usable IP)
	gateway, err := calcGateway(req.CIDR)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot determine gateway")
		return
	}

	// Allocate VNI
	vni, err := h.db.AllocateVNI(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to allocate VNI")
		return
	}

	n := &state.Network{
		ProjectID: getProjectID(r),
		Name:      req.Name,
		CIDR:      req.CIDR,
		Gateway:   gateway,
		VNI:       vni,
		Status:    "active",
	}
	if err := h.db.CreateNetwork(r.Context(), n); err != nil {
		h.logger.Error("create network failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create network")
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (h *Handler) listNetworks(w http.ResponseWriter, r *http.Request) {
	nets, err := h.db.ListNetworks(r.Context(), getProjectID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list networks")
		return
	}
	writeJSON(w, http.StatusOK, nets)
}

func (h *Handler) getNetwork(w http.ResponseWriter, r *http.Request) {
	n, err := h.db.GetNetwork(r.Context(), chi.URLParam(r, "networkID"), getProjectID(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "network not found")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (h *Handler) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	if err := h.db.DeleteNetwork(r.Context(), chi.URLParam(r, "networkID"), getProjectID(r)); err != nil {
		writeError(w, http.StatusNotFound, "network not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// VMs

func (h *Handler) createVM(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ImageID  string `json:"image_id"`
		VCPUs    int    `json:"vcpus"`
		RamMB    int    `json:"ram_mb"`
		DiskGB   int    `json:"disk_gb"`
		Networks []struct {
			NetworkID string `json:"network_id"`
		} `json:"networks"`
		SSHPublicKey string `json:"ssh_public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.ImageID == "" || req.VCPUs == 0 || req.RamMB == 0 || req.DiskGB == 0 {
		writeError(w, http.StatusBadRequest, "name, image_id, vcpus, ram_mb, disk_gb are required")
		return
	}

	projectID := getProjectID(r)
	ctx := r.Context()

	// Verify image exists
	img, err := h.db.GetImage(ctx, req.ImageID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "image not found")
		return
	}

	// Quota check
	project, err := h.db.GetProject(ctx, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get project")
		return
	}
	usage, err := h.db.GetQuotaUsage(ctx, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check quota")
		return
	}
	if usage.UsedVCPUs+req.VCPUs > project.QuotaVCPUs {
		writeError(w, http.StatusForbidden, "vCPU quota exceeded")
		return
	}
	if usage.UsedRamMB+req.RamMB > project.QuotaRamMB {
		writeError(w, http.StatusForbidden, "RAM quota exceeded")
		return
	}
	if usage.UsedVMs+1 > project.QuotaVMs {
		writeError(w, http.StatusForbidden, "VM count quota exceeded")
		return
	}

	// Create VM record
	vm := &state.VM{
		ProjectID: projectID,
		Name:      req.Name,
		ImageID:   req.ImageID,
		VCPUs:     req.VCPUs,
		RamMB:     req.RamMB,
		DiskGB:    req.DiskGB,
		Status:    "scheduling",
	}
	if err := h.db.CreateVM(ctx, vm); err != nil {
		h.logger.Error("create vm failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create VM")
		return
	}

	// Create ports for each requested network
	var ports []state.Port
	var networkForJob *state.Network
	for _, nReq := range req.Networks {
		network, err := h.db.GetNetwork(ctx, nReq.NetworkID, projectID)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("network %s not found", nReq.NetworkID))
			return
		}
		networkForJob = network

		// Allocate IP
		usedIPs, err := h.db.GetUsedIPs(ctx, network.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get used IPs")
			return
		}
		ip, err := state.NextAvailableIP(network.CIDR, network.Gateway, usedIPs)
		if err != nil {
			writeError(w, http.StatusConflict, "no available IPs in network")
			return
		}

		// Generate MAC
		mac, err := state.GenerateMAC()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate MAC")
			return
		}

		port := state.Port{
			ProjectID:  projectID,
			NetworkID:  network.ID,
			VMID:       &vm.ID,
			MACAddress: mac,
			IPAddress:  ip,
			Status:     "down",
		}
		if err := h.db.CreatePort(ctx, &port); err != nil {
			h.logger.Error("create port failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create port")
			return
		}
		ports = append(ports, port)
	}
	vm.Ports = ports

	// Submit async job
	h.jobs <- vmJob{VM: vm, Ports: ports, Net: networkForJob, Image: img}

	writeJSON(w, http.StatusAccepted, vm)
}

func (h *Handler) listVMs(w http.ResponseWriter, r *http.Request) {
	vms, err := h.db.ListVMs(r.Context(), getProjectID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list VMs")
		return
	}
	// Attach ports to each VM
	for i := range vms {
		ports, _ := h.db.ListPortsByVM(r.Context(), vms[i].ID)
		vms[i].Ports = ports
	}
	writeJSON(w, http.StatusOK, vms)
}

func (h *Handler) getVM(w http.ResponseWriter, r *http.Request) {
	vm, err := h.db.GetVM(r.Context(), chi.URLParam(r, "vmID"), getProjectID(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "VM not found")
		return
	}
	ports, _ := h.db.ListPortsByVM(r.Context(), vm.ID)
	vm.Ports = ports
	writeJSON(w, http.StatusOK, vm)
}

func (h *Handler) deleteVM(w http.ResponseWriter, r *http.Request) {
	vmID := chi.URLParam(r, "vmID")
	projectID := getProjectID(r)
	ctx := r.Context()

	vm, err := h.db.GetVM(ctx, vmID, projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "VM not found")
		return
	}

	if err := h.db.UpdateVMStatus(ctx, vm.ID, "deleting", nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update VM status")
		return
	}

	// Delete on worker if assigned
	if vm.WorkerID != nil {
		go h.deleteVMOnWorker(vm)
	} else {
		h.db.UpdateVMStatus(context.Background(), vm.ID, "deleted", nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) vmAction(w http.ResponseWriter, r *http.Request) {
	vmID := chi.URLParam(r, "vmID")
	projectID := getProjectID(r)
	ctx := r.Context()

	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	vm, err := h.db.GetVM(ctx, vmID, projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "VM not found")
		return
	}
	if vm.WorkerID == nil {
		writeError(w, http.StatusConflict, "VM not assigned to a worker")
		return
	}

	worker, err := h.db.GetWorker(ctx, *vm.WorkerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "worker not found")
		return
	}

	conn, err := grpc.NewClient(worker.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect to worker")
		return
	}
	defer conn.Close()
	client := pb.NewWorkerAgentClient(conn)

	switch req.Action {
	case "start":
		resp, err := client.StartVM(ctx, &pb.StartVMRequest{VmId: vm.ID})
		if err != nil || !resp.Success {
			errMsg := "start failed"
			if err != nil {
				errMsg = err.Error()
			} else if resp.Error != "" {
				errMsg = resp.Error
			}
			writeError(w, http.StatusInternalServerError, errMsg)
			return
		}
		h.db.UpdateVMStatus(ctx, vm.ID, "active", nil)
	case "stop":
		resp, err := client.StopVM(ctx, &pb.StopVMRequest{VmId: vm.ID})
		if err != nil || !resp.Success {
			errMsg := "stop failed"
			if err != nil {
				errMsg = err.Error()
			} else if resp.Error != "" {
				errMsg = resp.Error
			}
			writeError(w, http.StatusInternalServerError, errMsg)
			return
		}
		h.db.UpdateVMStatus(ctx, vm.ID, "shutoff", nil)
	case "reboot":
		_, _ = client.StopVM(ctx, &pb.StopVMRequest{VmId: vm.ID})
		resp, err := client.StartVM(ctx, &pb.StartVMRequest{VmId: vm.ID})
		if err != nil || !resp.Success {
			writeError(w, http.StatusInternalServerError, "reboot failed")
			return
		}
		h.db.UpdateVMStatus(ctx, vm.ID, "active", nil)
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+req.Action)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ports

func (h *Handler) listPorts(w http.ResponseWriter, r *http.Request) {
	ports, err := h.db.ListPortsByProject(r.Context(), getProjectID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list ports")
		return
	}
	writeJSON(w, http.StatusOK, ports)
}

func (h *Handler) getPort(w http.ResponseWriter, r *http.Request) {
	port, err := h.db.GetPort(r.Context(), chi.URLParam(r, "portID"), getProjectID(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "port not found")
		return
	}
	writeJSON(w, http.StatusOK, port)
}

// Async VM creation

func (h *Handler) processJobs() {
	for job := range h.jobs {
		h.buildVM(job)
	}
}

func (h *Handler) buildVM(job vmJob) {
	ctx := context.Background()
	vm := job.VM

	// Schedule: pick a worker
	worker, err := h.scheduler.Schedule(ctx, vm.VCPUs, vm.RamMB, vm.DiskGB)
	if err != nil {
		errMsg := err.Error()
		h.logger.Error("scheduling failed", "vm_id", vm.ID, "error", err)
		h.db.UpdateVMStatus(ctx, vm.ID, "error", &errMsg)
		return
	}

	if err := h.db.UpdateVMWorker(ctx, vm.ID, worker.ID); err != nil {
		errMsg := err.Error()
		h.db.UpdateVMStatus(ctx, vm.ID, "error", &errMsg)
		return
	}

	// Connect to worker
	conn, err := grpc.NewClient(worker.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		errMsg := fmt.Sprintf("connect to worker: %v", err)
		h.db.UpdateVMStatus(ctx, vm.ID, "error", &errMsg)
		return
	}
	defer conn.Close()
	client := pb.NewWorkerAgentClient(conn)

	// Build port specs
	var portSpecs []*pb.PortSpec
	for _, p := range job.Ports {
		ps := &pb.PortSpec{
			Id:         p.ID,
			MacAddress: p.MACAddress,
			IpAddress:  p.IPAddress,
			Vni:        int32(job.Net.VNI),
			Cidr:       job.Net.CIDR,
			Gateway:    job.Net.Gateway,
		}
		portSpecs = append(portSpecs, ps)
	}

	// Call CreateVM on worker
	resp, err := client.CreateVM(ctx, &pb.CreateVMRequest{
		VmId:  vm.ID,
		Name:  vm.Name,
		Vcpus: int32(vm.VCPUs),
		RamMb: int32(vm.RamMB),
		Disk: &pb.DiskSpec{
			BaseImagePath: job.Image.Path,
			SizeGb:        int32(vm.DiskGB),
		},
		Ports: portSpecs,
	})
	if err != nil {
		errMsg := fmt.Sprintf("worker CreateVM: %v", err)
		h.logger.Error("worker CreateVM failed", "vm_id", vm.ID, "error", err)
		h.db.UpdateVMStatus(ctx, vm.ID, "error", &errMsg)
		return
	}
	if !resp.Success {
		h.logger.Error("worker CreateVM returned error", "vm_id", vm.ID, "error", resp.Error)
		h.db.UpdateVMStatus(ctx, vm.ID, "error", &resp.Error)
		return
	}

	// Success
	h.db.UpdateVMStatus(ctx, vm.ID, "active", nil)
	for _, p := range job.Ports {
		h.db.UpdatePortStatus(ctx, p.ID, "active")
	}
	h.logger.Info("VM created successfully", "vm_id", vm.ID, "worker", worker.Name)
}

func (h *Handler) deleteVMOnWorker(vm *state.VM) {
	ctx := context.Background()

	worker, err := h.db.GetWorker(ctx, *vm.WorkerID)
	if err != nil {
		h.logger.Error("get worker for VM delete", "error", err)
		return
	}

	conn, err := grpc.NewClient(worker.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		h.logger.Error("connect to worker for VM delete", "error", err)
		return
	}
	defer conn.Close()
	client := pb.NewWorkerAgentClient(conn)

	// Delete ports on worker
	ports, _ := h.db.ListPortsByVM(ctx, vm.ID)
	for _, p := range ports {
		client.RemovePort(ctx, &pb.RemovePortRequest{PortId: p.ID})
		h.db.UpdatePortStatus(ctx, p.ID, "down")
	}

	// Delete VM on worker
	resp, err := client.DeleteVM(ctx, &pb.DeleteVMRequest{VmId: vm.ID})
	if err != nil || !resp.Success {
		h.logger.Error("worker DeleteVM failed", "vm_id", vm.ID)
	}

	h.db.UpdateVMStatus(ctx, vm.ID, "deleted", nil)
}

// Helpers

func calcGateway(cidr string) (string, error) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	ip = ip.To4()
	if ip == nil {
		return "", fmt.Errorf("IPv6 not supported")
	}
	ip[3]++
	return ip.String(), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// extractHost extracts the host part from an address (strips port if present).
func extractHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return strings.TrimSpace(addr)
	}
	return host
}
