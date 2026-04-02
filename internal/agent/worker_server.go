package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/blockdev"
	"github.com/tjst-t/cirrus/internal/hypervisor"
	"github.com/tjst-t/cirrus/internal/storage"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// WorkerServer implements the gRPC WorkerService on the worker side.
// The controller calls CreateVM/DeleteVM on this server to manage VMs.
type WorkerServer struct {
	pb.UnimplementedWorkerServiceServer
	driver   hypervisor.Driver
	blockMgr blockdev.Manager
	logger   *slog.Logger
}

// NewWorkerServer creates a new WorkerServer.
func NewWorkerServer(driver hypervisor.Driver, blockMgr blockdev.Manager, logger *slog.Logger) *WorkerServer {
	return &WorkerServer{
		driver:   driver,
		blockMgr: blockMgr,
		logger:   logger,
	}
}

// CreateVM attaches disks, defines, and starts a VM.
// Steps: for each disk, Attach → collect device paths; then DefineVM (with interfaces) → StartVM.
func (s *WorkerServer) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.CreateVMResponse, error) {
	s.logger.Info("CreateVM called", "vm_id", req.VmId, "name", req.Name)

	// Attach disks via blockMgr using the ExportInfo encoded in protocol+params.
	var disks []hypervisor.DiskSpec
	for i, d := range req.Disks {
		info := &storage.ExportInfo{Protocol: d.Protocol, Params: d.Params}
		result, err := s.blockMgr.Attach(ctx, info)
		if err != nil {
			return nil, fmt.Errorf("CreateVM: attach disk %d: %w", i, err)
		}
		targetDev := d.TargetDev
		if targetDev == "" {
			targetDev = fmt.Sprintf("vd%c", rune('a'+i))
		}
		disks = append(disks, hypervisor.DiskSpec{
			DevicePath: result.DevicePath,
			TargetDev:  targetDev,
		})
	}

	// Build interface specs
	var ifaces []hypervisor.InterfaceSpec
	for _, p := range req.Ports {
		ifaces = append(ifaces, hypervisor.InterfaceSpec{
			PortID:     p.PortId,
			MACAddress: p.MacAddress,
			BridgeName: p.BridgeName,
		})
	}

	// Build cloud-init spec
	var cloudInit *hypervisor.CloudInitSpec
	if req.CloudInit != nil && req.CloudInit.Hostname != "" {
		cloudInit = &hypervisor.CloudInitSpec{
			Hostname:      req.CloudInit.Hostname,
			UserData:      req.CloudInit.UserData,
			MetaData:      req.CloudInit.MetaData,
			NetworkConfig: req.CloudInit.NetworkConfig,
		}
	}

	spec := hypervisor.VMSpec{
		Name:       req.Name,
		VCPUs:      req.Vcpus,
		RAMMB:      req.RamMb,
		Disks:      disks,
		Interfaces: ifaces,
		CloudInit:  cloudInit,
	}

	info, err := s.driver.DefineVM(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("CreateVM: DefineVM: %w", err)
	}

	s.logger.Info("VM created", "vm_id", req.VmId, "name", req.Name, "state", info.State)
	return &pb.CreateVMResponse{
		VmId:   req.VmId,
		Status: string(info.State),
	}, nil
}

// DeleteVM destroys and undefines a VM, then detaches its disks.
func (s *WorkerServer) DeleteVM(ctx context.Context, req *pb.DeleteVMRequest) (*pb.DeleteVMResponse, error) {
	s.logger.Info("DeleteVM called", "vm_id", req.VmId, "name", req.Name)

	// Destroy (force-off) then undefine; ignore errors if already gone.
	if err := s.driver.DestroyVM(ctx, req.Name); err != nil {
		s.logger.Warn("DestroyVM failed (may already be stopped)", "name", req.Name, "error", err)
	}
	if err := s.driver.UndefineVM(ctx, req.Name); err != nil {
		s.logger.Warn("UndefineVM failed (may already be undefined)", "name", req.Name, "error", err)
	}

	// Detach disks
	for _, d := range req.Disks {
		info := &storage.ExportInfo{Protocol: d.Protocol, Params: d.Params}
		if err := s.blockMgr.Detach(ctx, info); err != nil {
			s.logger.Warn("detach disk failed", "error", err)
		}
	}

	return &pb.DeleteVMResponse{}, nil
}

// StartGRPCServer starts a gRPC server for WorkerService on the given listener.
// It blocks until the context is cancelled.
func StartGRPCServer(ctx context.Context, lis net.Listener, srv *WorkerServer, logger *slog.Logger) error {
	grpcSrv := grpc.NewServer()
	pb.RegisterWorkerServiceServer(grpcSrv, srv)

	go func() {
		<-ctx.Done()
		grpcSrv.GracefulStop()
	}()

	logger.Info("worker gRPC server starting", "addr", lis.Addr())
	if err := grpcSrv.Serve(lis); err != nil && err != grpc.ErrServerStopped {
		return fmt.Errorf("worker grpc: %w", err)
	}
	return nil
}
