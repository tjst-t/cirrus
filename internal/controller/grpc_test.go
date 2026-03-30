package controller_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/controller"
	"github.com/tjst-t/cirrus/internal/host"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
)

// mockHostSvc is a minimal host.Service for testing gRPC handlers.
type mockHostSvc struct {
	hosts      map[string]*host.Host // name -> host
	heartbeats map[string]int        // hostID -> count
}

func newMockHostSvc() *mockHostSvc {
	return &mockHostSvc{
		hosts:      make(map[string]*host.Host),
		heartbeats: make(map[string]int),
	}
}

func (m *mockHostSvc) RegisterOrGet(_ context.Context, name, address, capability string) (*host.Host, bool, error) {
	if h, ok := m.hosts[name]; ok {
		return h, false, nil
	}
	h := &host.Host{
		ID:               uuid.New(),
		Name:             name,
		Address:          address,
		OperationalState: host.StateRegistering,
		Capability:       json.RawMessage("{}"),
		ResourcePhysical: json.RawMessage("{}"),
		OvercommitRatios: json.RawMessage("{}"),
		ResourceUsed:     json.RawMessage("{}"),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	m.hosts[name] = h
	return h, true, nil
}

func (m *mockHostSvc) Heartbeat(_ context.Context, hostID string, _ host.ResourceReport) error {
	// Check if host exists by ID
	for _, h := range m.hosts {
		if h.ID.String() == hostID {
			m.heartbeats[hostID]++
			return nil
		}
	}
	return host.ErrNotFound
}

// Unused methods to satisfy host.Service interface
func (m *mockHostSvc) Register(context.Context, *uuid.UUID, string, string) (*host.Host, error) {
	return nil, nil
}
func (m *mockHostSvc) GetHost(context.Context, uuid.UUID) (*host.Host, error) { return nil, nil }
func (m *mockHostSvc) ListHosts(context.Context) ([]host.Host, error)         { return nil, nil }
func (m *mockHostSvc) ListHostsByState(context.Context, host.OperationalState) ([]host.Host, error) {
	return nil, nil
}
func (m *mockHostSvc) DeleteHost(context.Context, uuid.UUID) error                    { return nil }
func (m *mockHostSvc) UpdateCapability(context.Context, uuid.UUID, []byte) error      { return nil }
func (m *mockHostSvc) UpdateResourcePhysical(context.Context, uuid.UUID, []byte) error { return nil }
func (m *mockHostSvc) UpdateOvercommitRatios(context.Context, uuid.UUID, []byte) error { return nil }
func (m *mockHostSvc) SetOperationalState(context.Context, uuid.UUID, host.OperationalState) error {
	return nil
}
func (m *mockHostSvc) GetAllocatable(context.Context, uuid.UUID) (*host.AllocatableResources, error) {
	return nil, nil
}

// startTestServer starts a gRPC server on a random port and returns the client and cleanup func.
func startTestServer(t *testing.T, svc host.Service, regToken string) (pb.ControllerServiceClient, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := controller.NewGRPCServer(slog.Default(), svc, nil, nil, regToken)
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := pb.NewControllerServiceClient(conn)
	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
	}
	return client, cleanup
}

func TestRegisterHost_InvalidToken(t *testing.T) {
	svc := newMockHostSvc()
	client, cleanup := startTestServer(t, svc, "valid-token")
	defer cleanup()

	resp, err := client.RegisterHost(context.Background(), &pb.RegisterHostRequest{
		RegistrationToken: "wrong-token",
		Hostname:          "host-001",
		Address:           "tcp://localhost:16510",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Accepted {
		t.Fatal("expected registration to be rejected with invalid token")
	}
	if len(svc.hosts) != 0 {
		t.Fatal("host should not have been created")
	}
}

func TestRegisterHost_EmptyToken(t *testing.T) {
	svc := newMockHostSvc()
	client, cleanup := startTestServer(t, svc, "valid-token")
	defer cleanup()

	resp, err := client.RegisterHost(context.Background(), &pb.RegisterHostRequest{
		RegistrationToken: "",
		Hostname:          "host-001",
		Address:           "tcp://localhost:16510",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Accepted {
		t.Fatal("expected registration to be rejected with empty token")
	}
}

func TestRegisterHost_ValidToken_ThenHeartbeat(t *testing.T) {
	svc := newMockHostSvc()
	client, cleanup := startTestServer(t, svc, "valid-token")
	defer cleanup()

	// 1. Register
	resp, err := client.RegisterHost(context.Background(), &pb.RegisterHostRequest{
		RegistrationToken: "valid-token",
		Hostname:          "host-001",
		Address:           "tcp://localhost:16510",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("expected accepted, got rejected: %s", resp.Message)
	}
	if resp.HostId == "" {
		t.Fatal("expected non-empty host_id")
	}

	// 2. Heartbeat with assigned UUID and valid token
	hbResp, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		HostId:            resp.HostId,
		RegistrationToken: "valid-token",
		Resources:         &pb.ResourceReport{UsedVcpus: 2, UsedRamMb: 1024},
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !hbResp.Accepted {
		t.Fatal("heartbeat should be accepted for registered host")
	}
	if svc.heartbeats[resp.HostId] != 1 {
		t.Fatalf("expected 1 heartbeat, got %d", svc.heartbeats[resp.HostId])
	}
}

func TestHeartbeat_InvalidToken(t *testing.T) {
	svc := newMockHostSvc()
	client, cleanup := startTestServer(t, svc, "valid-token")
	defer cleanup()

	resp, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		HostId:            uuid.New().String(),
		RegistrationToken: "wrong-token",
		Resources:         &pb.ResourceReport{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Accepted {
		t.Fatal("heartbeat should be rejected with invalid token")
	}
}

func TestRegisterHost_Idempotent(t *testing.T) {
	svc := newMockHostSvc()
	client, cleanup := startTestServer(t, svc, "valid-token")
	defer cleanup()

	req := &pb.RegisterHostRequest{
		RegistrationToken: "valid-token",
		Hostname:          "host-dup",
		Address:           "tcp://localhost:16510",
	}

	// Register twice
	resp1, err := client.RegisterHost(context.Background(), req)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	resp2, err := client.RegisterHost(context.Background(), req)
	if err != nil {
		t.Fatalf("second register: %v", err)
	}

	if !resp1.Accepted || !resp2.Accepted {
		t.Fatal("both registrations should be accepted")
	}
	if resp1.HostId != resp2.HostId {
		t.Fatalf("idempotent registration should return same host_id: %s != %s", resp1.HostId, resp2.HostId)
	}
	if len(svc.hosts) != 1 {
		t.Fatalf("expected 1 host in store, got %d", len(svc.hosts))
	}
}
