// Package state provides in-memory storage for backends and volumes.
package state

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Snapshot represents a point-in-time snapshot of a volume.
type Snapshot struct {
	SnapshotID  string            `json:"snapshot_id"`
	VolumeID    string            `json:"volume_id"`
	SizeGB      int64             `json:"size_gb"`
	ConsumedGB  int64             `json:"consumed_gb"`
	State       string            `json:"state"`
	ChildClones []string          `json:"child_clones"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// OperationState represents the state of an async operation.
type OperationState string

const (
	// OperationRunning indicates the operation is in progress.
	OperationRunning OperationState = "running"
	// OperationCompleted indicates the operation finished successfully.
	OperationCompleted OperationState = "completed"
	// OperationFailed indicates the operation failed.
	OperationFailed OperationState = "failed"
)

// Operation represents an async operation such as flatten.
type Operation struct {
	OperationID         string         `json:"operation_id"`
	Type                string         `json:"type"`
	VolumeID            string         `json:"volume_id"`
	State               OperationState `json:"state"`
	ProgressPercent     int            `json:"progress_percent"`
	ElapsedMs           int64          `json:"elapsed_ms"`
	EstimatedRemainingMs int64         `json:"estimated_remaining_ms"`
	StartedAt           time.Time      `json:"started_at"`
}

// BackendState represents the operational state of a storage backend.
type BackendState string

const (
	// BackendActive indicates the backend is fully operational.
	BackendActive BackendState = "active"
	// BackendDraining indicates the backend is draining (no new volumes).
	BackendDraining BackendState = "draining"
	// BackendReadOnly indicates the backend is read-only.
	BackendReadOnly BackendState = "read_only"
	// BackendRetired indicates the backend is retired.
	BackendRetired BackendState = "retired"
)

// VolumeState represents the state of a volume.
type VolumeState string

const (
	// VolumeAvailable indicates the volume is available for use.
	VolumeAvailable VolumeState = "available"
	// VolumeInUse indicates the volume is currently exported.
	VolumeInUse VolumeState = "in_use"
)

// QoSPolicy defines quality of service limits for a volume.
type QoSPolicy struct {
	MaxIOPS         int `json:"max_iops,omitempty"`
	MaxBandwidthMBs int `json:"max_bandwidth_mbps,omitempty"`
}

// Backend represents a storage backend.
type Backend struct {
	BackendID          string       `json:"backend_id"`
	TotalCapacityGB    int64        `json:"total_capacity_gb"`
	UsedCapacityGB     int64        `json:"used_capacity_gb"`
	AllocatedCapacityGB int64       `json:"allocated_capacity_gb"`
	TotalIOPS          int64        `json:"total_iops"`
	UsedIOPSEstimate   int64        `json:"used_iops_estimate"`
	Capabilities       []string     `json:"capabilities"`
	State              BackendState `json:"state"`
	OverprovisionRatio float64      `json:"overprovision_ratio"`
}

// Volume represents a storage volume.
type Volume struct {
	VolumeID         string            `json:"volume_id"`
	BackendID        string            `json:"backend_id"`
	SizeGB           int64             `json:"size_gb"`
	ConsumedGB       int64             `json:"consumed_gb"`
	ThinProvisioned  bool              `json:"thin_provisioned"`
	State            VolumeState       `json:"state"`
	QoSPolicy        *QoSPolicy        `json:"qos_policy,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	ExportInfo       *ExportInfo       `json:"export_info,omitempty"`
	Snapshots        []string          `json:"snapshots,omitempty"`
	ParentSnapshotID string            `json:"parent_snapshot_id,omitempty"`
}

// ExportInfo holds details about an exported volume.
type ExportInfo struct {
	HostID   string `json:"host_id"`
	Protocol string `json:"protocol"`
}

// SimConfig holds simulation configuration.
type SimConfig struct {
	DefaultLatencyMs int `json:"default_latency_ms"`
	FlattenPerGBMs   int `json:"flatten_per_gb_ms,omitempty"`
	MigrationPerGBMs int `json:"migration_per_gb_ms,omitempty"`
}

// Stats holds overall simulation statistics.
type Stats struct {
	BackendCount int `json:"backend_count"`
	VolumeCount  int `json:"volume_count"`
	ExportCount  int `json:"export_count"`
}

// Store is a thread-safe in-memory state store for storage simulation.
type Store struct {
	mu         sync.RWMutex
	backends   map[string]*Backend
	volumes    map[string]*Volume
	snapshots  map[string]*Snapshot
	operations map[string]*Operation
	migrations map[string]*Migration
	config     SimConfig
	logger     *slog.Logger
	opCounter  int
	timeNow    func() time.Time // injectable clock for testing
}

// NewStore creates a new empty Store.
func NewStore(logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{
		backends:   make(map[string]*Backend),
		volumes:    make(map[string]*Volume),
		snapshots:  make(map[string]*Snapshot),
		operations: make(map[string]*Operation),
		migrations: make(map[string]*Migration),
		config:     SimConfig{DefaultLatencyMs: 2, FlattenPerGBMs: 100, MigrationPerGBMs: 200},
		logger:     logger,
		timeNow:    time.Now,
	}
}

// AddBackend registers a new storage backend.
func (s *Store) AddBackend(_ context.Context, b Backend) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if b.BackendID == "" {
		return fmt.Errorf("add backend: %w", ErrEmptyBackendID)
	}
	if _, exists := s.backends[b.BackendID]; exists {
		return fmt.Errorf("add backend %q: %w", b.BackendID, ErrBackendExists)
	}
	if b.State == "" {
		b.State = BackendActive
	}
	if b.OverprovisionRatio == 0 {
		b.OverprovisionRatio = 1.0
	}
	stored := b
	s.backends[b.BackendID] = &stored
	s.logger.Info("backend registered", "backend_id", b.BackendID)
	return nil
}

// GetBackend returns a backend by ID.
func (s *Store) GetBackend(_ context.Context, id string) (*Backend, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.backends[id]
	if !ok {
		return nil, fmt.Errorf("get backend %q: %w", id, ErrBackendNotFound)
	}
	cp := *b
	return &cp, nil
}

// ListBackends returns all registered backends.
func (s *Store) ListBackends(_ context.Context) []Backend {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Backend, 0, len(s.backends))
	for _, b := range s.backends {
		result = append(result, *b)
	}
	return result
}

// SetBackendState changes the state of a backend.
func (s *Store) SetBackendState(_ context.Context, id string, state BackendState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.backends[id]
	if !ok {
		return fmt.Errorf("set backend state %q: %w", id, ErrBackendNotFound)
	}
	b.State = state
	s.logger.Info("backend state changed", "backend_id", id, "state", state)
	return nil
}

// CreateVolume creates a new volume on the specified backend.
func (s *Store) CreateVolume(_ context.Context, v Volume) (*Volume, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v.VolumeID == "" {
		return nil, fmt.Errorf("create volume: %w", ErrEmptyVolumeID)
	}
	if _, exists := s.volumes[v.VolumeID]; exists {
		return nil, fmt.Errorf("create volume %q: %w", v.VolumeID, ErrVolumeExists)
	}

	b, ok := s.backends[v.BackendID]
	if !ok {
		return nil, fmt.Errorf("create volume on backend %q: %w", v.BackendID, ErrBackendNotFound)
	}

	if b.State != BackendActive {
		return nil, fmt.Errorf("create volume on backend %q (state %s): %w", v.BackendID, b.State, ErrBackendNotActive)
	}

	// Capacity check
	if v.ThinProvisioned {
		maxAlloc := int64(float64(b.TotalCapacityGB) * b.OverprovisionRatio)
		if b.AllocatedCapacityGB+v.SizeGB > maxAlloc {
			return nil, fmt.Errorf("create volume %q: %w", v.VolumeID, ErrInsufficientCapacity)
		}
		b.AllocatedCapacityGB += v.SizeGB
	} else {
		if b.UsedCapacityGB+v.SizeGB > b.TotalCapacityGB {
			return nil, fmt.Errorf("create volume %q: %w", v.VolumeID, ErrInsufficientCapacity)
		}
		b.UsedCapacityGB += v.SizeGB
		b.AllocatedCapacityGB += v.SizeGB
	}

	v.State = VolumeAvailable
	v.ConsumedGB = 0
	v.CreatedAt = time.Now()
	stored := v
	s.volumes[v.VolumeID] = &stored
	s.logger.Info("volume created", "volume_id", v.VolumeID, "backend_id", v.BackendID)
	cp := stored
	return &cp, nil
}

// GetVolume returns a volume by ID.
func (s *Store) GetVolume(_ context.Context, id string) (*Volume, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.volumes[id]
	if !ok {
		return nil, fmt.Errorf("get volume %q: %w", id, ErrVolumeNotFound)
	}
	cp := *v
	return &cp, nil
}

// ListVolumes returns volumes, optionally filtered by state and/or backend.
func (s *Store) ListVolumes(_ context.Context, backendID string, stateFilter VolumeState) []Volume {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Volume, 0)
	for _, v := range s.volumes {
		if backendID != "" && v.BackendID != backendID {
			continue
		}
		if stateFilter != "" && v.State != stateFilter {
			continue
		}
		result = append(result, *v)
	}
	return result
}

// DeleteVolume removes a volume if it is available and has no snapshots.
func (s *Store) DeleteVolume(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.volumes[id]
	if !ok {
		return fmt.Errorf("delete volume %q: %w", id, ErrVolumeNotFound)
	}
	if v.State == VolumeInUse {
		return fmt.Errorf("delete volume %q: %w", id, ErrVolumeInUse)
	}
	if len(v.Snapshots) > 0 {
		return fmt.Errorf("delete volume %q: %w", id, ErrVolumeHasSnapshots)
	}

	// Return capacity to backend
	if b, ok := s.backends[v.BackendID]; ok {
		b.AllocatedCapacityGB -= v.SizeGB
		if !v.ThinProvisioned {
			b.UsedCapacityGB -= v.SizeGB
		}
	}

	delete(s.volumes, id)
	s.logger.Info("volume deleted", "volume_id", id)
	return nil
}

// ExtendVolume increases the size of a volume.
func (s *Store) ExtendVolume(_ context.Context, id string, newSizeGB int64) (*Volume, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.volumes[id]
	if !ok {
		return nil, fmt.Errorf("extend volume %q: %w", id, ErrVolumeNotFound)
	}
	if newSizeGB <= v.SizeGB {
		return nil, fmt.Errorf("extend volume %q: new size %d <= current size %d: %w", id, newSizeGB, v.SizeGB, ErrShrinkNotAllowed)
	}

	b, ok := s.backends[v.BackendID]
	if !ok {
		return nil, fmt.Errorf("extend volume %q: backend %q: %w", id, v.BackendID, ErrBackendNotFound)
	}

	diff := newSizeGB - v.SizeGB

	if v.ThinProvisioned {
		maxAlloc := int64(float64(b.TotalCapacityGB) * b.OverprovisionRatio)
		if b.AllocatedCapacityGB+diff > maxAlloc {
			return nil, fmt.Errorf("extend volume %q: %w", id, ErrInsufficientCapacity)
		}
		b.AllocatedCapacityGB += diff
	} else {
		if b.UsedCapacityGB+diff > b.TotalCapacityGB {
			return nil, fmt.Errorf("extend volume %q: %w", id, ErrInsufficientCapacity)
		}
		b.UsedCapacityGB += diff
		b.AllocatedCapacityGB += diff
	}

	v.SizeGB = newSizeGB
	s.logger.Info("volume extended", "volume_id", id, "new_size_gb", newSizeGB)
	cp := *v
	return &cp, nil
}

// ExportVolume marks a volume as exported to a host.
func (s *Store) ExportVolume(_ context.Context, id string, hostID string, protocol string) (*Volume, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.volumes[id]
	if !ok {
		return nil, fmt.Errorf("export volume %q: %w", id, ErrVolumeNotFound)
	}
	if v.State == VolumeInUse {
		return nil, fmt.Errorf("export volume %q: %w", id, ErrVolumeAlreadyExported)
	}

	v.State = VolumeInUse
	v.ExportInfo = &ExportInfo{
		HostID:   hostID,
		Protocol: protocol,
	}
	s.logger.Info("volume exported", "volume_id", id, "host_id", hostID)
	cp := *v
	return &cp, nil
}

// UnexportVolume removes an export from a volume.
func (s *Store) UnexportVolume(_ context.Context, id string) (*Volume, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.volumes[id]
	if !ok {
		return nil, fmt.Errorf("unexport volume %q: %w", id, ErrVolumeNotFound)
	}
	if v.State != VolumeInUse {
		return nil, fmt.Errorf("unexport volume %q: %w", id, ErrVolumeNotExported)
	}

	v.State = VolumeAvailable
	v.ExportInfo = nil
	s.logger.Info("volume unexported", "volume_id", id)
	cp := *v
	return &cp, nil
}

// SetConfig updates the simulation configuration.
func (s *Store) SetConfig(_ context.Context, cfg SimConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
}

// GetConfig returns the current simulation configuration.
func (s *Store) GetConfig(_ context.Context) SimConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetStats returns overall simulation statistics.
func (s *Store) GetStats(_ context.Context) Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	exportCount := 0
	for _, v := range s.volumes {
		if v.State == VolumeInUse {
			exportCount++
		}
	}
	return Stats{
		BackendCount: len(s.backends),
		VolumeCount:  len(s.volumes),
		ExportCount:  exportCount,
	}
}

// Reset clears all state.
func (s *Store) Reset(_ context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends = make(map[string]*Backend)
	s.volumes = make(map[string]*Volume)
	s.snapshots = make(map[string]*Snapshot)
	s.operations = make(map[string]*Operation)
	s.migrations = make(map[string]*Migration)
	s.config = SimConfig{DefaultLatencyMs: 2, FlattenPerGBMs: 100, MigrationPerGBMs: 200}
	s.opCounter = 0
	s.logger.Info("state reset")
}

// CreateSnapshot creates a snapshot for a volume.
func (s *Store) CreateSnapshot(_ context.Context, volumeID string, snapshotID string, metadata map[string]string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if snapshotID == "" {
		return nil, fmt.Errorf("create snapshot: %w", ErrEmptySnapshotID)
	}
	if _, exists := s.snapshots[snapshotID]; exists {
		return nil, fmt.Errorf("create snapshot %q: %w", snapshotID, ErrSnapshotExists)
	}

	v, ok := s.volumes[volumeID]
	if !ok {
		return nil, fmt.Errorf("create snapshot for volume %q: %w", volumeID, ErrVolumeNotFound)
	}

	snap := &Snapshot{
		SnapshotID:  snapshotID,
		VolumeID:    volumeID,
		SizeGB:      v.SizeGB,
		ConsumedGB:  0,
		State:       "available",
		ChildClones: []string{},
		Metadata:    metadata,
		CreatedAt:   s.timeNow(),
	}

	s.snapshots[snapshotID] = snap
	v.Snapshots = append(v.Snapshots, snapshotID)
	s.logger.Info("snapshot created", "snapshot_id", snapshotID, "volume_id", volumeID)
	cp := *snap
	return &cp, nil
}

// ListSnapshots returns all snapshots for a volume.
func (s *Store) ListSnapshots(_ context.Context, volumeID string) ([]Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.volumes[volumeID]
	if !ok {
		return nil, fmt.Errorf("list snapshots for volume %q: %w", volumeID, ErrVolumeNotFound)
	}

	result := make([]Snapshot, 0, len(v.Snapshots))
	for _, sid := range v.Snapshots {
		if snap, ok := s.snapshots[sid]; ok {
			result = append(result, *snap)
		}
	}
	return result, nil
}

// GetSnapshot returns a snapshot by ID.
func (s *Store) GetSnapshot(_ context.Context, snapshotID string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap, ok := s.snapshots[snapshotID]
	if !ok {
		return nil, fmt.Errorf("get snapshot %q: %w", snapshotID, ErrSnapshotNotFound)
	}
	cp := *snap
	return &cp, nil
}

// DeleteSnapshot removes a snapshot if it has no child clones.
func (s *Store) DeleteSnapshot(_ context.Context, snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, ok := s.snapshots[snapshotID]
	if !ok {
		return fmt.Errorf("delete snapshot %q: %w", snapshotID, ErrSnapshotNotFound)
	}
	if len(snap.ChildClones) > 0 {
		return fmt.Errorf("delete snapshot %q: %w", snapshotID, ErrSnapshotHasClones)
	}

	// Remove snapshot ID from parent volume's Snapshots list
	if v, ok := s.volumes[snap.VolumeID]; ok {
		for i, sid := range v.Snapshots {
			if sid == snapshotID {
				v.Snapshots = append(v.Snapshots[:i], v.Snapshots[i+1:]...)
				break
			}
		}
	}

	delete(s.snapshots, snapshotID)
	s.logger.Info("snapshot deleted", "snapshot_id", snapshotID)
	return nil
}

// CloneFromSnapshot creates a new volume cloned from a snapshot.
func (s *Store) CloneFromSnapshot(_ context.Context, snapshotID string, volumeID string, metadata map[string]string) (*Volume, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if volumeID == "" {
		return nil, fmt.Errorf("clone from snapshot: %w", ErrEmptyVolumeID)
	}
	if _, exists := s.volumes[volumeID]; exists {
		return nil, fmt.Errorf("clone from snapshot %q: %w", volumeID, ErrVolumeExists)
	}

	snap, ok := s.snapshots[snapshotID]
	if !ok {
		return nil, fmt.Errorf("clone from snapshot %q: %w", snapshotID, ErrSnapshotNotFound)
	}

	// Find the source volume to get the backend
	srcVol, ok := s.volumes[snap.VolumeID]
	if !ok {
		return nil, fmt.Errorf("clone from snapshot %q: source volume %q: %w", snapshotID, snap.VolumeID, ErrVolumeNotFound)
	}

	b, ok := s.backends[srcVol.BackendID]
	if !ok {
		return nil, fmt.Errorf("clone from snapshot %q: backend %q: %w", snapshotID, srcVol.BackendID, ErrBackendNotFound)
	}

	if b.State != BackendActive {
		return nil, fmt.Errorf("clone from snapshot %q on backend %q (state %s): %w", snapshotID, srcVol.BackendID, b.State, ErrBackendNotActive)
	}

	// Capacity check: clone counts against allocated_capacity_gb (thin provisioned)
	maxAlloc := int64(float64(b.TotalCapacityGB) * b.OverprovisionRatio)
	if b.AllocatedCapacityGB+snap.SizeGB > maxAlloc {
		return nil, fmt.Errorf("clone from snapshot %q: %w", snapshotID, ErrInsufficientCapacity)
	}
	b.AllocatedCapacityGB += snap.SizeGB

	clone := &Volume{
		VolumeID:         volumeID,
		BackendID:        srcVol.BackendID,
		SizeGB:           snap.SizeGB,
		ConsumedGB:       0,
		ThinProvisioned:  true,
		State:            VolumeAvailable,
		Metadata:         metadata,
		CreatedAt:        s.timeNow(),
		ParentSnapshotID: snapshotID,
	}

	s.volumes[volumeID] = clone
	snap.ChildClones = append(snap.ChildClones, volumeID)
	s.logger.Info("volume cloned from snapshot", "volume_id", volumeID, "snapshot_id", snapshotID)
	cp := *clone
	return &cp, nil
}

// StartFlatten begins an async flatten operation for a volume.
func (s *Store) StartFlatten(ctx context.Context, volumeID string) (*Operation, error) {
	s.mu.Lock()

	v, ok := s.volumes[volumeID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("flatten volume %q: %w", volumeID, ErrVolumeNotFound)
	}
	if v.ParentSnapshotID == "" {
		s.mu.Unlock()
		return nil, fmt.Errorf("flatten volume %q: %w", volumeID, ErrVolumeNoParent)
	}

	s.opCounter++
	opID := fmt.Sprintf("op-%03d", s.opCounter)

	flattenPerGBMs := s.config.FlattenPerGBMs
	if flattenPerGBMs <= 0 {
		flattenPerGBMs = 100
	}
	durationMs := int64(v.SizeGB) * int64(flattenPerGBMs)

	op := &Operation{
		OperationID:         opID,
		Type:                "flatten",
		VolumeID:            volumeID,
		State:               OperationRunning,
		ProgressPercent:     0,
		ElapsedMs:           0,
		EstimatedRemainingMs: durationMs,
		StartedAt:           s.timeNow(),
	}

	s.operations[opID] = op
	s.logger.Info("flatten started", "operation_id", opID, "volume_id", volumeID, "duration_ms", durationMs)

	cp := *op
	s.mu.Unlock()

	// Run the flatten in a goroutine
	go s.runFlatten(ctx, opID, volumeID, durationMs)

	return &cp, nil
}

// runFlatten simulates async flatten progress.
func (s *Store) runFlatten(_ context.Context, opID string, volumeID string, durationMs int64) {
	// Update progress in steps
	steps := 10
	stepDuration := time.Duration(durationMs/int64(steps)) * time.Millisecond

	for i := 1; i <= steps; i++ {
		time.Sleep(stepDuration)

		s.mu.Lock()
		op, ok := s.operations[opID]
		if !ok {
			s.mu.Unlock()
			return
		}
		elapsed := time.Since(op.StartedAt).Milliseconds()
		progress := int(float64(i) / float64(steps) * 100)
		if progress > 100 {
			progress = 100
		}
		remaining := durationMs - elapsed
		if remaining < 0 {
			remaining = 0
		}
		op.ProgressPercent = progress
		op.ElapsedMs = elapsed
		op.EstimatedRemainingMs = remaining
		s.mu.Unlock()
	}

	// Complete the flatten
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[opID]
	if !ok {
		return
	}

	v, ok := s.volumes[volumeID]
	if !ok {
		op.State = OperationFailed
		return
	}

	parentSnapID := v.ParentSnapshotID

	// Update volume
	v.ConsumedGB = v.SizeGB
	v.ParentSnapshotID = ""

	// Remove volume from snapshot's child_clones
	if snap, ok := s.snapshots[parentSnapID]; ok {
		for i, cid := range snap.ChildClones {
			if cid == volumeID {
				snap.ChildClones = append(snap.ChildClones[:i], snap.ChildClones[i+1:]...)
				break
			}
		}
	}

	op.State = OperationCompleted
	op.ProgressPercent = 100
	op.ElapsedMs = time.Since(op.StartedAt).Milliseconds()
	op.EstimatedRemainingMs = 0
	s.logger.Info("flatten completed", "operation_id", opID, "volume_id", volumeID)
}

// GetOperation returns an operation by ID.
func (s *Store) GetOperation(_ context.Context, opID string) (*Operation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	op, ok := s.operations[opID]
	if !ok {
		return nil, fmt.Errorf("get operation %q: %w", opID, ErrOperationNotFound)
	}
	cp := *op
	return &cp, nil
}

// Migration represents an in-progress storage migration.
type Migration struct {
	MigrationID      string         `json:"migration_id"`
	VolumeID         string         `json:"volume_id"`
	SourceBackendID  string         `json:"source_backend_id"`
	DestBackendID    string         `json:"dest_backend_id"`
	State            OperationState `json:"state"`
	ProgressPercent  int            `json:"progress_percent"`
	BytesTransferred int64          `json:"bytes_transferred"`
	ElapsedMs        int64          `json:"elapsed_ms"`
	EstimatedRemainingMs int64      `json:"estimated_remaining_ms"`
	StartedAt        time.Time      `json:"started_at"`
	cancelled        bool
}

// StartMigration begins an async storage migration for a volume.
func (s *Store) StartMigration(ctx context.Context, volumeID, destBackendID string) (*Migration, error) {
	s.mu.Lock()

	v, ok := s.volumes[volumeID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("migrate volume %q: %w", volumeID, ErrVolumeNotFound)
	}
	if len(v.Snapshots) > 0 {
		s.mu.Unlock()
		return nil, fmt.Errorf("migrate volume %q: %w", volumeID, ErrVolumeHasSnapshots)
	}
	srcBackendID := v.BackendID

	destBackend, ok := s.backends[destBackendID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("migrate volume %q to %q: %w", volumeID, destBackendID, ErrBackendNotFound)
	}

	// Check destination capacity
	if v.ThinProvisioned {
		maxAlloc := int64(float64(destBackend.TotalCapacityGB) * destBackend.OverprovisionRatio)
		if destBackend.AllocatedCapacityGB+v.SizeGB > maxAlloc {
			s.mu.Unlock()
			return nil, fmt.Errorf("migrate volume %q: %w", volumeID, ErrInsufficientCapacity)
		}
	} else {
		if destBackend.UsedCapacityGB+v.SizeGB > destBackend.TotalCapacityGB {
			s.mu.Unlock()
			return nil, fmt.Errorf("migrate volume %q: %w", volumeID, ErrInsufficientCapacity)
		}
	}

	s.opCounter++
	migID := fmt.Sprintf("mig-%03d", s.opCounter)

	migrationPerGBMs := s.config.MigrationPerGBMs
	if migrationPerGBMs <= 0 {
		migrationPerGBMs = 200
	}
	durationMs := int64(v.SizeGB) * int64(migrationPerGBMs)
	totalBytes := v.SizeGB * 1024 * 1024 * 1024

	mig := &Migration{
		MigrationID:         migID,
		VolumeID:            volumeID,
		SourceBackendID:     srcBackendID,
		DestBackendID:       destBackendID,
		State:               "preparing",
		ProgressPercent:     0,
		BytesTransferred:    0,
		ElapsedMs:           0,
		EstimatedRemainingMs: durationMs,
		StartedAt:           s.timeNow(),
	}

	if s.migrations == nil {
		s.migrations = make(map[string]*Migration)
	}
	s.migrations[migID] = mig

	sizeGB := v.SizeGB
	thinProvisioned := v.ThinProvisioned
	cp := *mig
	s.mu.Unlock()

	go s.runMigration(ctx, migID, volumeID, srcBackendID, destBackendID, sizeGB, thinProvisioned, totalBytes, durationMs)

	return &cp, nil
}

// GetMigration returns a migration by ID.
func (s *Store) GetMigration(_ context.Context, migID string) (*Migration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mig, ok := s.migrations[migID]
	if !ok {
		return nil, fmt.Errorf("get migration %q: %w", migID, ErrMigrationNotFound)
	}
	cp := *mig
	return &cp, nil
}

// CancelMigration cancels an in-progress migration.
func (s *Store) CancelMigration(_ context.Context, migID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mig, ok := s.migrations[migID]
	if !ok {
		return fmt.Errorf("cancel migration %q: %w", migID, ErrMigrationNotFound)
	}
	if mig.State != "preparing" && mig.State != "transferring" {
		return fmt.Errorf("cancel migration %q: cannot cancel in %s state", migID, mig.State)
	}

	mig.cancelled = true
	mig.State = "failed"
	return nil
}

func (s *Store) runMigration(_ context.Context, migID, volumeID, srcBackendID, destBackendID string, sizeGB int64, thinProvisioned bool, totalBytes, durationMs int64) {
	// Preparing phase (10% of time)
	prepTime := time.Duration(durationMs/10) * time.Millisecond
	time.Sleep(prepTime)

	s.mu.Lock()
	mig, ok := s.migrations[migID]
	if !ok || mig.cancelled {
		s.mu.Unlock()
		return
	}
	mig.State = "transferring"
	s.mu.Unlock()

	// Transferring phase (80% of time in 10 steps)
	steps := 10
	stepDuration := time.Duration(durationMs*8/10/int64(steps)) * time.Millisecond

	for i := 1; i <= steps; i++ {
		time.Sleep(stepDuration)

		s.mu.Lock()
		mig, ok = s.migrations[migID]
		if !ok || mig.cancelled {
			s.mu.Unlock()
			return
		}
		progress := int(float64(i) / float64(steps) * 90)
		mig.ProgressPercent = progress
		mig.BytesTransferred = totalBytes * int64(i) / int64(steps)
		mig.ElapsedMs = time.Since(mig.StartedAt).Milliseconds()
		remaining := durationMs - mig.ElapsedMs
		if remaining < 0 {
			remaining = 0
		}
		mig.EstimatedRemainingMs = remaining
		s.mu.Unlock()
	}

	// Finishing phase
	s.mu.Lock()
	mig, ok = s.migrations[migID]
	if !ok || mig.cancelled {
		s.mu.Unlock()
		return
	}
	mig.State = "finishing"
	s.mu.Unlock()

	finishTime := time.Duration(durationMs/10) * time.Millisecond
	time.Sleep(finishTime)

	// Complete: move volume from source to destination
	s.mu.Lock()
	defer s.mu.Unlock()

	mig, ok = s.migrations[migID]
	if !ok || mig.cancelled {
		return
	}

	v, ok := s.volumes[volumeID]
	if !ok {
		mig.State = "failed"
		return
	}

	// Remove capacity from source backend
	if srcB, ok := s.backends[srcBackendID]; ok {
		srcB.AllocatedCapacityGB -= sizeGB
		if !thinProvisioned {
			srcB.UsedCapacityGB -= sizeGB
		}
	}

	// Add capacity to destination backend
	if destB, ok := s.backends[destBackendID]; ok {
		destB.AllocatedCapacityGB += sizeGB
		if !thinProvisioned {
			destB.UsedCapacityGB += sizeGB
		}
	}

	v.BackendID = destBackendID

	mig.State = "completed"
	mig.ProgressPercent = 100
	mig.BytesTransferred = totalBytes
	mig.ElapsedMs = time.Since(mig.StartedAt).Milliseconds()
	mig.EstimatedRemainingMs = 0
	s.logger.Info("migration completed", "migration_id", migID, "volume_id", volumeID)
}

// Sentinel errors for the state package.
var (
	ErrEmptyBackendID       = fmt.Errorf("backend_id is required")
	ErrBackendExists        = fmt.Errorf("backend already exists")
	ErrBackendNotFound      = fmt.Errorf("backend not found")
	ErrBackendNotActive     = fmt.Errorf("backend is not active")
	ErrEmptyVolumeID        = fmt.Errorf("volume_id is required")
	ErrVolumeExists         = fmt.Errorf("volume already exists")
	ErrVolumeNotFound       = fmt.Errorf("volume not found")
	ErrVolumeInUse          = fmt.Errorf("volume is in use")
	ErrVolumeHasSnapshots   = fmt.Errorf("volume has snapshots")
	ErrVolumeAlreadyExported = fmt.Errorf("volume is already exported")
	ErrVolumeNotExported    = fmt.Errorf("volume is not exported")
	ErrInsufficientCapacity = fmt.Errorf("insufficient storage capacity")
	ErrShrinkNotAllowed     = fmt.Errorf("shrinking volumes is not allowed")
	ErrEmptySnapshotID      = fmt.Errorf("snapshot_id is required")
	ErrSnapshotExists       = fmt.Errorf("snapshot already exists")
	ErrSnapshotNotFound     = fmt.Errorf("snapshot not found")
	ErrSnapshotHasClones    = fmt.Errorf("snapshot has active clones")
	ErrVolumeNoParent       = fmt.Errorf("volume has no parent snapshot")
	ErrOperationNotFound    = fmt.Errorf("operation not found")
	ErrMigrationNotFound    = fmt.Errorf("migration not found")
)
