package controller

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"

	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// GRPCServer implements the ControllerService that workers connect to.
type GRPCServer struct {
	pb.UnimplementedControllerServiceServer
	logger *slog.Logger
}

// NewGRPCServer creates a new gRPC server with the ControllerService registered.
func NewGRPCServer(logger *slog.Logger) *grpc.Server {
	srv := grpc.NewServer()
	pb.RegisterControllerServiceServer(srv, &GRPCServer{logger: logger})
	return srv
}

// Heartbeat receives heartbeat from a worker.
func (s *GRPCServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.logger.Info("heartbeat received",
		"host_id", req.HostId,
		"time", time.Now().Format(time.RFC3339),
	)
	return &pb.HeartbeatResponse{Accepted: true}, nil
}
