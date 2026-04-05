package scheduler_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/scheduler"
	"github.com/tjst-t/cirrus/internal/storage"
	"github.com/tjst-t/cirrus/internal/topology"
)

// --- fakes ---

type fakeHostSvc struct {
	hosts       map[uuid.UUID]*host.Host
	allocatable map[uuid.UUID]*host.AllocatableResources
}

func (f *fakeHostSvc) RegisterOrGet(ctx context.Context, name, address, workerGRPCAddr, fabricIP, capability string) (*host.Host, bool, error) {
	return nil, false, nil
}
func (f *fakeHostSvc) GetHost(ctx context.Context, id uuid.UUID) (*host.Host, error) {
	if h, ok := f.hosts[id]; ok {
		return h, nil
	}
	return nil, host.ErrNotFound
}
func (f *fakeHostSvc) ListHosts(ctx context.Context) ([]host.Host, error) {
	var out []host.Host
	for _, h := range f.hosts {
		out = append(out, *h)
	}
	return out, nil
}
func (f *fakeHostSvc) ListHostsByState(ctx context.Context, state host.OperationalState) ([]host.Host, error) {
	return nil, nil
}
func (f *fakeHostSvc) DeleteHost(ctx context.Context, id uuid.UUID) error { return nil }
func (f *fakeHostSvc) UpdateCapability(ctx context.Context, id uuid.UUID, capability []byte) error {
	return nil
}
func (f *fakeHostSvc) UpdateResourcePhysical(ctx context.Context, id uuid.UUID, resources []byte) error {
	return nil
}
func (f *fakeHostSvc) UpdateOvercommitRatios(ctx context.Context, id uuid.UUID, ratios []byte) error {
	return nil
}
func (f *fakeHostSvc) SetOperationalState(ctx context.Context, id uuid.UUID, state host.OperationalState) error {
	return nil
}
func (f *fakeHostSvc) Heartbeat(ctx context.Context, id string, report host.ResourceReport) error {
	return nil
}
func (f *fakeHostSvc) GetAllocatable(ctx context.Context, id uuid.UUID) (*host.AllocatableResources, error) {
	if a, ok := f.allocatable[id]; ok {
		return a, nil
	}
	return &host.AllocatableResources{}, nil
}
func (f *fakeHostSvc) ListHostsPage(_ context.Context, _ time.Time, _ uuid.UUID, _ int) ([]host.Host, error) {
	return nil, nil
}

type fakeStorageSvc struct {
	backendsForAZ    []storage.Backend
	backendsFromHost []storage.Backend
}

func (f *fakeStorageSvc) ListBackendsForAZ(ctx context.Context, azID uuid.UUID) ([]storage.Backend, error) {
	return f.backendsForAZ, nil
}
func (f *fakeStorageSvc) ListBackendsReachableFromHost(ctx context.Context, hostID uuid.UUID) ([]storage.Backend, error) {
	return f.backendsFromHost, nil
}
func (f *fakeStorageSvc) RegisterBackend(ctx context.Context, spec storage.RegisterBackendSpec) (*storage.Backend, error) {
	return nil, nil
}
func (f *fakeStorageSvc) GetBackend(ctx context.Context, id uuid.UUID) (*storage.Backend, error) {
	return nil, nil
}
func (f *fakeStorageSvc) ListBackends(ctx context.Context) ([]storage.Backend, error) { return nil, nil }
func (f *fakeStorageSvc) DrainBackend(ctx context.Context, id uuid.UUID) error        { return nil }
func (f *fakeStorageSvc) CreateVolumeType(ctx context.Context, name, description string, requiredCaps []string, qosPolicy map[string]any, isPublic bool) (*storage.VolumeType, error) {
	return nil, nil
}
func (f *fakeStorageSvc) GetVolumeType(ctx context.Context, id uuid.UUID) (*storage.VolumeType, error) {
	return nil, nil
}
func (f *fakeStorageSvc) ListVolumeTypes(ctx context.Context) ([]storage.VolumeType, error) {
	return nil, nil
}
func (f *fakeStorageSvc) CreateVolume(ctx context.Context, spec storage.CreateVolumeSpec) (*storage.Volume, error) {
	return nil, nil
}
func (f *fakeStorageSvc) GetVolume(ctx context.Context, tenantID, volumeID uuid.UUID) (*storage.Volume, error) {
	return nil, nil
}
func (f *fakeStorageSvc) ListVolumes(ctx context.Context, tenantID uuid.UUID) ([]storage.Volume, error) {
	return nil, nil
}
func (f *fakeStorageSvc) DeleteVolume(ctx context.Context, tenantID, volumeID uuid.UUID) error {
	return nil
}
func (f *fakeStorageSvc) ResizeVolume(ctx context.Context, tenantID, volumeID uuid.UUID, newSizeGB int64) (*storage.Volume, error) {
	return nil, nil
}
func (f *fakeStorageSvc) ExportVolume(ctx context.Context, volumeID, hostID uuid.UUID) (*storage.ExportInfo, error) {
	return nil, nil
}
func (f *fakeStorageSvc) UnexportVolume(ctx context.Context, volumeID uuid.UUID) error { return nil }
func (f *fakeStorageSvc) ListVolumesOnBackend(ctx context.Context, backendID uuid.UUID) ([]storage.Volume, error) {
	return nil, nil
}
func (f *fakeStorageSvc) ListVolumesPage(_ context.Context, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]storage.Volume, error) {
	return nil, nil
}

type fakeTopologySvc struct {
	reachableHosts []uuid.UUID
	storageDomains []topology.StorageDomain
}

func (f *fakeTopologySvc) CreateStorageDomain(ctx context.Context, name string) (*topology.StorageDomain, error) {
	return nil, nil
}
func (f *fakeTopologySvc) GetStorageDomain(ctx context.Context, id uuid.UUID) (*topology.StorageDomain, error) {
	return nil, nil
}
func (f *fakeTopologySvc) ListStorageDomains(ctx context.Context) ([]topology.StorageDomain, error) {
	return f.storageDomains, nil
}
func (f *fakeTopologySvc) CreateLocation(ctx context.Context, parentID *uuid.UUID, name string, locType topology.LocationType, faultAttrs []byte) (*topology.Location, error) {
	return nil, nil
}
func (f *fakeTopologySvc) GetLocation(ctx context.Context, id uuid.UUID) (*topology.Location, error) {
	return nil, nil
}
func (f *fakeTopologySvc) ListLocations(ctx context.Context) ([]topology.Location, error) {
	return nil, nil
}
func (f *fakeTopologySvc) GetLocationPath(ctx context.Context, id uuid.UUID) ([]topology.Location, error) {
	return nil, nil
}
func (f *fakeTopologySvc) GetLocationTree(ctx context.Context, id uuid.UUID) (*topology.Location, error) {
	return nil, nil
}
func (f *fakeTopologySvc) AssociateHostStorageDomain(ctx context.Context, hostID, sdID uuid.UUID) error {
	return nil
}
func (f *fakeTopologySvc) DissociateHostStorageDomain(ctx context.Context, hostID, sdID uuid.UUID) error {
	return nil
}
func (f *fakeTopologySvc) SetHostLocation(ctx context.Context, hostID, locationID uuid.UUID) error {
	return nil
}
func (f *fakeTopologySvc) GetComputePool(ctx context.Context, sdID uuid.UUID) (*topology.ComputePool, error) {
	return nil, nil
}
func (f *fakeTopologySvc) GetFaultDomains(ctx context.Context, level topology.LocationType) ([]topology.FaultDomain, error) {
	return nil, nil
}
func (f *fakeTopologySvc) ListReachableHosts(ctx context.Context, sdID uuid.UUID) ([]uuid.UUID, error) {
	return f.reachableHosts, nil
}
func (f *fakeTopologySvc) ListReachableBackends(ctx context.Context, hostID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

// --- helpers ---

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// --- tests ---

func TestSchedule_PicksBestHost(t *testing.T) {
	azID := uuid.New()
	sdID := uuid.New()
	host1ID := uuid.New()
	host2ID := uuid.New()

	sd := topology.StorageDomain{ID: sdID, Name: "sd1"}
	backendID := uuid.New()
	backend := storage.Backend{
		ID:              backendID,
		StorageDomainID: sdID,
		Name:            "b1",
		State:           storage.BackendStateActive,
		TotalCapacityGB: 100,
		Capabilities:    mustJSON([]string{}),
	}

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: {
				ID: host1ID, OperationalState: host.StateActive,
				ResourcePhysical: mustJSON(host.PhysicalResources{Vcpus: 16, MemoryMB: 32768}),
			},
			host2ID: {
				ID: host2ID, OperationalState: host.StateActive,
				ResourcePhysical: mustJSON(host.PhysicalResources{Vcpus: 16, MemoryMB: 32768}),
			},
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			host1ID: {Vcpus: 14, MemoryMB: 28000}, // more free → higher score
			host2ID: {Vcpus: 4, MemoryMB: 8000},
		},
	}
	storageSvc := &fakeStorageSvc{
		backendsForAZ:    []storage.Backend{backend},
		backendsFromHost: []storage.Backend{backend},
	}
	topologySvc := &fakeTopologySvc{
		reachableHosts: []uuid.UUID{host1ID, host2ID},
		storageDomains: []topology.StorageDomain{sd},
	}

	sched := scheduler.New(hostSvc, storageSvc, topologySvc)

	vtID := uuid.New()
	f := &flavor.Flavor{VCPUs: 2, RAMMB: 4096}
	result, err := sched.Schedule(context.Background(), scheduler.ScheduleSpec{
		AZID:         azID,
		Flavor:       f,
		VolumeTypeID: &vtID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HostID != host1ID {
		t.Errorf("expected host1 (more free resources), got %s", result.HostID)
	}
	if result.BackendID == nil || *result.BackendID != backendID {
		t.Errorf("expected backend %s, got %v", backendID, result.BackendID)
	}
}

func TestSchedule_InsufficientResources(t *testing.T) {
	azID := uuid.New()
	sdID := uuid.New()
	hostID := uuid.New()
	sd := topology.StorageDomain{ID: sdID, Name: "sd1"}
	backend := storage.Backend{
		ID: uuid.New(), StorageDomainID: sdID,
		State: storage.BackendStateActive, TotalCapacityGB: 100,
		Capabilities: mustJSON([]string{}),
	}
	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			hostID: {ID: hostID, OperationalState: host.StateActive,
				ResourcePhysical: mustJSON(host.PhysicalResources{Vcpus: 2, MemoryMB: 4096})},
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			hostID: {Vcpus: 0, MemoryMB: 0}, // fully used
		},
	}
	storageSvc := &fakeStorageSvc{
		backendsForAZ:    []storage.Backend{backend},
		backendsFromHost: []storage.Backend{backend},
	}
	topologySvc := &fakeTopologySvc{
		reachableHosts: []uuid.UUID{hostID},
		storageDomains: []topology.StorageDomain{sd},
	}

	sched := scheduler.New(hostSvc, storageSvc, topologySvc)
	_, err := sched.Schedule(context.Background(), scheduler.ScheduleSpec{
		AZID:   azID,
		Flavor: &flavor.Flavor{VCPUs: 2, RAMMB: 4096},
	})
	if err == nil {
		t.Fatal("expected error for insufficient resources, got nil")
	}
}
