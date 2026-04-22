package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/controller"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/jobqueue"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/quota"
	"github.com/tjst-t/cirrus/internal/scheduler"
	"github.com/tjst-t/cirrus/internal/storage"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// Job type constants for the compute domain.
const (
	JobTypeVMCreate = "vm_create"
	JobTypeVMDelete = "vm_delete"
)

// VMCreatePayload is the JSON payload stored in the job for vm_create jobs.
type VMCreatePayload struct {
	Spec CreateVMSpec `json:"spec"`
	VMID uuid.UUID    `json:"vm_id"`
}

// VMDeletePayload is the JSON payload stored in the job for vm_delete jobs.
type VMDeletePayload struct {
	TenantID uuid.UUID `json:"tenant_id"`
	VMID     uuid.UUID `json:"vm_id"`
}

// Orchestrator implements Service and manages the async VM creation/deletion pipeline.
type Orchestrator struct {
	pool       *pgxpool.Pool
	flavorSvc  flavor.Service
	networkSvc network.Service
	storageSvc storage.Service
	scheduler  scheduler.Scheduler
	workers    *controller.WorkerClientPool
	quotaSvc   quota.Service
	queue      jobqueue.Queue
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
	queue jobqueue.Queue,
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
		queue:      queue,
		logger:     logger,
	}
}

// RegisterHandlers registers vm_create and vm_delete job handlers with the dispatcher.
func (o *Orchestrator) RegisterHandlers(d *jobqueue.Dispatcher) {
	d.Register(JobTypeVMCreate, o.handleVMCreate)
	d.Register(JobTypeVMDelete, o.handleVMDelete)
}

func (o *Orchestrator) handleVMCreate(ctx context.Context, job *jobqueue.Job) error {
	var p VMCreatePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("compute: vm_create handler: unmarshal payload: %w", err)
	}

	flv, err := o.flavorSvc.Get(ctx, p.Spec.FlavorID)
	if err != nil {
		return fmt.Errorf("compute: vm_create handler: resolve flavor: %w", err)
	}

	if err := o.buildVM(ctx, p.VMID, p.Spec, flv); err != nil {
		o.logger.Error("VM build failed", "vm_id", p.VMID, "job_id", job.ID, "error", err)
		if o.quotaSvc != nil {
			_ = o.quotaSvc.Release(context.Background(), quota.ResourceTypeVM, p.VMID)
		}
		_ = o.setVMStatus(context.Background(), p.VMID, VMStatusError, err.Error())
		return err
	}
	if o.quotaSvc != nil {
		if err := o.quotaSvc.Commit(context.Background(), quota.ResourceTypeVM, p.VMID); err != nil {
			o.logger.Warn("quota commit failed after VM build", "vm_id", p.VMID, "error", err)
		}
	}
	return nil
}

func (o *Orchestrator) handleVMDelete(ctx context.Context, job *jobqueue.Job) error {
	var p VMDeletePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("compute: vm_delete handler: unmarshal payload: %w", err)
	}

	vm, err := o.getVM(ctx, p.TenantID, p.VMID)
	if err != nil {
		return fmt.Errorf("compute: vm_delete handler: get vm: %w", err)
	}

	// Capture flavor for quota decommit after teardown.
	var vmFlavor *flavor.Flavor
	if vm.FlavorID != nil {
		if flv, err := o.flavorSvc.Get(ctx, *vm.FlavorID); err == nil {
			vmFlavor = flv
		}
	}

	if err := o.teardownVM(ctx, vm); err != nil {
		_ = o.setVMStatus(context.Background(), p.VMID, VMStatusError, err.Error())
		return err
	}
	_ = o.deleteVMRecord(context.Background(), p.VMID)
	if o.quotaSvc != nil && vmFlavor != nil {
		if err := o.quotaSvc.Decommit(context.Background(), vm.TenantID, quota.ResourceDelta{
			Vcpus: vmFlavor.VCPUs,
			RAMMB: int(vmFlavor.RAMMB),
			VMs:   1,
		}); err != nil {
			o.logger.Warn("quota decommit failed after VM teardown", "vm_id", p.VMID, "error", err)
		}
	}
	return nil
}

// CreateVM inserts a VM record in "pending" status, enqueues a vm_create job, and returns
// both the VM record and the job ID.
func (o *Orchestrator) CreateVM(ctx context.Context, spec CreateVMSpec) (*CreateVMResponse, error) {
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

	if o.queue == nil {
		_ = o.setVMStatus(context.Background(), vm.ID, VMStatusError, "jobqueue not configured")
		if o.quotaSvc != nil {
			_ = o.quotaSvc.Release(context.Background(), quota.ResourceTypeVM, vm.ID)
		}
		return nil, fmt.Errorf("compute: create vm: jobqueue not configured")
	}

	// Enqueue the vm_create job. The dispatcher will pick it up and call handleVMCreate.
	payload, err := json.Marshal(VMCreatePayload{Spec: spec, VMID: vm.ID})
	if err != nil {
		_ = o.setVMStatus(context.Background(), vm.ID, VMStatusError, "failed to marshal vm_create payload")
		if o.quotaSvc != nil {
			_ = o.quotaSvc.Release(context.Background(), quota.ResourceTypeVM, vm.ID)
		}
		return nil, fmt.Errorf("compute: create vm: marshal payload: %w", err)
	}
	tenantID := spec.TenantID
	job, err := o.queue.Enqueue(ctx, jobqueue.EnqueueParams{
		Type:      JobTypeVMCreate,
		Payload:   payload,
		TenantID:  &tenantID,
		CreatedBy: nil,
	})
	if err != nil {
		// If we fail to enqueue, set VM to error so it is not orphaned.
		_ = o.setVMStatus(context.Background(), vm.ID, VMStatusError, "failed to enqueue vm_create job")
		if o.quotaSvc != nil {
			_ = o.quotaSvc.Release(context.Background(), quota.ResourceTypeVM, vm.ID)
		}
		return nil, fmt.Errorf("compute: create vm: enqueue job: %w", err)
	}
	return &CreateVMResponse{VM: vm, JobID: job.ID}, nil
}

// GetVM returns a VM by ID for the given tenant.
func (o *Orchestrator) GetVM(ctx context.Context, tenantID, vmID uuid.UUID) (*VM, error) {
	return o.getVM(ctx, tenantID, vmID)
}

// ListVMs returns all VMs for the given tenant.
func (o *Orchestrator) ListVMs(ctx context.Context, tenantID uuid.UUID) ([]VM, error) {
	return o.listVMs(ctx, tenantID)
}

// DeleteVM marks the VM as "deleting" and enqueues a vm_delete job.
// Only allowed when status is stopped or error.
func (o *Orchestrator) DeleteVM(ctx context.Context, tenantID, vmID uuid.UUID) (*DeleteVMResponse, error) {
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return nil, err
	}
	if vm.IsTransitional() {
		return nil, ErrConflict
	}
	if !vm.CanDelete() {
		return nil, ErrConflict
	}
	if err := o.setVMStatus(ctx, vmID, VMStatusDeleting, ""); err != nil {
		return nil, fmt.Errorf("compute: delete vm: %w", err)
	}

	if o.queue == nil {
		return nil, fmt.Errorf("compute: delete vm: jobqueue not configured")
	}

	// Enqueue the vm_delete job.
	payload, err := json.Marshal(VMDeletePayload{TenantID: tenantID, VMID: vmID})
	if err != nil {
		return nil, fmt.Errorf("compute: delete vm: marshal payload: %w", err)
	}
	job, err := o.queue.Enqueue(ctx, jobqueue.EnqueueParams{
		Type:      JobTypeVMDelete,
		Payload:   payload,
		TenantID:  &tenantID,
		CreatedBy: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("compute: delete vm: enqueue job: %w", err)
	}
	return &DeleteVMResponse{JobID: job.ID}, nil
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

// MigrateVM live-migrates a running VM to a new host.
// If targetHostID is nil, the scheduler selects the destination.
func (o *Orchestrator) MigrateVM(ctx context.Context, tenantID, vmID uuid.UUID, targetHostID *uuid.UUID) (retErr error) {
	// 1. VM を取得・検証（running 状態のみ許可）
	vm, err := o.getVM(ctx, tenantID, vmID)
	if err != nil {
		return err
	}
	if vm.IsTransitional() {
		return ErrConflict
	}
	if vm.Status != VMStatusRunning {
		return ErrConflict
	}
	if vm.HostID == nil {
		return fmt.Errorf("compute: MigrateVM: VM has no assigned host")
	}

	// 2. ステータスを migrating に更新
	if err := o.setVMStatus(ctx, vmID, VMStatusMigrating, ""); err != nil {
		return fmt.Errorf("compute: MigrateVM: set status migrating: %w", err)
	}
	defer func() {
		if retErr != nil {
			// ロールバック: error ステータスに
			_ = o.setVMStatus(context.Background(), vmID, VMStatusError, retErr.Error())
		}
	}()

	// 3. フレーバーを取得（スケジューラーと AcceptMigratedVM で共用）
	var flv *flavor.Flavor
	if vm.FlavorID != nil {
		flv, err = o.flavorSvc.Get(ctx, *vm.FlavorID)
		if err != nil {
			return fmt.Errorf("compute: MigrateVM: get flavor: %w", err)
		}
	}

	// 3b. 宛先ホストを決定
	var destHostID uuid.UUID
	if targetHostID != nil {
		destHostID = *targetHostID
	} else {
		// Use AZID from the VM record; falls back to uuid.Nil (any AZ) if not set.
		var azID uuid.UUID
		if vm.AZID != nil {
			azID = *vm.AZID
		}
		result, err := o.scheduler.Reschedule(ctx, scheduler.RescheduleSpec{
			ExcludeHostID: *vm.HostID,
			AZID:          azID,
			Flavor:        flv,
		})
		if err != nil {
			return fmt.Errorf("compute: MigrateVM: reschedule: %w", err)
		}
		destHostID = result.HostID
	}

	// 4. 宛先ホストの WorkerClient を取得
	destHost, err := o.getHostByID(ctx, destHostID)
	if err != nil {
		return fmt.Errorf("compute: MigrateVM: get dest host: %w", err)
	}
	if destHost.WorkerGRPCAddr == "" {
		return fmt.Errorf("compute: MigrateVM: dest host has no worker_grpc_addr")
	}
	destWorker, err := o.workers.Get(destHost.WorkerGRPCAddr)
	if err != nil {
		return fmt.Errorf("compute: MigrateVM: get dest worker: %w", err)
	}

	// 5. 宛先ワーカーに PrepareMigration
	srcWorker, vmName, err := o.resolveWorker(ctx, vm)
	if err != nil {
		return fmt.Errorf("compute: MigrateVM: resolve src worker: %w", err)
	}

	// 5a. VM に紐づくポートを取得（FallbackRoute に必要）
	port, err := o.networkSvc.GetPortByVMID(ctx, vmID)
	if err != nil {
		return fmt.Errorf("compute: MigrateVM: get port for vm %s: %w", vmID, err)
	}

	if _, err := destWorker.PrepareMigration(ctx, &pb.PrepareMigrationRequest{
		VmId:      vmID.String(),
		VmName:    vmName,
		SrcHostId: vm.HostID.String(),
	}); err != nil {
		return fmt.Errorf("compute: MigrateVM: PrepareMigration: %w", err)
	}

	// 5b. 移行元ホストに FallbackRoute を設定
	// これにより src host がトラフィックを dest host にトンネル転送する
	fallbackID, err := o.insertFallbackRoute(ctx, port.ID, *vm.HostID, destHostID)
	if err != nil {
		return fmt.Errorf("compute: MigrateVM: insert fallback route: %w", err)
	}
	// FallbackRoute は migration 完了後（またはエラー時）に必ず削除する
	defer func() {
		if delErr := o.deleteFallbackRoute(context.Background(), fallbackID); delErr != nil {
			o.logger.Warn("MigrateVM: failed to delete fallback route", "fallback_id", fallbackID, "error", delErr)
		}
	}()

	// 5c. ネットワーク controller が次の poll で FallbackRoute を配信するまで待機
	// GRPCStateServer の pollInterval は 2 秒。3 秒待つことで少なくとも 1 poll 分の
	// マージンを確保する（本番実装では確認 ACK に切り替える予定: S023-2 の TODO）
	const migrationNetworkSettleTime = 3 * time.Second
	time.Sleep(migrationNetworkSettleTime)

	// 6. 移行元ワーカーに StartMigration
	if _, err := srcWorker.StartMigration(ctx, &pb.StartMigrationRequest{
		VmId:       vmID.String(),
		VmName:     vmName,
		DestHostId: destHostID.String(),
	}); err != nil {
		return fmt.Errorf("compute: MigrateVM: StartMigration: %w", err)
	}

	// 6.5. 移行先ワーカーに AcceptMigratedVM を通知（HostInstance sim モードで dest が VM を受け取る）
	var vcpus int32
	var ramMB int64
	if flv != nil {
		vcpus = int32(flv.VCPUs)
		ramMB = int64(flv.RAMMB)
	}
	if _, err := destWorker.AcceptMigratedVM(ctx, &pb.AcceptMigratedVMRequest{
		VmId:         vmID.String(),
		VmName:       vmName,
		Vcpus:        vcpus,
		RamMb:        ramMB,
		InterfaceIds: []string{port.ID.String()},
	}); err != nil {
		// Non-fatal: real libvirt handles this via migration protocol.
		// Sim may fail in single-process mode (dest already has the domain).
		o.logger.Warn("MigrateVM: AcceptMigratedVM failed (non-fatal)", "vm_id", vmID, "error", err)
	}

	// 7. DB 更新: host_id を移行先に変更し、status を running に戻す
	// この更新により全ホストの RemotePort が自動的に dest host を指すようになる
	if err := o.setVMHost(ctx, vmID, destHostID); err != nil {
		return fmt.Errorf("compute: MigrateVM: set vm host: %w", err)
	}
	if err := o.setVMStatus(ctx, vmID, VMStatusRunning, ""); err != nil {
		return fmt.Errorf("compute: MigrateVM: set status running: %w", err)
	}

	// 8. FallbackRoute を削除（defer で実行）
	// defer が実行されることで src host の fallback フローが次の poll で削除される

	o.logger.Info("VM migration complete", "vm_id", vmID, "dest_host_id", destHostID)
	return nil
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
// Cleanup defers are registered after each resource is created so that partial
// failures do not leave dangling ports or volumes.
func (o *Orchestrator) buildVM(ctx context.Context, vmID uuid.UUID, spec CreateVMSpec, flv *flavor.Flavor) (retErr error) {
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

	// 2. Persist host assignment immediately (for observability)
	if err := o.setVMHost(ctx, vmID, hostID); err != nil {
		return fmt.Errorf("persist host: %w", err)
	}

	// 2b. If no AZ was requested, resolve the placed host's AZ and record it.
	if spec.AZID == uuid.Nil {
		if resolvedAZ, err := o.resolveAZForHost(ctx, hostID); err == nil && resolvedAZ != uuid.Nil {
			_ = o.setVMAZ(ctx, vmID, resolvedAZ)
		}
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
		// Cleanup: if a subsequent step fails, delete the port we just created.
		defer func() {
			if retErr != nil && port != nil {
				if delErr := o.networkSvc.DeletePort(context.Background(), port.ID); delErr != nil {
					o.logger.Warn("buildVM cleanup: failed to delete port", "port_id", port.ID, "error", delErr)
				}
			}
		}()
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
	vol, err := o.storageSvc.SyncCreateVolume(ctx, volSpec)
	if err != nil {
		return fmt.Errorf("create volume: %w", err)
	}
	// Cleanup: if a subsequent step fails, delete the volume we just created.
	defer func() {
		if retErr != nil {
			if unexErr := o.storageSvc.UnexportVolume(context.Background(), vol.ID); unexErr != nil {
				o.logger.Warn("buildVM cleanup: unexport volume failed", "volume_id", vol.ID, "error", unexErr)
			}
			if delErr := o.storageSvc.SyncDeleteVolume(context.Background(), spec.TenantID, vol.ID); delErr != nil {
				o.logger.Warn("buildVM cleanup: delete volume failed", "volume_id", vol.ID, "error", delErr)
			}
		}
	}()

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
// All errors are collected and logged at the end; individual step failures do not
// abort subsequent steps.
func (o *Orchestrator) teardownVM(ctx context.Context, vm *VM) error {
	vmName := fmt.Sprintf("vm-%s", vm.ID.String()[:8])

	var errs []error

	volumeIDs, err := o.listVMVolumeIDs(ctx, vm.ID)
	if err != nil {
		o.logger.Warn("teardownVM: list vm volumes failed", "vm_id", vm.ID, "error", err)
		errs = append(errs, fmt.Errorf("list volumes: %w", err))
	}

	if vm.HostID != nil {
		h, err := o.getHostByID(ctx, *vm.HostID)
		if err != nil {
			o.logger.Warn("teardownVM: get host failed", "vm_id", vm.ID, "error", err)
			errs = append(errs, fmt.Errorf("get host: %w", err))
		} else if h.WorkerGRPCAddr != "" {
			workerClient, err := o.workers.Get(h.WorkerGRPCAddr)
			if err != nil {
				o.logger.Warn("teardownVM: get worker client failed", "vm_id", vm.ID, "error", err)
				errs = append(errs, fmt.Errorf("get worker client: %w", err))
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
					errs = append(errs, fmt.Errorf("worker DeleteVM: %w", err))
				}
			}
		}
	}

	port, err := o.networkSvc.GetPortByVMID(ctx, vm.ID)
	if err == nil {
		if err := o.networkSvc.DeletePort(ctx, port.ID); err != nil {
			o.logger.Warn("teardownVM: delete port failed", "vm_id", vm.ID, "error", err)
			errs = append(errs, fmt.Errorf("delete port: %w", err))
		}
	}

	// UnexportVolume failure should not block DeleteVolume so we continue regardless.
	for _, vid := range volumeIDs {
		if err := o.storageSvc.UnexportVolume(ctx, vid); err != nil {
			o.logger.Warn("teardownVM: unexport volume failed", "volume_id", vid, "error", err)
			errs = append(errs, fmt.Errorf("unexport volume %s: %w", vid, err))
		}
		// Delete even if unexport failed — best effort; reconciler handles drift.
		if err := o.storageSvc.SyncDeleteVolume(ctx, vm.TenantID, vid); err != nil {
			o.logger.Warn("teardownVM: delete volume failed", "volume_id", vid, "error", err)
			errs = append(errs, fmt.Errorf("delete volume %s: %w", vid, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardownVM: %d error(s): %w", len(errs), errors.Join(errs...))
	}
	return nil
}
