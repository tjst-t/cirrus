package compute

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/controller"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/scheduler"
	"github.com/tjst-t/cirrus/internal/storage"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// Orchestrator implements Service and manages the async VM creation/deletion pipeline.
type Orchestrator struct {
	pool       *pgxpool.Pool
	flavorSvc  flavor.Service
	networkSvc network.Service
	storageSvc storage.Service
	scheduler  scheduler.Scheduler
	workers    *controller.WorkerClientPool
	logger     *slog.Logger
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(
	pool *pgxpool.Pool,
	flavorSvc flavor.Service,
	networkSvc network.Service,
	storageSvc storage.Service,
	sched scheduler.Scheduler,
	workers *controller.WorkerClientPool,
	logger *slog.Logger,
) *Orchestrator {
	return &Orchestrator{
		pool:       pool,
		flavorSvc:  flavorSvc,
		networkSvc: networkSvc,
		storageSvc: storageSvc,
		scheduler:  sched,
		workers:    workers,
		logger:     logger,
	}
}

// CreateVM inserts a VM record in "pending" status, then launches a goroutine to build it.
func (o *Orchestrator) CreateVM(ctx context.Context, spec CreateVMSpec) (*VM, error) {
	vm := &VM{
		ID:        uuid.New(),
		TenantID:  spec.TenantID,
		Name:      spec.Name,
		FlavorID:  &spec.FlavorID,
		Status:    VMStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if spec.AZID != uuid.Nil {
		vm.AZID = &spec.AZID
	}
	if spec.NetworkID != uuid.Nil {
		vm.NetworkID = &spec.NetworkID
	}

	if err := o.insertVM(ctx, vm); err != nil {
		return nil, fmt.Errorf("compute: create vm: insert: %w", err)
	}

	// Launch the async build goroutine. Use a detached context so the build
	// continues even after the HTTP request context is cancelled.
	go func() {
		buildCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := o.buildVM(buildCtx, vm.ID, spec); err != nil {
			o.logger.Error("VM build failed", "vm_id", vm.ID, "error", err)
			_ = o.setVMStatus(context.Background(), vm.ID, VMStatusError, err.Error())
		}
	}()

	return vm, nil
}

// GetVM returns a VM by ID for the given tenant.
func (o *Orchestrator) GetVM(ctx context.Context, tenantID, vmID uuid.UUID) (*VM, error) {
	return o.getVM(ctx, tenantID, vmID)
}

// ListVMs returns all VMs for the given tenant.
func (o *Orchestrator) ListVMs(ctx context.Context, tenantID uuid.UUID) ([]VM, error) {
	return o.listVMs(ctx, tenantID)
}

// DeleteVM marks the VM as "deleting" and launches an async cleanup goroutine.
func (o *Orchestrator) DeleteVM(ctx context.Context, tenantID, vmID uuid.UUID) error {
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return err
	}
	if err := o.setVMStatus(ctx, vmID, VMStatusDeleting, ""); err != nil {
		return fmt.Errorf("compute: delete vm: %w", err)
	}

	go func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := o.teardownVM(delCtx, vm); err != nil {
			o.logger.Error("VM teardown failed", "vm_id", vmID, "error", err)
			_ = o.setVMStatus(context.Background(), vmID, VMStatusError, err.Error())
		} else {
			_ = o.deleteVMRecord(context.Background(), vmID)
		}
	}()

	return nil
}

// buildVM orchestrates the full VM creation pipeline.
// Each step is idempotent: if a step was already completed (e.g., volume exists),
// it re-uses the existing resource.
func (o *Orchestrator) buildVM(ctx context.Context, vmID uuid.UUID, spec CreateVMSpec) error {
	if err := o.setVMStatus(ctx, vmID, VMStatusBuilding, ""); err != nil {
		return err
	}

	// 1. Resolve flavor
	flv, err := o.flavorSvc.Get(ctx, spec.FlavorID)
	if err != nil {
		return fmt.Errorf("resolve flavor: %w", err)
	}

	// 2. Schedule: pick host + storage backend
	schedSpec := scheduler.ScheduleSpec{
		AZID:   spec.AZID,
		Flavor: flv,
	}
	if spec.VolumeTypeID != nil {
		schedSpec.VolumeTypeID = spec.VolumeTypeID
	}
	result, err := o.scheduler.Schedule(ctx, schedSpec)
	if err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	hostID := result.HostID

	// Persist host assignment immediately (for observability)
	if err := o.setVMHost(ctx, vmID, hostID); err != nil {
		return fmt.Errorf("persist host: %w", err)
	}

	vmName := fmt.Sprintf("vm-%s", vmID.String()[:8])

	// 3. Create network port (idempotent: check if already exists)
	var port *network.Port
	if spec.NetworkID != uuid.Nil {
		port, err = o.networkSvc.CreatePort(ctx, network.PortSpec{
			TenantID:  spec.TenantID,
			NetworkID: spec.NetworkID,
			HostID:    hostID,
			VMName:    vmName,
		})
		if err != nil {
			return fmt.Errorf("create port: %w", err)
		}
	}

	// 4. Create root volume (idempotent)
	volSpec := storage.CreateVolumeSpec{
		TenantID: spec.TenantID,
		SizeGB:   flv.DiskGB,
	}
	if spec.VolumeTypeID != nil {
		volSpec.VolumeTypeID = spec.VolumeTypeID
	}
	vol, err := o.storageSvc.CreateVolume(ctx, volSpec)
	if err != nil {
		return fmt.Errorf("create volume: %w", err)
	}

	// Persist volume association
	if err := o.insertVMVolume(ctx, vmID, vol.ID, "vda"); err != nil {
		return fmt.Errorf("persist vm_volume: %w", err)
	}

	// 5. Export volume to host
	exportInfo, err := o.storageSvc.ExportVolume(ctx, vol.ID, hostID)
	if err != nil {
		return fmt.Errorf("export volume: %w", err)
	}

	// 6. Get worker gRPC address from host record
	host, err := o.getHostByID(ctx, hostID)
	if err != nil {
		return fmt.Errorf("get host: %w", err)
	}
	if host.WorkerGRPCAddr == "" {
		return fmt.Errorf("host %s has no worker_grpc_addr", hostID)
	}

	// 7. Call worker CreateVM RPC
	workerClient, err := o.workers.Get(host.WorkerGRPCAddr)
	if err != nil {
		return fmt.Errorf("get worker client: %w", err)
	}

	req := &pb.CreateVMRequest{
		VmId:  vmID.String(),
		Name:  vmName,
		Vcpus: int32(flv.VCPUs),
		RamMb: flv.RAMMB,
		Disks: []*pb.DiskSpec{
			{TargetDev: "vda", Protocol: exportInfo.Protocol, Params: exportInfo.Params},
		},
		CloudInit: &pb.CloudInitSpec{
			Hostname: vmName,
			UserData: "#cloud-config\n",
		},
	}

	if port != nil {
		req.Ports = []*pb.PortSpec{
			{
				PortId:     port.ID.String(),
				MacAddress: port.MACAddress,
				BridgeName: "br-int",
			},
		}
	}

	resp, err := workerClient.CreateVM(ctx, req)
	if err != nil {
		return fmt.Errorf("worker CreateVM: %w", err)
	}

	o.logger.Info("VM created on worker", "vm_id", vmID, "status", resp.Status)
	return o.setVMStatus(ctx, vmID, VMStatusRunning, "")
}

// teardownVM stops and deletes a VM on the worker, then cleans up volumes and ports.
func (o *Orchestrator) teardownVM(ctx context.Context, vm *VM) error {
	if vm.HostID == nil {
		return nil // never scheduled; nothing to tear down
	}

	host, err := o.getHostByID(ctx, *vm.HostID)
	if err != nil {
		return fmt.Errorf("get host: %w", err)
	}

	if host.WorkerGRPCAddr != "" {
		workerClient, err := o.workers.Get(host.WorkerGRPCAddr)
		if err != nil {
			return fmt.Errorf("get worker client: %w", err)
		}
		vmName := fmt.Sprintf("vm-%s", vm.ID.String()[:8])
		if _, err := workerClient.DeleteVM(ctx, &pb.DeleteVMRequest{
			VmId: vm.ID.String(),
			Name: vmName,
		}); err != nil {
			o.logger.Warn("worker DeleteVM failed", "vm_id", vm.ID, "error", err)
		}
	}

	return nil
}
