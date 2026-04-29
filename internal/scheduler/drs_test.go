package scheduler_test

// drs_test.go — unit tests for the DRS engine (internal/scheduler/drs.go).
//
// All tests use in-memory fakes; no database or external services are required.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/scheduler"
)

// --- fakes for DRS tests ---

// fakeDRSHostSvc implements host.Service (subset used by DRS engine).
type fakeDRSHostSvc struct {
	hosts       map[uuid.UUID]*host.Host
	allocatable map[uuid.UUID]*host.AllocatableResources
}

// Satisfy the full host.Service interface via fakeHostSvc (already in scheduler_test.go).
// We embed fakeHostSvc so we only need to override the methods DRS uses.

// fakeDRSAZSvc implements DRSAZService.
type fakeDRSAZSvc struct {
	zones []az.AvailabilityZone
}

func (f *fakeDRSAZSvc) ListEnabled(_ context.Context) ([]az.AvailabilityZone, error) {
	return f.zones, nil
}

// fakeDRSComputeSvc implements DRSComputeService.
type fakeDRSComputeSvc struct {
	// vmsByHost maps hostID → list of VMs on that host.
	vmsByHost map[uuid.UUID][]scheduler.DRSVM
}

func (f *fakeDRSComputeSvc) ListVMsByHost(_ context.Context, hostID uuid.UUID) ([]scheduler.DRSVM, error) {
	return f.vmsByHost[hostID], nil
}

// fakeDRSFlavorSvc implements DRSFlavorService.
type fakeDRSFlavorSvc struct {
	flavors map[uuid.UUID]*flavor.Flavor
}

func (f *fakeDRSFlavorSvc) Get(_ context.Context, id uuid.UUID) (*flavor.Flavor, error) {
	if flv, ok := f.flavors[id]; ok {
		return flv, nil
	}
	return nil, flavor.ErrNotFound
}

// fakeDRSScheduler implements Scheduler.Reschedule and CandidateHostsForAZ.
// It always returns the pre-configured destHostID for Reschedule.
// CandidateHostsForAZ returns candidateIDs when set, or nil (triggers fallback
// to all hosts in the engine).
type fakeDRSScheduler struct {
	destHostID   uuid.UUID
	err          error
	candidateIDs map[uuid.UUID][]uuid.UUID // azID → host IDs; nil means "use all"
}

func (f *fakeDRSScheduler) Schedule(_ context.Context, _ scheduler.ScheduleSpec) (*scheduler.ScheduleResult, error) {
	return nil, nil
}

func (f *fakeDRSScheduler) Reschedule(_ context.Context, _ scheduler.RescheduleSpec) (*scheduler.ScheduleResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &scheduler.ScheduleResult{HostID: f.destHostID}, nil
}

func (f *fakeDRSScheduler) CandidateHostsForAZ(_ context.Context, azID uuid.UUID) ([]uuid.UUID, error) {
	if f.candidateIDs == nil {
		return nil, nil // triggers fallback to all hosts
	}
	return f.candidateIDs[azID], nil
}

// --- helper: build a host.Host with ResourcePhysical set ---

func makeHost(id uuid.UUID, state host.OperationalState, vcpus int, memMB int64) *host.Host {
	phys, _ := json.Marshal(host.PhysicalResources{Vcpus: vcpus, MemoryMB: memMB})
	return &host.Host{
		ID:               id,
		OperationalState: state,
		ResourcePhysical: phys,
	}
}

// --- TestDRSStddevComputation verifies the stddev helper indirectly via engine output ---

func TestDRS_StddevBelowThreshold_NoPlan(t *testing.T) {
	azID := uuid.New()
	host1ID := uuid.New()
	host2ID := uuid.New()

	// Both hosts have similar free fractions → stddev will be 0 → no plan.
	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: makeHost(host1ID, host.StateActive, 16, 32768),
			host2ID: makeHost(host2ID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			host1ID: {Vcpus: 8, MemoryMB: 16384, PhysicalKnown: true},
			host2ID: {Vcpus: 8, MemoryMB: 16384, PhysicalKnown: true},
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Name: "az1", Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{vmsByHost: map[uuid.UUID][]scheduler.DRSVM{}}
	flavorSvc := &fakeDRSFlavorSvc{flavors: map[uuid.UUID]*flavor.Flavor{}}
	sched := &fakeDRSScheduler{}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)
	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.15,
		MaxConcurrent:   2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 AZ result, got %d", len(results))
	}
	if len(results[0].PlannedMoves) != 0 {
		t.Errorf("expected 0 planned moves when balanced, got %d", len(results[0].PlannedMoves))
	}
}

func TestDRS_GreedyPicksMostLoaded(t *testing.T) {
	azID := uuid.New()
	host1ID := uuid.New() // heavily loaded: 14/16 vCPUs used → free frac low
	host2ID := uuid.New() // lightly loaded: 2/16 vCPUs used → free frac high

	flavorID := uuid.New()
	flv := &flavor.Flavor{ID: flavorID, VCPUs: 2, RAMMB: 2048}

	vmID := uuid.New()
	tenantID := uuid.New()

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: makeHost(host1ID, host.StateActive, 16, 32768),
			host2ID: makeHost(host2ID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			// host1: only 2 vCPUs free out of 16 → free fraction ~0.06
			host1ID: {Vcpus: 2, MemoryMB: 4096, PhysicalKnown: true},
			// host2: 14 vCPUs free out of 16 → free fraction ~0.87
			host2ID: {Vcpus: 14, MemoryMB: 28672, PhysicalKnown: true},
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Name: "az1", Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{
		vmsByHost: map[uuid.UUID][]scheduler.DRSVM{
			host1ID: {
				{
					ID:       vmID,
					TenantID: tenantID,
					HostID:   &host1ID,
					FlavorID: &flavorID,
					Status:   "running",
				},
			},
		},
	}
	flavorSvc := &fakeDRSFlavorSvc{
		flavors: map[uuid.UUID]*flavor.Flavor{flavorID: flv},
	}
	sched := &fakeDRSScheduler{destHostID: host2ID}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)
	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.05, // low threshold so imbalance is detected
		MaxConcurrent:   2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 AZ result, got %d", len(results))
	}
	moves := results[0].PlannedMoves
	if len(moves) == 0 {
		t.Fatal("expected at least 1 planned move, got 0")
	}
	if moves[0].SrcHostID != host1ID {
		t.Errorf("expected src to be most-loaded host1 (%s), got %s", host1ID, moves[0].SrcHostID)
	}
	if moves[0].DestHostID != host2ID {
		t.Errorf("expected dest to be least-loaded host2 (%s), got %s", host2ID, moves[0].DestHostID)
	}
	if moves[0].VMID != vmID {
		t.Errorf("expected VM %s to be moved, got %s", vmID, moves[0].VMID)
	}
}

func TestDRS_RespectsMaxConcurrentCap(t *testing.T) {
	azID := uuid.New()
	host1ID := uuid.New()
	host2ID := uuid.New()

	flavorID := uuid.New()
	flv := &flavor.Flavor{ID: flavorID, VCPUs: 1, RAMMB: 512}

	tenantID := uuid.New()
	// Put 5 VMs on host1 to give the engine plenty to choose from.
	var vmsOnHost1 []scheduler.DRSVM
	for i := 0; i < 5; i++ {
		vmID := uuid.New()
		h := host1ID
		vmsOnHost1 = append(vmsOnHost1, scheduler.DRSVM{
			ID:       vmID,
			TenantID: tenantID,
			HostID:   &h,
			FlavorID: &flavorID,
			Status:   "running",
		})
	}

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: makeHost(host1ID, host.StateActive, 16, 32768),
			host2ID: makeHost(host2ID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			host1ID: {Vcpus: 1, MemoryMB: 2048, PhysicalKnown: true},  // very loaded
			host2ID: {Vcpus: 15, MemoryMB: 30720, PhysicalKnown: true}, // mostly free
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Name: "az1", Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{
		vmsByHost: map[uuid.UUID][]scheduler.DRSVM{
			host1ID: vmsOnHost1,
		},
	}
	flavorSvc := &fakeDRSFlavorSvc{
		flavors: map[uuid.UUID]*flavor.Flavor{flavorID: flv},
	}
	sched := &fakeDRSScheduler{destHostID: host2ID}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)
	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.01, // very tight threshold: will keep planning until cap
		MaxConcurrent:   2,    // cap at 2
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	totalMoves := 0
	for _, r := range results {
		totalMoves += len(r.PlannedMoves)
	}
	if totalMoves > 2 {
		t.Errorf("MaxConcurrent=2 but got %d planned moves", totalMoves)
	}
}

func TestDRS_SkipsHostsWithPhysicalUnknown(t *testing.T) {
	azID := uuid.New()
	host1ID := uuid.New() // physical unknown
	host2ID := uuid.New() // physical unknown

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: makeHost(host1ID, host.StateActive, 0, 0), // 0 → physical unknown
			host2ID: makeHost(host2ID, host.StateActive, 0, 0),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			host1ID: {Vcpus: 0, MemoryMB: 0, PhysicalKnown: false},
			host2ID: {Vcpus: 0, MemoryMB: 0, PhysicalKnown: false},
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Name: "az1", Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{vmsByHost: map[uuid.UUID][]scheduler.DRSVM{}}
	flavorSvc := &fakeDRSFlavorSvc{flavors: map[uuid.UUID]*flavor.Flavor{}}
	sched := &fakeDRSScheduler{}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)
	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.15,
		MaxConcurrent:   2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if r.EvaluatedHosts != 0 {
			t.Errorf("expected 0 evaluated hosts (all physical unknown), got %d", r.EvaluatedHosts)
		}
		if len(r.PlannedMoves) != 0 {
			t.Errorf("expected 0 moves when all hosts have !PhysicalKnown, got %d", len(r.PlannedMoves))
		}
	}
}

func TestDRS_StddevBefore_Populated(t *testing.T) {
	// Verify that StddevBefore in the result is set even when no plan is generated.
	azID := uuid.New()
	host1ID := uuid.New()
	host2ID := uuid.New()

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: makeHost(host1ID, host.StateActive, 16, 32768),
			host2ID: makeHost(host2ID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			host1ID: {Vcpus: 8, MemoryMB: 16384, PhysicalKnown: true},
			host2ID: {Vcpus: 8, MemoryMB: 16384, PhysicalKnown: true},
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Name: "az1", Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{vmsByHost: map[uuid.UUID][]scheduler.DRSVM{}}
	flavorSvc := &fakeDRSFlavorSvc{flavors: map[uuid.UUID]*flavor.Flavor{}}
	sched := &fakeDRSScheduler{}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)
	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.15,
		MaxConcurrent:   2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result")
	}
	// Both hosts at 50% free → stddev = 0
	if results[0].StddevBefore != 0 {
		t.Errorf("expected stddev=0 for balanced hosts, got %f", results[0].StddevBefore)
	}
}

func TestDRS_NoMoveForNonRunningVMs(t *testing.T) {
	azID := uuid.New()
	host1ID := uuid.New()
	host2ID := uuid.New()

	flavorID := uuid.New()
	flv := &flavor.Flavor{ID: flavorID, VCPUs: 2, RAMMB: 2048}

	tenantID := uuid.New()
	vmID := uuid.New()

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: makeHost(host1ID, host.StateActive, 16, 32768),
			host2ID: makeHost(host2ID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			host1ID: {Vcpus: 2, MemoryMB: 4096, PhysicalKnown: true},  // loaded
			host2ID: {Vcpus: 14, MemoryMB: 28672, PhysicalKnown: true}, // free
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{
		vmsByHost: map[uuid.UUID][]scheduler.DRSVM{
			host1ID: {
				{
					ID:       vmID,
					TenantID: tenantID,
					HostID:   &host1ID,
					FlavorID: &flavorID,
					Status:   "stopped", // NOT running
				},
			},
		},
	}
	flavorSvc := &fakeDRSFlavorSvc{
		flavors: map[uuid.UUID]*flavor.Flavor{flavorID: flv},
	}
	sched := &fakeDRSScheduler{destHostID: host2ID}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)
	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.01,
		MaxConcurrent:   2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if len(r.PlannedMoves) != 0 {
			t.Errorf("expected no moves for stopped VMs, got %d", len(r.PlannedMoves))
		}
	}
}

// TestDRS_GreedyDoesNotPickSameVMTwice ensures the greedy loop never adds the
// same VM to the plan more than once, even when MaxConcurrent > available VMs.
func TestDRS_GreedyDoesNotPickSameVMTwice(t *testing.T) {
	azID := uuid.New()
	host1ID := uuid.New() // very loaded
	host2ID := uuid.New() // empty

	flavorID := uuid.New()
	flv := &flavor.Flavor{ID: flavorID, VCPUs: 2, RAMMB: 2048}
	tenantID := uuid.New()
	vmID := uuid.New() // only ONE VM available

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			host1ID: makeHost(host1ID, host.StateActive, 16, 32768),
			host2ID: makeHost(host2ID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			host1ID: {Vcpus: 1, MemoryMB: 1024, PhysicalKnown: true},  // very loaded
			host2ID: {Vcpus: 15, MemoryMB: 31744, PhysicalKnown: true}, // mostly free
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Name: "az1", Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{
		vmsByHost: map[uuid.UUID][]scheduler.DRSVM{
			host1ID: {
				{
					ID:       vmID,
					TenantID: tenantID,
					HostID:   &host1ID,
					FlavorID: &flavorID,
					Status:   "running",
				},
			},
		},
	}
	flavorSvc := &fakeDRSFlavorSvc{
		flavors: map[uuid.UUID]*flavor.Flavor{flavorID: flv},
	}
	sched := &fakeDRSScheduler{destHostID: host2ID}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)
	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.01, // very tight: ensures greedy would loop if not capped
		MaxConcurrent:   2,    // budget allows 2 moves, but only 1 VM exists
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 AZ result, got %d", len(results))
	}
	moves := results[0].PlannedMoves
	if len(moves) != 1 {
		t.Errorf("expected exactly 1 planned move (only 1 VM available), got %d", len(moves))
	}
	if len(moves) > 0 && moves[0].VMID != vmID {
		t.Errorf("expected VM %s to be moved, got %s", vmID, moves[0].VMID)
	}
}

// TestDRS_TwoAZsDoNotCrosspollinate verifies that VMs in AZ-A are never
// proposed to be migrated to hosts in AZ-B, and vice-versa.
func TestDRS_TwoAZsDoNotCrosspollinate(t *testing.T) {
	azAID := uuid.New()
	azBID := uuid.New()
	hostAID := uuid.New() // AZ-A, heavily loaded
	hostBID := uuid.New() // AZ-A, empty
	hostCID := uuid.New() // AZ-B, heavily loaded
	hostDID := uuid.New() // AZ-B, empty

	flavorID := uuid.New()
	flv := &flavor.Flavor{ID: flavorID, VCPUs: 2, RAMMB: 2048}
	tenantID := uuid.New()
	vmAID := uuid.New() // VM on hostA (AZ-A)
	vmCID := uuid.New() // VM on hostC (AZ-B)

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			hostAID: makeHost(hostAID, host.StateActive, 16, 32768),
			hostBID: makeHost(hostBID, host.StateActive, 16, 32768),
			hostCID: makeHost(hostCID, host.StateActive, 16, 32768),
			hostDID: makeHost(hostDID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			hostAID: {Vcpus: 2, MemoryMB: 4096, PhysicalKnown: true},  // loaded
			hostBID: {Vcpus: 14, MemoryMB: 28672, PhysicalKnown: true}, // free
			hostCID: {Vcpus: 2, MemoryMB: 4096, PhysicalKnown: true},  // loaded
			hostDID: {Vcpus: 14, MemoryMB: 28672, PhysicalKnown: true}, // free
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{
			{ID: azAID, Name: "az-a", Enabled: true},
			{ID: azBID, Name: "az-b", Enabled: true},
		},
	}
	computeSvc := &fakeDRSComputeSvc{
		vmsByHost: map[uuid.UUID][]scheduler.DRSVM{
			hostAID: {
				{ID: vmAID, TenantID: tenantID, HostID: &hostAID, FlavorID: &flavorID, Status: "running"},
			},
			hostCID: {
				{ID: vmCID, TenantID: tenantID, HostID: &hostCID, FlavorID: &flavorID, Status: "running"},
			},
		},
	}
	flavorSvc := &fakeDRSFlavorSvc{
		flavors: map[uuid.UUID]*flavor.Flavor{flavorID: flv},
	}

	// The scheduler returns the per-AZ candidate lists explicitly.
	// AZ-A contains only hostA and hostB; AZ-B contains only hostC and hostD.
	// Reschedule always returns hostBID (for AZ-A calls) or hostDID (for AZ-B calls).
	// We configure destHostID=hostBID; for AZ-B Reschedule calls the engine will
	// check that the returned dest is in the AZ's states map.  We use two separate
	// engine instances to keep the test simple, or configure candidateIDs properly.
	schedA := &fakeDRSScheduler{
		destHostID: hostBID,
		candidateIDs: map[uuid.UUID][]uuid.UUID{
			azAID: {hostAID, hostBID},
			azBID: {hostCID, hostDID},
		},
	}

	// Use a single scheduler for both AZs, but Reschedule will return hostBID for
	// AZ-A moves and hostDID for AZ-B moves. Since fakeDRSScheduler returns a fixed
	// destHostID, we split into two engine calls or verify via dest filtering.
	// For simplicity: use destHostID=hostBID; for AZ-B the dest (hostBID) will not be
	// in states (only hostCID/hostDID are), so pickBestMove will skip it.
	// This correctly validates the AZ isolation via candidateHosts scoping.
	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, schedA)

	results, err := eng.Plan(context.Background(), scheduler.DRSPolicy{
		StddevThreshold: 0.05,
		MaxConcurrent:   4,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Gather all src/dest host IDs across both AZ plans.
	azAHosts := map[uuid.UUID]bool{hostAID: true, hostBID: true}
	azBHosts := map[uuid.UUID]bool{hostCID: true, hostDID: true}

	for _, r := range results {
		for _, m := range r.PlannedMoves {
			if r.AZID == azAID {
				// Moves in AZ-A must stay within AZ-A hosts.
				if !azAHosts[m.SrcHostID] {
					t.Errorf("AZ-A move: src %s is not an AZ-A host", m.SrcHostID)
				}
				if !azAHosts[m.DestHostID] {
					t.Errorf("AZ-A move: dest %s is not an AZ-A host (cross-AZ leak!)", m.DestHostID)
				}
			}
			if r.AZID == azBID {
				// Moves in AZ-B must stay within AZ-B hosts.
				if !azBHosts[m.SrcHostID] {
					t.Errorf("AZ-B move: src %s is not an AZ-B host", m.SrcHostID)
				}
				if !azBHosts[m.DestHostID] {
					t.Errorf("AZ-B move: dest %s is not an AZ-B host (cross-AZ leak!)", m.DestHostID)
				}
			}
		}
	}
}
