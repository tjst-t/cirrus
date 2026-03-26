package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/storage"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

type Agent struct {
	pb.UnimplementedWorkerAgentServer
	compute compute.Driver
	network network.Provider
	storage storage.Backend
	logger  *slog.Logger
}

func New(c compute.Driver, n network.Provider, s storage.Backend, logger *slog.Logger) *Agent {
	return &Agent{
		compute: c,
		network: n,
		storage: s,
		logger:  logger,
	}
}

func (a *Agent) Serve(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	srv := grpc.NewServer()
	pb.RegisterWorkerAgentServer(srv, a)

	a.logger.Info("worker agent listening", "addr", addr)
	return srv.Serve(lis)
}

func (a *Agent) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.CreateVMResponse, error) {
	a.logger.Info("creating VM", "vm_id", req.VmId, "name", req.Name)

	// Create disk
	baseImage := req.Disk.BaseImagePath
	diskResult, err := a.storage.CreateDisk(ctx, req.VmId, baseImage, int(req.Disk.SizeGb))
	if err != nil {
		a.logger.Error("create disk failed", "vm_id", req.VmId, "error", err)
		return &pb.CreateVMResponse{Success: false, Error: err.Error()}, nil
	}

	// Get disk spec for libvirt
	diskSpec := a.storage.DiskSpec(req.VmId, diskResult.DriverData)

	// Build cloud-init with network config
	cloudInit := buildCloudInit(req)

	// Build port specs
	var ports []compute.PortSpec
	for _, p := range req.Ports {
		ports = append(ports, compute.PortSpec{
			ID:  p.Id,
			MAC: p.MacAddress,
		})
	}

	// Create VM via compute driver
	vmSpec := compute.VMSpec{
		ID:    req.VmId,
		Name:  req.Name,
		VCPUs: int(req.Vcpus),
		RamMB: int(req.RamMb),
		Disk: compute.DiskSpec{
			Type:   diskSpec.Type,
			Source: diskSpec.Source,
			Format: diskSpec.Format,
		},
		Ports:     ports,
		CloudInit: cloudInit,
	}

	if err := a.compute.CreateVM(ctx, vmSpec); err != nil {
		a.logger.Error("compute create VM failed", "vm_id", req.VmId, "error", err)
		return &pb.CreateVMResponse{Success: false, Error: err.Error()}, nil
	}

	// Configure network ports
	for _, p := range req.Ports {
		if err := a.network.AttachPort(ctx, network.PortConfig{
			ID:  p.Id,
			MAC: p.MacAddress,
			VNI: int(p.Vni),
		}); err != nil {
			a.logger.Error("attach port failed", "port_id", p.Id, "error", err)
			return &pb.CreateVMResponse{Success: false, Error: fmt.Sprintf("attach port %s: %v", p.Id, err)}, nil
		}
	}

	a.logger.Info("VM created successfully", "vm_id", req.VmId)
	return &pb.CreateVMResponse{Success: true}, nil
}

func (a *Agent) DeleteVM(ctx context.Context, req *pb.DeleteVMRequest) (*pb.DeleteVMResponse, error) {
	a.logger.Info("deleting VM", "vm_id", req.VmId)

	if err := a.compute.DeleteVM(ctx, req.VmId); err != nil {
		a.logger.Error("compute delete VM failed", "vm_id", req.VmId, "error", err)
		return &pb.DeleteVMResponse{Success: false, Error: err.Error()}, nil
	}

	if err := a.storage.DeleteDisk(ctx, req.VmId, nil); err != nil {
		a.logger.Error("delete disk failed", "vm_id", req.VmId, "error", err)
	}

	return &pb.DeleteVMResponse{Success: true}, nil
}

func (a *Agent) StopVM(ctx context.Context, req *pb.StopVMRequest) (*pb.StopVMResponse, error) {
	if err := a.compute.StopVM(ctx, req.VmId); err != nil {
		return &pb.StopVMResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.StopVMResponse{Success: true}, nil
}

func (a *Agent) StartVM(ctx context.Context, req *pb.StartVMRequest) (*pb.StartVMResponse, error) {
	if err := a.compute.StartVM(ctx, req.VmId); err != nil {
		return &pb.StartVMResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.StartVMResponse{Success: true}, nil
}

func (a *Agent) GetVMStatus(ctx context.Context, req *pb.GetVMStatusRequest) (*pb.VMStatusResponse, error) {
	status, err := a.compute.GetStatus(ctx, req.VmId)
	if err != nil {
		return &pb.VMStatusResponse{VmId: req.VmId, Status: "error"}, nil
	}
	return &pb.VMStatusResponse{VmId: req.VmId, Status: status.Status}, nil
}

func (a *Agent) ConfigurePort(ctx context.Context, req *pb.ConfigurePortRequest) (*pb.ConfigurePortResponse, error) {
	if err := a.network.AttachPort(ctx, network.PortConfig{
		ID:  req.PortId,
		MAC: req.MacAddress,
		VNI: int(req.Vni),
	}); err != nil {
		return &pb.ConfigurePortResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.ConfigurePortResponse{Success: true}, nil
}

func (a *Agent) RemovePort(ctx context.Context, req *pb.RemovePortRequest) (*pb.RemovePortResponse, error) {
	if err := a.network.DetachPort(ctx, req.PortId); err != nil {
		return &pb.RemovePortResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.RemovePortResponse{Success: true}, nil
}

func (a *Agent) ConfigureTunnel(ctx context.Context, req *pb.ConfigureTunnelRequest) (*pb.ConfigureTunnelResponse, error) {
	if err := a.network.AddTunnel(ctx, req.PeerAddress, req.PeerName); err != nil {
		return &pb.ConfigureTunnelResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.ConfigureTunnelResponse{Success: true}, nil
}

func (a *Agent) RemoveTunnel(ctx context.Context, req *pb.RemoveTunnelRequest) (*pb.RemoveTunnelResponse, error) {
	if err := a.network.RemoveTunnel(ctx, req.PeerAddress, req.PeerName); err != nil {
		return &pb.RemoveTunnelResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.RemoveTunnelResponse{Success: true}, nil
}

func buildCloudInit(req *pb.CreateVMRequest) []byte {
	ci := "#cloud-config\n"
	if req.SshPublicKey != "" {
		ci += "ssh_authorized_keys:\n"
		ci += "  - " + req.SshPublicKey + "\n"
	}

	// Network config via cloud-init
	if len(req.Ports) > 0 {
		p := req.Ports[0]
		ci += fmt.Sprintf(`
write_files:
  - path: /etc/netplan/50-cirrus.yaml
    content: |
      network:
        version: 2
        ethernets:
          ens2:
            addresses:
              - %s/%s
            gateway4: %s
`, p.IpAddress, cidrMask(p.Cidr), p.Gateway)
	}
	return []byte(ci)
}

func cidrMask(cidr string) string {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "24"
	}
	ones, _ := ipNet.Mask.Size()
	return fmt.Sprintf("%d", ones)
}
