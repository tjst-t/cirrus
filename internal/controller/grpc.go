package controller

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"time"

	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/host"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// GRPCServer implements the ControllerService that workers connect to.
type GRPCServer struct {
	pb.UnimplementedControllerServiceServer
	hostSvc           host.Service
	logger            *slog.Logger
	registrationToken string
}

// NewGRPCServer creates a new gRPC server with the ControllerService registered.
func NewGRPCServer(logger *slog.Logger, hostSvc host.Service, registrationToken string) *grpc.Server {
	srv := grpc.NewServer()
	pb.RegisterControllerServiceServer(srv, &GRPCServer{
		hostSvc:           hostSvc,
		logger:            logger,
		registrationToken: registrationToken,
	})
	return srv
}

// RegisterHost handles worker self-registration.
func (s *GRPCServer) RegisterHost(ctx context.Context, req *pb.RegisterHostRequest) (*pb.RegisterHostResponse, error) {
	// Validate registration token
	if s.registrationToken == "" {
		s.logger.Warn("registration rejected: no registration token configured on controller")
		return &pb.RegisterHostResponse{Accepted: false, Message: "registration not enabled"}, nil
	}
	if subtle.ConstantTimeCompare([]byte(req.RegistrationToken), []byte(s.registrationToken)) != 1 {
		s.logger.Warn("registration rejected: invalid token", "hostname", req.Hostname)
		return &pb.RegisterHostResponse{Accepted: false, Message: "invalid registration token"}, nil
	}

	if req.Hostname == "" {
		return &pb.RegisterHostResponse{Accepted: false, Message: "hostname is required"}, nil
	}

	h, err := s.hostSvc.RegisterOrGet(ctx, req.Hostname, req.Address, req.Capability)
	if err != nil {
		s.logger.Error("registration failed", "hostname", req.Hostname, "error", err)
		return &pb.RegisterHostResponse{Accepted: false, Message: "registration failed"}, nil
	}

	s.logger.Info("host registered",
		"host_id", h.ID.String(),
		"hostname", h.Name,
		"address", h.Address,
		"state", h.OperationalState,
	)

	return &pb.RegisterHostResponse{
		HostId:   h.ID.String(),
		Accepted: true,
		Message:  "registered",
	}, nil
}

// Heartbeat receives heartbeat from a worker.
func (s *GRPCServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	// Authenticate heartbeat with registration token
	if s.registrationToken != "" {
		if subtle.ConstantTimeCompare([]byte(req.RegistrationToken), []byte(s.registrationToken)) != 1 {
			s.logger.Warn("heartbeat rejected: invalid token", "host_id", req.HostId)
			return &pb.HeartbeatResponse{Accepted: false}, nil
		}
	}

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
			s.logger.Warn("heartbeat rejected", "host_id", req.HostId, "error", err)
			return &pb.HeartbeatResponse{Accepted: false}, nil
		}
	}

	return &pb.HeartbeatResponse{Accepted: true}, nil
}
