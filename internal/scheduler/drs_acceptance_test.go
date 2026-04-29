package scheduler_test

// drs_acceptance_test.go — acceptance-level test for the DRS engine.
//
// TestAC_S025_1_DRS_RedistributesLoad verifies end-to-end behaviour:
//   - Several VMs are placed on host A, none on host B.
//   - engine.Plan detects imbalance and proposes a migration from A to B.
//   - The DRS runner executes the plan via a fake migrator.
//   - After the simulated move, the recomputed imbalance is below the threshold.
//
// This test uses in-memory fakes only — no real database or compute calls.

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/controller/drs"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/scheduler"
)

// TestAC_S025_1_DRS_RedistributesLoad is the acceptance test for S025-1.
// It validates that:
//  1. The engine identifies an imbalanced cluster (all VMs on host A, none on B).
//  2. The plan proposes moving a VM from A to B.
//  3. After executing the plan, the stddev of free fractions drops below threshold.
func TestAC_S025_1_DRS_RedistributesLoad(t *testing.T) {
	azID := uuid.New()
	hostAID := uuid.New() // heavily loaded
	hostBID := uuid.New() // empty

	flavorID := uuid.New()
	flv := &flavor.Flavor{ID: flavorID, VCPUs: 2, RAMMB: 2048}

	tenantID := uuid.New()

	// 4 VMs on host A.
	var vmsOnA []scheduler.DRSVM
	for i := 0; i < 4; i++ {
		vmID := uuid.New()
		h := hostAID
		vmsOnA = append(vmsOnA, scheduler.DRSVM{
			ID:       vmID,
			TenantID: tenantID,
			HostID:   &h,
			FlavorID: &flavorID,
			Status:   "running",
		})
	}

	// Host A: 8 of 16 vCPUs allocated (4 VMs × 2 vCPUs each) → 8 free / 16 = 0.5 free frac.
	// Host B: 0 allocated → 16 free / 16 = 1.0 free frac.
	// Mean = 0.75, stddev = 0.25 → above threshold 0.15.

	hostSvc := &fakeHostSvc{
		hosts: map[uuid.UUID]*host.Host{
			hostAID: makeHost(hostAID, host.StateActive, 16, 32768),
			hostBID: makeHost(hostBID, host.StateActive, 16, 32768),
		},
		allocatable: map[uuid.UUID]*host.AllocatableResources{
			hostAID: {Vcpus: 8, MemoryMB: 16384, PhysicalKnown: true},  // 50% free
			hostBID: {Vcpus: 16, MemoryMB: 32768, PhysicalKnown: true}, // 100% free
		},
	}
	azSvc := &fakeDRSAZSvc{
		zones: []az.AvailabilityZone{{ID: azID, Name: "az1", Enabled: true}},
	}
	computeSvc := &fakeDRSComputeSvc{
		vmsByHost: map[uuid.UUID][]scheduler.DRSVM{
			hostAID: vmsOnA,
		},
	}
	flavorSvc := &fakeDRSFlavorSvc{
		flavors: map[uuid.UUID]*flavor.Flavor{flavorID: flv},
	}
	sched := &fakeDRSScheduler{destHostID: hostBID}

	policy := scheduler.DRSPolicy{
		StddevThreshold: 0.15,
		MaxConcurrent:   2,
	}

	eng := scheduler.NewEngine(hostSvc, computeSvc, azSvc, flavorSvc, sched)

	// Step 1: Plan — verify at least one migration is proposed from A to B.
	results, err := eng.Plan(context.Background(), policy)
	if err != nil {
		t.Fatalf("engine.Plan error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one AZ result")
	}

	totalMoves := 0
	for _, r := range results {
		totalMoves += len(r.PlannedMoves)
	}
	if totalMoves == 0 {
		t.Fatal("expected at least one planned move, got 0")
	}

	for _, r := range results {
		for _, m := range r.PlannedMoves {
			if m.SrcHostID != hostAID {
				t.Errorf("expected src=hostA (%s), got %s", hostAID, m.SrcHostID)
			}
			if m.DestHostID != hostBID {
				t.Errorf("expected dest=hostB (%s), got %s", hostBID, m.DestHostID)
			}
		}
	}

	// Step 2: Execute plans via the runner with a fake migrator.
	migrator := newACFakeMigrator()
	runner := drs.NewRunner(eng, migrator, policy, time.Minute, nil)

	// Rebuild the engine plan from the cached results (simulate what runner does).
	report, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("runner.RunOnce error: %v", err)
	}
	if report.Failures > 0 {
		t.Errorf("expected 0 failures from runner, got %d: %v", report.Failures, report.Errors)
	}

	// Step 3: Recompute imbalance after moves.
	// Each move shifts 2 vCPUs + 2048 MB from A to B.
	// Number of moves executed = report.Successes.
	movesExecuted := report.Successes

	freeVCPUsA := 8.0 + float64(movesExecuted*flv.VCPUs)
	freeRAMBytesA := 16384.0 + float64(movesExecuted)*float64(flv.RAMMB)
	freeVCPUsB := 16.0 - float64(movesExecuted*flv.VCPUs)
	freeRAMBytesB := 32768.0 - float64(movesExecuted)*float64(flv.RAMMB)

	totalVCPUs := 16.0
	totalRAM := 32768.0
	fracA := (freeVCPUsA/totalVCPUs + freeRAMBytesA/totalRAM) / 2
	fracB := (freeVCPUsB/totalVCPUs + freeRAMBytesB/totalRAM) / 2
	mean := (fracA + fracB) / 2
	stddev := math.Sqrt(((fracA-mean)*(fracA-mean) + (fracB-mean)*(fracB-mean)) / 2)

	t.Logf("after %d moves: fracA=%.3f, fracB=%.3f, stddev=%.3f", movesExecuted, fracA, fracB, stddev)

	if stddev >= policy.StddevThreshold {
		t.Errorf("expected stddev %.3f < threshold %.3f after DRS run", stddev, policy.StddevThreshold)
	}
}

// --- acceptance test fakes (local to this file) ---

type acFakeMigrator struct {
	mu     sync.Mutex
	called []uuid.UUID
}

func newACFakeMigrator() *acFakeMigrator {
	return &acFakeMigrator{}
}

func (f *acFakeMigrator) MigrateVM(_ context.Context, _, vmID uuid.UUID, _ *uuid.UUID) error {
	f.mu.Lock()
	f.called = append(f.called, vmID)
	f.mu.Unlock()
	return nil
}
