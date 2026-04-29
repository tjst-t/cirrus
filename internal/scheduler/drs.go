package scheduler

// drs.go — Distributed Resource Scheduler (DRS) engine.
//
// The Engine analyses each Availability Zone and proposes a set of VM migrations
// that will reduce the standard deviation of free-resource fractions across
// hosts in that AZ.  Callers (the controller DRS runner) are responsible for
// executing the returned MigrationPlans.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
)

// DRSPolicy configures the DRS algorithm.
type DRSPolicy struct {
	// StddevThreshold is the minimum standard deviation of free-fraction across
	// hosts in an AZ that will trigger rebalancing.  AZs below this threshold
	// are skipped.  Default 0.15.
	StddevThreshold float64

	// MaxConcurrent caps the number of MigrationPlans returned per Engine.Plan
	// call (across all AZs combined).
	MaxConcurrent int
}

// MigrationPlan describes a single DRS-recommended VM migration.
type MigrationPlan struct {
	VMID       uuid.UUID
	TenantID   uuid.UUID
	SrcHostID  uuid.UUID
	DestHostID uuid.UUID
	AZID       uuid.UUID
}

// DRSResult is the per-AZ output from Engine.Plan.
type DRSResult struct {
	AZID           uuid.UUID
	StddevBefore   float64
	PlannedMoves   []MigrationPlan
	EvaluatedHosts int
}

// DRSComputeService is the subset of compute.Service used by the DRS engine.
// Defined here (not in compute) to avoid an import cycle.
type DRSComputeService interface {
	// ListVMsByHost returns all VMs currently placed on the given host.
	ListVMsByHost(ctx context.Context, hostID uuid.UUID) ([]DRSVM, error)
}

// DRSVM is the VM projection the DRS engine needs.  The engine only reads
// flavor dimensions; it does not need network, storage or status fields.
type DRSVM struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	HostID   *uuid.UUID
	FlavorID *uuid.UUID
	Status   string
	Flavor   *flavor.Flavor // populated by the engine after resolving FlavorID
}

// DRSAZService is the subset of az.Service used by the DRS engine.
type DRSAZService interface {
	ListEnabled(ctx context.Context) ([]az.AvailabilityZone, error)
}

// DRSFlavorService is the subset of flavor.Service used by the DRS engine.
type DRSFlavorService interface {
	Get(ctx context.Context, id uuid.UUID) (*flavor.Flavor, error)
}

// Engine analyses load imbalance and proposes migration plans.
type Engine interface {
	// Plan inspects all enabled AZs and returns migration proposals.
	Plan(ctx context.Context, policy DRSPolicy) ([]DRSResult, error)
}

// drsHostState tracks current + projected resource fractions for a host.
type drsHostState struct {
	hostID    uuid.UUID
	h         *host.Host
	alloc     *host.AllocatableResources
	phys      host.PhysicalResources
	freeFrac  float64 // (freeVCPU/totalVCPU + freeRAM/totalRAM) / 2
}

// drsEngine is the default Engine implementation.
type drsEngine struct {
	hostSvc    host.Service
	computeSvc DRSComputeService
	azSvc      DRSAZService
	flavorSvc  DRSFlavorService
	scheduler  Scheduler
}

// NewEngine creates a DRS Engine wired to the supplied services.
func NewEngine(
	hostSvc host.Service,
	computeSvc DRSComputeService,
	azSvc DRSAZService,
	flavorSvc DRSFlavorService,
	sched Scheduler,
) Engine {
	return &drsEngine{
		hostSvc:    hostSvc,
		computeSvc: computeSvc,
		azSvc:      azSvc,
		flavorSvc:  flavorSvc,
		scheduler:  sched,
	}
}

// Plan implements Engine.
func (e *drsEngine) Plan(ctx context.Context, policy DRSPolicy) ([]DRSResult, error) {
	azList, err := e.azSvc.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("drs: list enabled AZs: %w", err)
	}

	var results []DRSResult
	totalPlanned := 0

	for _, zone := range azList {
		if totalPlanned >= policy.MaxConcurrent {
			break
		}

		result, err := e.planAZ(ctx, zone, policy, policy.MaxConcurrent-totalPlanned)
		if err != nil {
			return nil, fmt.Errorf("drs: plan AZ %s: %w", zone.ID, err)
		}
		results = append(results, result)
		totalPlanned += len(result.PlannedMoves)
	}

	return results, nil
}

// planAZ runs the greedy algorithm for one AZ.
func (e *drsEngine) planAZ(ctx context.Context, zone az.AvailabilityZone, policy DRSPolicy, budget int) (DRSResult, error) {
	result := DRSResult{AZID: zone.ID}

	// List active hosts in this AZ.
	allHosts, err := e.hostSvc.ListHosts(ctx)
	if err != nil {
		return result, fmt.Errorf("list hosts: %w", err)
	}

	// Build per-host state: only active hosts whose physical resources are known.
	states := make(map[uuid.UUID]*drsHostState)
	for i := range allHosts {
		h := &allHosts[i]
		if h.OperationalState != host.StateActive {
			continue
		}

		alloc, err := e.hostSvc.GetAllocatable(ctx, h.ID)
		if err != nil {
			continue
		}
		if !alloc.PhysicalKnown {
			continue
		}

		phys := parsePhysicalForDRS(h.ResourcePhysical)
		if phys.Vcpus == 0 || phys.MemoryMB == 0 {
			continue
		}

		// Copy AllocatableResources so the greedy loop can mutate the copy
		// for projection without side-effects on the service state.
		allocCopy := *alloc
		states[h.ID] = &drsHostState{
			hostID:   h.ID,
			h:        h,
			alloc:    &allocCopy,
			phys:     phys,
			freeFrac: drsFreeFraction(allocCopy.Vcpus, float64(phys.Vcpus), allocCopy.MemoryMB, float64(phys.MemoryMB)),
		}
	}

	result.EvaluatedHosts = len(states)
	if len(states) < 2 {
		// Need at least 2 hosts to balance.
		return result, nil
	}

	// Compute initial stddev.
	stddev := computeStddev(states)
	result.StddevBefore = stddev

	if stddev <= policy.StddevThreshold {
		// Already balanced.
		return result, nil
	}

	// Greedy rebalancing loop.
	for len(result.PlannedMoves) < budget {
		// Pick the most-loaded (lowest free-frac) and least-loaded hosts.
		mostLoaded, leastLoaded := findExtremes(states)
		if mostLoaded == nil || leastLoaded == nil || mostLoaded.hostID == leastLoaded.hostID {
			break
		}

		// Find a running VM on the most-loaded host to move.
		vms, err := e.computeSvc.ListVMsByHost(ctx, mostLoaded.hostID)
		if err != nil {
			return result, fmt.Errorf("list vms by host %s: %w", mostLoaded.hostID, err)
		}

		// Resolve flavors.
		resolvedVMs := e.resolveVMFlavors(ctx, vms)

		// Pick the VM whose move would most reduce stddev.
		bestVM, destHostID, err := e.pickBestMove(ctx, resolvedVMs, mostLoaded, leastLoaded, states, zone.ID)
		if err != nil || bestVM == nil {
			break
		}

		plan := MigrationPlan{
			VMID:       bestVM.ID,
			TenantID:   bestVM.TenantID,
			SrcHostID:  mostLoaded.hostID,
			DestHostID: destHostID,
			AZID:       zone.ID,
		}
		result.PlannedMoves = append(result.PlannedMoves, plan)

		// Update projected fractions assuming this move happened.
		if flv := bestVM.Flavor; flv != nil {
			applyMove(states, mostLoaded.hostID, destHostID, flv)
		}

		// Recompute stddev.
		newStddev := computeStddev(states)
		if newStddev <= policy.StddevThreshold {
			break
		}
	}

	return result, nil
}

// pickBestMove finds the running VM on srcHost whose relocation to any valid
// destination most reduces load imbalance.  Returns the chosen VM and the
// destination host ID.
func (e *drsEngine) pickBestMove(
	ctx context.Context,
	vms []DRSVM,
	srcHost *drsHostState,
	_ *drsHostState, // leastLoaded (used via Reschedule)
	states map[uuid.UUID]*drsHostState,
	azID uuid.UUID,
) (*DRSVM, uuid.UUID, error) {
	var bestVM *DRSVM
	var bestDest uuid.UUID
	bestImprovement := -math.MaxFloat64

	for i := range vms {
		vm := &vms[i]
		if vm.Status != "running" {
			continue
		}
		if vm.Flavor == nil {
			continue
		}

		// Ask the scheduler for the best destination (excluding src host).
		res, err := e.scheduler.Reschedule(ctx, RescheduleSpec{
			ExcludeHostID: srcHost.hostID,
			AZID:          azID,
			Flavor:        vm.Flavor,
		})
		if err != nil {
			// No suitable destination for this VM — skip.
			continue
		}

		// Simulate the move and measure improvement.
		destID := res.HostID
		if _, ok := states[destID]; !ok {
			continue
		}

		simStates := copyStates(states)
		applyMove(simStates, srcHost.hostID, destID, vm.Flavor)
		improvement := computeStddev(states) - computeStddev(simStates)

		if improvement > bestImprovement {
			bestImprovement = improvement
			vm2 := *vm
			bestVM = &vm2
			bestDest = destID
		}
	}

	return bestVM, bestDest, nil
}

// resolveVMFlavors enriches each DRSVM with its Flavor (best-effort).
func (e *drsEngine) resolveVMFlavors(ctx context.Context, vms []DRSVM) []DRSVM {
	out := make([]DRSVM, 0, len(vms))
	for _, vm := range vms {
		if vm.FlavorID == nil {
			out = append(out, vm)
			continue
		}
		flv, err := e.flavorSvc.Get(ctx, *vm.FlavorID)
		if err == nil {
			vm.Flavor = flv
		}
		out = append(out, vm)
	}
	return out
}

// --- helpers ---

func drsFreeFraction(freeVCPUs, totalVCPUs, freeRAM, totalRAM float64) float64 {
	if totalVCPUs == 0 || totalRAM == 0 {
		return 0
	}
	vcpuFrac := freeVCPUs / totalVCPUs
	ramFrac := freeRAM / totalRAM
	return (vcpuFrac + ramFrac) / 2
}

func computeStddev(states map[uuid.UUID]*drsHostState) float64 {
	if len(states) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range states {
		sum += s.freeFrac
	}
	mean := sum / float64(len(states))

	variance := 0.0
	for _, s := range states {
		diff := s.freeFrac - mean
		variance += diff * diff
	}
	variance /= float64(len(states))
	return math.Sqrt(variance)
}

func findExtremes(states map[uuid.UUID]*drsHostState) (mostLoaded, leastLoaded *drsHostState) {
	for _, s := range states {
		if mostLoaded == nil || s.freeFrac < mostLoaded.freeFrac {
			mostLoaded = s
		}
		if leastLoaded == nil || s.freeFrac > leastLoaded.freeFrac {
			leastLoaded = s
		}
	}
	return mostLoaded, leastLoaded
}

func copyStates(states map[uuid.UUID]*drsHostState) map[uuid.UUID]*drsHostState {
	out := make(map[uuid.UUID]*drsHostState, len(states))
	for k, v := range states {
		cp := *v
		// Deep-copy the alloc pointer so that applyMove on the simulation
		// does not mutate the original states' AllocatableResources.
		allocCopy := *v.alloc
		cp.alloc = &allocCopy
		out[k] = &cp
	}
	return out
}

// applyMove adjusts projected free fractions as if a VM with the given flavor
// moved from srcHostID to destHostID.
func applyMove(states map[uuid.UUID]*drsHostState, srcID, destID uuid.UUID, flv *flavor.Flavor) {
	if flv == nil {
		return
	}
	src, okSrc := states[srcID]
	dst, okDst := states[destID]

	if okSrc {
		src.alloc.Vcpus += float64(flv.VCPUs)
		src.alloc.MemoryMB += float64(flv.RAMMB)
		src.freeFrac = drsFreeFraction(src.alloc.Vcpus, float64(src.phys.Vcpus), src.alloc.MemoryMB, float64(src.phys.MemoryMB))
	}
	if okDst {
		dst.alloc.Vcpus -= float64(flv.VCPUs)
		dst.alloc.MemoryMB -= float64(flv.RAMMB)
		dst.freeFrac = drsFreeFraction(dst.alloc.Vcpus, float64(dst.phys.Vcpus), dst.alloc.MemoryMB, float64(dst.phys.MemoryMB))
	}
}

func parsePhysicalForDRS(raw json.RawMessage) host.PhysicalResources {
	var p host.PhysicalResources
	_ = json.Unmarshal(raw, &p)
	return p
}
