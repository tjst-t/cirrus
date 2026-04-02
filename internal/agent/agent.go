package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/tjst-t/cirrus/internal/hypervisor"
	netagent "github.com/tjst-t/cirrus/internal/network/agent"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// Agent is the worker-side process that connects to the controller.
type Agent struct {
	hostID            string
	registrationToken string
	conn              *grpc.ClientConn
	client            pb.ControllerServiceClient
	driver            hypervisor.Driver
	logger            *slog.Logger
}

// New creates an Agent that connects to the controller's gRPC endpoint.
func New(controllerAddr, hostID string, logger *slog.Logger, driver hypervisor.Driver) (*Agent, error) {
	conn, err := grpc.NewClient(controllerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("agent: dial controller %s: %w", controllerAddr, err)
	}
	return &Agent{
		hostID: hostID,
		conn:   conn,
		client: pb.NewControllerServiceClient(conn),
		driver: driver,
		logger: logger,
	}, nil
}

// TopologyDeclaration holds the topology information a worker declares at registration.
type TopologyDeclaration struct {
	StorageDomains []string
	Location       string
}

// Register performs self-registration with the controller using the given token.
// workerGRPCAddr is the address (host:port) at which the controller can reach this worker's WorkerService.
// On success, it stores the assigned host UUID for use in subsequent heartbeats.
func (a *Agent) Register(ctx context.Context, token, libvirtURI, fabricIP, workerGRPCAddr string, topo *TopologyDeclaration) error {
	// HOSTNAME_OVERRIDE allows multiple workers on the same machine (dev/sim)
	hostname := os.Getenv("HOSTNAME_OVERRIDE")
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return fmt.Errorf("agent: get hostname: %w", err)
		}
	}

	// Use libvirt URI as address for the host
	address := libvirtURI

	a.logger.Info("registering with controller", "hostname", hostname, "address", address)

	req := &pb.RegisterHostRequest{
		RegistrationToken: token,
		Hostname:          hostname,
		Address:           address,
		FabricIp:          fabricIP,
		WorkerGrpcAddr:    workerGRPCAddr,
	}
	if topo != nil {
		req.StorageDomains = topo.StorageDomains
		req.Location = topo.Location
	}

	resp, err := a.client.RegisterHost(ctx, req)
	if err != nil {
		return fmt.Errorf("agent: register: %w", err)
	}
	if !resp.Accepted {
		return fmt.Errorf("agent: registration rejected: %s", resp.Message)
	}

	a.hostID = resp.HostId
	a.registrationToken = token
	a.logger.Info("registered with controller", "host_id", a.hostID, "hostname", hostname)
	return nil
}

// RunHeartbeat sends periodic heartbeats to the controller.
// Blocks until ctx is cancelled.
func (a *Agent) RunHeartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.logger.Info("heartbeat loop started", "host_id", a.hostID, "interval", interval)

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("heartbeat loop stopped", "host_id", a.hostID)
			return
		case <-ticker.C:
			report := a.collectResources(ctx)

			resp, err := a.client.Heartbeat(ctx, &pb.HeartbeatRequest{
				HostId:            a.hostID,
				Resources:         report,
				RegistrationToken: a.registrationToken,
			})
			if err != nil {
				a.logger.Warn("heartbeat failed", "host_id", a.hostID, "error", err)
				continue
			}
			a.logger.Debug("heartbeat sent",
				"host_id", a.hostID,
				"accepted", resp.Accepted,
				"used_vcpus", report.UsedVcpus,
				"used_ram_mb", report.UsedRamMb,
			)
		}
	}
}

// collectResources gathers resource usage from the hypervisor driver.
func (a *Agent) collectResources(ctx context.Context) *pb.ResourceReport {
	report := &pb.ResourceReport{}
	if a.driver == nil {
		return report
	}

	vms, err := a.driver.ListVMs(ctx)
	if err != nil {
		a.logger.Warn("failed to list VMs for resource report", "error", err)
		return report
	}

	for _, vm := range vms {
		if vm.State == hypervisor.StateRunning {
			report.UsedVcpus += vm.Vcpus
			report.UsedRamMb += vm.RAMMb
			report.RunningVms = append(report.RunningVms, &pb.VMInfo{
				VmId:   vm.ID,
				Status: string(vm.State),
				Vcpus:  vm.Vcpus,
				RamMb:  vm.RAMMb,
			})
		}
	}

	return report
}

// HostID returns the agent's host ID (assigned after registration).
func (a *Agent) HostID() string {
	return a.hostID
}

// CreateNetworkAgent creates a NetworkAgent that shares this agent's gRPC connection.
// Returns nil if the host ID is not set (not registered).
func (a *Agent) CreateNetworkAgent(controllerAddr, regToken string, logger *slog.Logger) *netagent.NetworkAgent {
	if a.hostID == "" {
		return nil
	}
	// Use real OVS client if OVS is available, otherwise run in state-only mode
	var ovsClient netagent.OVSClient
	if netagent.IsOVSAvailable() {
		ovsClient = netagent.NewExecOVSClient(netagent.BridgeName, logger)
		logger.Info("OVS client connected", "bridge", netagent.BridgeName)
	} else {
		logger.Warn("OVS not available, running in state-only mode")
	}
	return netagent.New(netagent.Config{
		HostID:         a.hostID,
		ControllerAddr: controllerAddr,
		RegToken:       regToken,
		Logger:         logger,
	}, a.conn, ovsClient)
}

// Close shuts down the agent's gRPC connection.
func (a *Agent) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
}
