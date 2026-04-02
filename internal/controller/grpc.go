package controller

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/topology"
	pb "github.com/tjst-t/cirrus/proto/agentpb"
	networkpb "github.com/tjst-t/cirrus/proto/networkpb"
)

// GRPCServer implements the ControllerService that workers connect to.
type GRPCServer struct {
	pb.UnimplementedControllerServiceServer
	hostSvc           host.Service
	topologySvc       topology.Service
	logger            *slog.Logger
	registrationToken string
}

// NewGRPCServer creates a new gRPC server with the ControllerService and
// NetworkStateService registered.
func NewGRPCServer(logger *slog.Logger, hostSvc host.Service, topologySvc topology.Service, networkStateSrv *network.GRPCStateServer, registrationToken string) *grpc.Server {
	srv := grpc.NewServer()
	pb.RegisterControllerServiceServer(srv, &GRPCServer{
		hostSvc:           hostSvc,
		topologySvc:       topologySvc,
		logger:            logger,
		registrationToken: registrationToken,
	})
	if networkStateSrv != nil {
		networkpb.RegisterNetworkStateServiceServer(srv, networkStateSrv)
	}
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

	// Validate fabric_ip if provided
	fabricIP := req.FabricIp
	if fabricIP != "" && net.ParseIP(fabricIP) == nil {
		s.logger.Warn("registration rejected: invalid fabric_ip", "hostname", req.Hostname, "fabric_ip", fabricIP)
		return &pb.RegisterHostResponse{Accepted: false, Message: "invalid fabric_ip"}, nil
	}

	h, created, err := s.hostSvc.RegisterOrGet(ctx, req.Hostname, req.Address, req.WorkerGrpcAddr, fabricIP, req.Capability)
	if err != nil {
		s.logger.Error("registration failed", "hostname", req.Hostname, "error", err)
		return &pb.RegisterHostResponse{Accepted: false, Message: "registration failed"}, nil
	}

	s.logger.Info("host registered",
		"host_id", h.ID.String(),
		"hostname", h.Name,
		"address", h.Address,
		"state", h.OperationalState,
		"created", created,
	)

	// Apply topology declarations only on initial registration to avoid
	// overwriting admin corrections when a worker re-registers.
	if created && s.topologySvc != nil {
		s.applyTopology(ctx, h.ID, req)
	}

	return &pb.RegisterHostResponse{
		HostId:   h.ID.String(),
		Accepted: true,
		Message:  "registered",
	}, nil
}

// applyTopology associates a host with declared topology resources.
// Invalid references are logged as warnings but do not fail registration.
func (s *GRPCServer) applyTopology(ctx context.Context, hostID uuid.UUID, req *pb.RegisterHostRequest) {
	for _, sd := range req.StorageDomains {
		sdID, err := s.resolveStorageDomain(ctx, sd)
		if err != nil {
			s.logger.Warn("topology: storage domain not found, skipping",
				"host_id", hostID, "storage_domain", sd, "error", err)
			continue
		}
		if err := s.topologySvc.AssociateHostStorageDomain(ctx, hostID, sdID); err != nil {
			s.logger.Warn("topology: failed to associate storage domain",
				"host_id", hostID, "storage_domain_id", sdID, "error", err)
		} else {
			s.logger.Info("topology: storage domain associated", "host_id", hostID, "storage_domain_id", sdID)
		}
	}

	if req.Location != "" {
		locID, err := s.resolveLocation(ctx, req.Location)
		if err != nil {
			s.logger.Warn("topology: location not found, skipping",
				"host_id", hostID, "location", req.Location, "error", err)
		} else if err := s.topologySvc.SetHostLocation(ctx, hostID, locID); err != nil {
			s.logger.Warn("topology: failed to set location",
				"host_id", hostID, "location_id", locID, "error", err)
		} else {
			s.logger.Info("topology: location set", "host_id", hostID, "location_id", locID)
		}
	}
}

// resolveStorageDomain resolves a name or UUID string to a storage domain UUID.
func (s *GRPCServer) resolveStorageDomain(ctx context.Context, nameOrID string) (uuid.UUID, error) {
	if id, err := uuid.Parse(nameOrID); err == nil {
		if _, err := s.topologySvc.GetStorageDomain(ctx, id); err != nil {
			return uuid.Nil, err
		}
		return id, nil
	}
	domains, err := s.topologySvc.ListStorageDomains(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var matched uuid.UUID
	count := 0
	for _, d := range domains {
		if d.Name == nameOrID {
			matched = d.ID
			count++
		}
	}
	switch count {
	case 0:
		return uuid.Nil, topology.ErrNotFound
	case 1:
		return matched, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple storage domains named %q, use UUID", nameOrID)
	}
}

// resolveLocation resolves a name or UUID string to a location UUID.
// Locations can have duplicate names under different parents, so multiple
// matches are treated as an error.
func (s *GRPCServer) resolveLocation(ctx context.Context, nameOrID string) (uuid.UUID, error) {
	if id, err := uuid.Parse(nameOrID); err == nil {
		if _, err := s.topologySvc.GetLocation(ctx, id); err != nil {
			return uuid.Nil, err
		}
		return id, nil
	}
	locations, err := s.topologySvc.ListLocations(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var matched uuid.UUID
	count := 0
	for _, l := range locations {
		if l.Name == nameOrID {
			matched = l.ID
			count++
		}
	}
	switch count {
	case 0:
		return uuid.Nil, topology.ErrNotFound
	case 1:
		return matched, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple locations named %q, use UUID", nameOrID)
	}
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
