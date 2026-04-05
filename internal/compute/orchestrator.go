package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/controller"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/quota"
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
	quotaSvc   quota.Service
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
	quotaSvc quota.Service,
	logger *slog.Logger,
) *Orchestrator {
	return &Orchestrator{
		pool:       pool,
		flavorSvc:  flavorSvc,
		networkSvc: networkSvc,
		storageSvc: storageSvc,
		scheduler:  sched,
		workers:    workers,
		quotaSvc:   quotaSvc,
		logger:     logger,
	}
}

// CreateVM inserts a VM record in "pending" status, then launches a goroutine to build it.
func (o *Orchestrator) CreateVM(ctx context.Context, spec CreateVMSpec) (*VM, error) {
	// Resolve flavor up-front to compute the quota delta.
	flv, err := o.flavorSvc.Get(ctx, spec.FlavorID)
	if err != nil {
		return nil, fmt.Errorf("compute: create vm: resolve flavor: %w", err)
	}

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

	// Reserve quota before persisting the VM record.
	if o.quotaSvc != nil {
		vmDelta := quota.ResourceDelta{Vcpus: flv.VCPUs, RAMMB: int(flv.RAMMB), VMs: 1}
		if err := o.quotaSvc.Reserve(ctx, spec.TenantID, quota.ResourceTypeVM, vm.ID, vmDelta); err != nil {
			return nil, fmt.Errorf("compute: create vm: quota reserve: %w", err)
		}
	}

	if err := o.insertVM(ctx, vm); err != nil {
		if o.quotaSvc != nil {
			_ = o.quotaSvc.Release(context.Background(), quota.ResourceTypeVM, vm.ID)
		}
		return nil, fmt.Errorf("compute: create vm: insert: %w", err)
	}

	// Launch the async build goroutine. Use a detached context so the build
	// continues even after the HTTP request context is cancelled.
	go func() {
		buildCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := o.buildVM(buildCtx, vm.ID, spec, flv); err != nil {
			o.logger.Error("VM build failed", "vm_id", vm.ID, "error", err)
			if o.quotaSvc != nil {
				_ = o.quotaSvc.Release(context.Background(), quota.ResourceTypeVM, vm.ID)
			}
			_ = o.setVMStatus(context.Background(), vm.ID, VMStatusError, err.Error())
		} else if o.quotaSvc != nil {
			if err := o.quotaSvc.Commit(context.Background(), quota.ResourceTypeVM, vm.ID); err != nil {
				o.logger.Warn("quota commit failed after VM build", "vm_id", vm.ID, "error", err)
			}
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
// Only allowed when status is stopped or error.
func (o *Orchestrator) DeleteVM(ctx context.Context, tenantID, vmID uuid.UUID) error {
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return err
	}
	if vm.IsTransitional() {
		return ErrConflict
	}
	if !vm.CanDelete() {
		return ErrConflict
	}
	if err := o.setVMStatus(ctx, vmID, VMStatusDeleting, ""); err != nil {
		return fmt.Errorf("compute: delete vm: %w", err)
	}

	// Capture flavor for quota decommit after teardown.
	var vmFlavor *flavor.Flavor
	if vm.FlavorID != nil {
		if flv, err := o.flavorSvc.Get(ctx, *vm.FlavorID); err == nil {
			vmFlavor = flv
		}
	}

	go func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := o.teardownVM(delCtx, vm); err != nil {
			o.logger.Error("VM teardown failed", "vm_id", vmID, "error", err)
			_ = o.setVMStatus(context.Background(), vmID, VMStatusError, err.Error())
		} else {
			_ = o.deleteVMRecord(context.Background(), vmID)
			if o.quotaSvc != nil && vmFlavor != nil {
				if err := o.quotaSvc.Decommit(context.Background(), vm.TenantID, quota.ResourceDelta{
					Vcpus: vmFlavor.VCPUs,
					RAMMB: int(vmFlavor.RAMMB),
					VMs:   1,
				}); err != nil {
					o.logger.Warn("quota decommit failed after VM teardown", "vm_id", vmID, "error", err)
				}
			}
		}
	}()

	return nil
}

// StartVM starts a stopped VM.
func (o *Orchestrator) StartVM(ctx context.Context, tenantID, vmID uuid.UUID) error {
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return err
	}
	if vm.IsTransitional() {
		return ErrConflict
	}
	if !vm.CanStart() {
		return ErrConflict
	}

	workerClient, vmName, err := o.resolveWorker(ctx, vm)
	if err != nil {
		return err
	}
	if _, err := workerClient.StartVM(ctx, &pb.StartVMRequest{VmId: vmID.String(), Name: vmName}); err != nil {
		return fmt.Errorf("compute: start vm: %w", err)
	}
	return o.setVMStatus(ctx, vmID, VMStatusRunning, "")
}

// StopVM gracefully shuts down a running VM.
func (o *Orchestrator) StopVM(ctx context.Context, tenantID, vmID uuid.UUID) error {
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return err
	}
	if vm.IsTransitional() {
		return ErrConflict
	}
	if !vm.CanStop() {
		return ErrConflict
	}

	workerClient, vmName, err := o.resolveWorker(ctx, vm)
	if err != nil {
		return err
	}
	if _, err := workerClient.StopVM(ctx, &pb.StopVMRequest{VmId: vmID.String(), Name: vmName}); err != nil {
		return fmt.Errorf("compute: stop vm: %w", err)
	}
	return o.setVMStatus(ctx, vmID, VMStatusStopped, "")
}

// ForceStopVM forcefully powers off a running VM.
func (o *Orchestrator) ForceStopVM(ctx context.Context, tenantID, vmID uuid.UUID) error {
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return err
	}
	if vm.IsTransitional() {
		return ErrConflict
	}
	if !vm.CanStop() {
		return ErrConflict
	}

	workerClient, vmName, err := o.resolveWorker(ctx, vm)
	if err != nil {
		return err
	}
	if _, err := workerClient.ForceStopVM(ctx, &pb.ForceStopVMRequest{VmId: vmID.String(), Name: vmName}); err != nil {
		return fmt.Errorf("compute: force-stop vm: %w", err)
	}
	return o.setVMStatus(ctx, vmID, VMStatusStopped, "")
}

// RebootVM reboots a running VM.
func (o *Orchestrator) RebootVM(ctx context.Context, tenantID, vmID uuid.UUID) error {
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return err
	}
	if vm.IsTransitional() {
		return ErrConflict
	}
	if !vm.CanReboot() {
		return ErrConflict
	}

	workerClient, vmName, err := o.resolveWorker(ctx, vm)
	if err != nil {
		return err
	}
	if _, err := workerClient.RebootVM(ctx, &pb.RebootVMRequest{VmId: vmID.String(), Name: vmName}); err != nil {
		return fmt.Errorf("compute: reboot vm: %w", err)
	}
	return o.setVMStatus(ctx, vmID, VMStatusRunning, "")
}

// RepairVM transitions a VM from error to stopped (admin use only).
func (o *Orchestrator) RepairVM(ctx context.Context, vmID uuid.UUID) error {
	vm, err := o.getVMByID(ctx, vmID)
	if err != nil {
		return err
	}
	if vm.Status != VMStatusError {
		return fmt.Errorf("compute: repair vm: vm is not in error state (current: %s)", vm.Status)
	}
	return o.setVMStatus(ctx, vmID, VMStatusStopped, "")
}

// resolveWorker returns the worker client and VM name for a VM.
func (o *Orchestrator) resolveWorker(ctx context.Context, vm *VM) (*controller.WorkerClient, string, error) {
	if vm.HostID == nil {
		return nil, "", fmt.Errorf("compute: vm has no assigned host")
	}
	h, err := o.getHostByID(ctx, *vm.HostID)
	if err != nil {
		return nil, "", fmt.Errorf("compute: get host: %w", err)
	}
	if h.WorkerGRPCAddr == "" {
		return nil, "", fmt.Errorf("compute: host has no worker_grpc_addr")
	}
	workerClient, err := o.workers.Get(h.WorkerGRPCAddr)
	if err != nil {
		return nil, "", fmt.Errorf("compute: get worker client: %w", err)
	}
	return workerClient, fmt.Sprintf("vm-%s", vm.ID.String()[:8]), nil
}

// buildVM orchestrates the full VM creation pipeline.
// Each step is idempotent: if a step was already completed (e.g., volume exists),
// it re-uses the existing resource.
func (o *Orchestrator) buildVM(ctx context.Context, vmID uuid.UUID, spec CreateVMSpec, flv *flavor.Flavor) error {
	if err := o.setVMStatus(ctx, vmID, VMStatusBuilding, ""); err != nil {
		return err
	}

	// 1. Schedule: pick host + storage backend
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
			VMID:      &vmID,
			VMName:    vmName,
		})
		if err != nil {
			return fmt.Errorf("create port: %w", err)
		}
	}

	// 4. Create root volume (idempotent)
	volSpec := storage.CreateVolumeSpec{
		TenantID: spec.TenantID,
		Name:     vmName,
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
// Order: worker.DeleteVM (DestroyVM→BlockDev.Detach→UndefineVM) →
//        Network.DeletePort → Storage.UnexportVolume → Storage.DeleteVolume → DB delete
func (o *Orchestrator) teardownVM(ctx context.Context, vm *VM) error {
	vmName := fmt.Sprintf("vm-%s", vm.ID.String()[:8])

	volumeIDs, err := o.listVMVolumeIDs(ctx, vm.ID)
	if err != nil {
		o.logger.Warn("teardownVM: list vm volumes failed", "vm_id", vm.ID, "error", err)
	}

	if vm.HostID != nil {
		h, err := o.getHostByID(ctx, *vm.HostID)
		if err != nil {
			o.logger.Warn("teardownVM: get host failed", "vm_id", vm.ID, "error", err)
		} else if h.WorkerGRPCAddr != "" {
			workerClient, err := o.workers.Get(h.WorkerGRPCAddr)
			if err != nil {
				o.logger.Warn("teardownVM: get worker client failed", "vm_id", vm.ID, "error", err)
			} else {
				req := &pb.DeleteVMRequest{VmId: vm.ID.String(), Name: vmName}
				for _, vid := range volumeIDs {
					vol, err := o.storageSvc.GetVolume(ctx, vm.TenantID, vid)
					if err != nil {
						o.logger.Warn("teardownVM: get volume failed", "volume_id", vid, "error", err)
						continue
					}
					if vol.ExportInfo != nil {
						var info storage.ExportInfo
						if err := json.Unmarshal(vol.ExportInfo, &info); err == nil {
							req.Disks = append(req.Disks, &pb.DiskSpec{
								Protocol: info.Protocol,
								Params:   info.Params,
							})
						}
					}
				}
				if _, err := workerClient.DeleteVM(ctx, req); err != nil {
					o.logger.Warn("worker DeleteVM failed", "vm_id", vm.ID, "error", err)
				}
			}
		}
	}

	port, err := o.networkSvc.GetPortByVMID(ctx, vm.ID)
	if err == nil {
		if err := o.networkSvc.DeletePort(ctx, port.ID); err != nil {
			o.logger.Warn("teardownVM: delete port failed", "vm_id", vm.ID, "error", err)
		}
	}

	for _, vid := range volumeIDs {
		if err := o.storageSvc.UnexportVolume(ctx, vid); err != nil {
			o.logger.Warn("teardownVM: unexport volume failed", "volume_id", vid, "error", err)
		}
		if err := o.storageSvc.DeleteVolume(ctx, vm.TenantID, vid); err != nil {
			o.logger.Warn("teardownVM: delete volume failed", "volume_id", vid, "error", err)
		}
	}

	return nil
}
