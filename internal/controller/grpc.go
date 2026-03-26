package controller

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/host"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// GRPCServer implements the ControllerService that workers connect to.
type GRPCServer struct {
	pb.UnimplementedControllerServiceServer
	hostSvc host.Service
	logger  *slog.Logger
}

// NewGRPCServer creates a new gRPC server with the ControllerService registered.
func NewGRPCServer(logger *slog.Logger, hostSvc host.Service) *grpc.Server {
	srv := grpc.NewServer()
	pb.RegisterControllerServiceServer(srv, &GRPCServer{
		hostSvc: hostSvc,
		logger:  logger,
	})
	return srv
}

// Heartbeat receives heartbeat from a worker.
func (s *GRPCServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.logger.Debug("heartbeat received",
		"host_id", req.HostId,
		"time", time.Now().Format(time.RFC3339),
		"used_vcpus", req.Resources.GetUsedVcpus(),
		"used_ram_mb", req.Resources.GetUsedRamMb(),
		"running_vms", len(req.Resources.GetRunningVms()),
	)

	if s.hostSvc != nil {
		report := host.ResourceReport{}
		if req.Resources != nil {
			report.UsedVcpus = req.Resources.UsedVcpus
			report.UsedRAMMB = req.Resources.UsedRamMb
		}
		if err := s.hostSvc.Heartbeat(ctx, req.HostId, report); err != nil {
			s.logger.Warn("heartbeat db update failed", "host_id", req.HostId, "error", err)
		}
	}

	return &pb.HeartbeatResponse{Accepted: true}, nil
}
