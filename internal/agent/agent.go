package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// Agent is the worker-side process that connects to the controller.
type Agent struct {
	hostID string
	conn   *grpc.ClientConn
	client pb.ControllerServiceClient
	logger *slog.Logger
}

// New creates an Agent that connects to the controller's gRPC endpoint.
func New(controllerAddr, hostID string, logger *slog.Logger) (*Agent, error) {
	conn, err := grpc.NewClient(controllerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("agent: dial controller %s: %w", controllerAddr, err)
	}
	return &Agent{
		hostID: hostID,
		conn:   conn,
		client: pb.NewControllerServiceClient(conn),
		logger: logger,
	}, nil
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
			resp, err := a.client.Heartbeat(ctx, &pb.HeartbeatRequest{
				HostId:    a.hostID,
				Resources: &pb.ResourceReport{},
			})
			if err != nil {
				a.logger.Warn("heartbeat failed", "host_id", a.hostID, "error", err)
				continue
			}
			a.logger.Debug("heartbeat sent", "host_id", a.hostID, "accepted", resp.Accepted)
		}
	}
}

// Close shuts down the agent's gRPC connection.
func (a *Agent) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
}
