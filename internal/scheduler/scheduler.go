// Package scheduler selects a (host, backend) pair for VM placement.
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/storage"
	"github.com/tjst-t/cirrus/internal/topology"
)

// ErrNoSuitableHost is returned when no host satisfies the placement constraints.
var ErrNoSuitableHost = errors.New("scheduler: no suitable host available")

// ErrNoSuitableBackend is returned when no backend satisfies the volume constraints.
var ErrNoSuitableBackend = errors.New("scheduler: no suitable storage backend available")

// ScheduleSpec describes the placement request.
type ScheduleSpec struct {
	// AZID constrains placement to hosts in the given availability zone.
	AZID uuid.UUID
	// Flavor defines the required vCPUs and RAM.
	Flavor *flavor.Flavor
	// VolumeTypeID, if set, requires a backend with matching capabilities.
	VolumeTypeID *uuid.UUID
	// RequiredCapabilities of the storage backend when VolumeTypeID is provided.
	RequiredBackendCapabilities []string
}

// ScheduleResult holds the selected host and optional backend.
type ScheduleResult struct {
	HostID    uuid.UUID
	BackendID *uuid.UUID // nil when no storage is requested
}

// Scheduler selects optimal placement for a VM.
type Scheduler interface {
	Schedule(ctx context.Context, spec ScheduleSpec) (*ScheduleResult, error)
}

// DefaultScheduler implements the Scheduler interface.
type DefaultScheduler struct {
	hostSvc     host.Service
	storageSvc  storage.Service
	topologySvc topology.Service
}

// New creates a new DefaultScheduler.
func New(hostSvc host.Service, storageSvc storage.Service, topologySvc topology.Service) *DefaultScheduler {
	return &DefaultScheduler{
		hostSvc:     hostSvc,
		storageSvc:  storageSvc,
		topologySvc: topologySvc,
	}
}

// Schedule picks the best (host, backend) pair for the given spec.
func (s *DefaultScheduler) Schedule(ctx context.Context, spec ScheduleSpec) (*ScheduleResult, error) {
	// 1. Get candidate hosts in the AZ via storage domain reachability.
	storageDomainIDs, err := s.storageDomainIDsForAZ(ctx, spec.AZID)
	if err != nil {
		return nil, fmt.Errorf("scheduler: resolve storage domains for az: %w", err)
	}

	hostIDSet := make(map[uuid.UUID]struct{})
	for _, sdID := range storageDomainIDs {
		ids, err := s.topologySvc.ListReachableHosts(ctx, sdID)
		if err != nil {
			return nil, fmt.Errorf("scheduler: list reachable hosts: %w", err)
		}
		for _, id := range ids {
			hostIDSet[id] = struct{}{}
		}
	}
	// When no storage topology is configured and no persistent volume is required,
	// fall back to all registered hosts (useful in dev/sim environments).
	if len(hostIDSet) == 0 && spec.VolumeTypeID == nil {
		allHosts, err := s.hostSvc.ListHosts(ctx)
		if err != nil {
			return nil, fmt.Errorf("scheduler: fallback list hosts: %w", err)
		}
		for _, h := range allHosts {
			hostIDSet[h.ID] = struct{}{}
		}
	}
	if len(hostIDSet) == 0 {
		return nil, ErrNoSuitableHost
	}

	// 2. Filter hosts by operational state and resource availability.
	type hostScore struct {
		id    uuid.UUID
		score float64 // higher is better
	}
	var candidates []hostScore

	for hostID := range hostIDSet {
		h, err := s.hostSvc.GetHost(ctx, hostID)
		if err != nil {
			continue
		}
		if h.OperationalState != host.StateActive {
			continue
		}
		// Check resource requirements when a flavor is specified.
		if spec.Flavor != nil {
			alloc, err := s.hostSvc.GetAllocatable(ctx, hostID)
			if err != nil {
				continue
			}
			// Skip resource check when physical resources are not yet reported
			// (e.g. sim workers that haven't sent a resource report yet).
			if alloc.PhysicalKnown && (alloc.Vcpus < float64(spec.Flavor.VCPUs) || alloc.MemoryMB < float64(spec.Flavor.RAMMB)) {
				continue
			}
			// Score = normalized free resource fraction (vCPU fraction + RAM fraction) / 2
			phys := parsePhysical(h.ResourcePhysical)
			score := resourceScore(alloc.Vcpus, float64(phys.Vcpus), alloc.MemoryMB, float64(phys.MemoryMB))
			candidates = append(candidates, hostScore{id: hostID, score: score})
		} else {
			candidates = append(candidates, hostScore{id: hostID, score: 0})
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoSuitableHost
	}

	// 3. Pick the highest-scoring host.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}
	result := &ScheduleResult{HostID: best.id}

	// 4. If storage is needed, find a suitable backend reachable from the chosen host.
	if spec.VolumeTypeID != nil {
		backends, err := s.storageSvc.ListBackendsReachableFromHost(ctx, best.id)
		if err != nil {
			return nil, fmt.Errorf("scheduler: list backends for host: %w", err)
		}

		var bestBackend *storage.Backend
		bestFree := int64(-1)
		for _, b := range backends {
			if b.State != storage.BackendStateActive {
				continue
			}
			if !capsMatch(b.Capabilities, spec.RequiredBackendCapabilities) {
				continue
			}
			// Score by free capacity. Use -1 as sentinel so backends with 0
			// total_capacity_gb (unknown/unlimited) are still eligible.
			if b.TotalCapacityGB > bestFree {
				bestFree = b.TotalCapacityGB
				bc := b
				bestBackend = &bc
			}
		}
		if bestBackend == nil {
			return nil, ErrNoSuitableBackend
		}
		result.BackendID = &bestBackend.ID
	}

	return result, nil
}

// storageDomainIDsForAZ resolves storage domain IDs for an AZ by querying
// the topology service for all active storage domains then cross-referencing
// via the storage domain association.
func (s *DefaultScheduler) storageDomainIDsForAZ(ctx context.Context, azID uuid.UUID) ([]uuid.UUID, error) {
	all, err := s.topologySvc.ListStorageDomains(ctx)
	if err != nil {
		return nil, err
	}
	// When no AZ is specified, all storage domains are candidates.
	if azID == uuid.Nil {
		ids := make([]uuid.UUID, 0, len(all))
		for _, sd := range all {
			ids = append(ids, sd.ID)
		}
		return ids, nil
	}
	// Filter to storage domains that have at least one active backend in the given AZ.
	backendsInAZ, err := s.storageSvc.ListBackendsForAZ(ctx, azID)
	if err != nil {
		return nil, err
	}
	sdSet := make(map[uuid.UUID]struct{})
	for _, b := range backendsInAZ {
		sdSet[b.StorageDomainID] = struct{}{}
	}
	var ids []uuid.UUID
	for _, sd := range all {
		if _, ok := sdSet[sd.ID]; ok {
			ids = append(ids, sd.ID)
		}
	}
	return ids, nil
}

func resourceScore(freeVCPUs, totalVCPUs, freeRAM, totalRAM float64) float64 {
	if totalVCPUs == 0 || totalRAM == 0 {
		return 0
	}
	return (freeVCPUs/totalVCPUs + freeRAM/totalRAM) / 2
}

func capsMatch(rawCaps json.RawMessage, required []string) bool {
	if len(required) == 0 {
		return true
	}
	var caps []string
	if err := json.Unmarshal(rawCaps, &caps); err != nil {
		return false
	}
	capSet := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		capSet[c] = struct{}{}
	}
	for _, r := range required {
		if _, ok := capSet[r]; !ok {
			return false
		}
	}
	return true
}

func parsePhysical(raw json.RawMessage) host.PhysicalResources {
	var p host.PhysicalResources
	_ = json.Unmarshal(raw, &p)
	return p
}
