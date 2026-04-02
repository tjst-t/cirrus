package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// DriverRegistry maps driver name → factory function.
// Populated by the application wiring (e.g. controller main).
// backendID is the UUID of the backend record, used as the backend identifier
// when communicating with the backend (e.g. X-Backend-Id header for storage-sim).
type DriverRegistry map[string]func(endpoint string, backendID string, config map[string]any) Driver

// storageStore is the subset of Store methods used by serviceImpl.
// Extracted as an interface to allow unit testing with fake implementations.
type storageStore interface {
	InsertBackend(ctx context.Context, b Backend) (*Backend, error)
	GetBackend(ctx context.Context, id uuid.UUID) (*Backend, error)
	ListBackends(ctx context.Context) ([]Backend, error)
	SetBackendState(ctx context.Context, id uuid.UUID, state BackendState) error
	ListActiveBackendsForAZ(ctx context.Context, azID uuid.UUID) ([]Backend, error)

	InsertVolumeType(ctx context.Context, vt VolumeType) (*VolumeType, error)
	GetVolumeType(ctx context.Context, id uuid.UUID) (*VolumeType, error)
	ListVolumeTypes(ctx context.Context) ([]VolumeType, error)

	InsertVolume(ctx context.Context, v Volume) (*Volume, error)
	GetVolume(ctx context.Context, id uuid.UUID) (*Volume, error)
	ListVolumesByTenant(ctx context.Context, tenantID uuid.UUID) ([]Volume, error)
	ListVolumesByBackend(ctx context.Context, backendID uuid.UUID) ([]Volume, error)
	SetVolumeState(ctx context.Context, id uuid.UUID, state VolumeState) error
	SetVolumeExport(ctx context.Context, id, hostID uuid.UUID, info json.RawMessage) error
	ClearVolumeExport(ctx context.Context, id uuid.UUID) error
	ResizeVolume(ctx context.Context, id uuid.UUID, newSizeGB int64) (*Volume, error)
	DeleteVolume(ctx context.Context, id uuid.UUID) error

	GetHostStorageProperties(ctx context.Context, hostID uuid.UUID) (map[string]string, error)
	GetHostDataIPs(ctx context.Context, hostID uuid.UUID) ([]string, error)
}

// serviceImpl implements Service.
type serviceImpl struct {
	store   storageStore
	drivers DriverRegistry
	logger  *slog.Logger
}

// NewService creates a new storage Service.
func NewService(store *Store, drivers DriverRegistry, logger *slog.Logger) Service {
	return &serviceImpl{store: store, drivers: drivers, logger: logger}
}

// --- Backend ---

func (s *serviceImpl) RegisterBackend(ctx context.Context, spec RegisterBackendSpec) (*Backend, error) {
	caps, _ := json.Marshal(spec.Capabilities)
	dcfg, _ := json.Marshal(spec.DriverConfig)
	b := Backend{
		StorageDomainID: spec.StorageDomainID,
		Name:            spec.Name,
		Driver:          spec.Driver,
		Endpoint:        spec.Endpoint,
		TotalCapacityGB: spec.TotalCapacityGB,
		TotalIOPS:       spec.TotalIOPS,
		Capabilities:    json.RawMessage(caps),
		DriverConfig:    json.RawMessage(dcfg),
	}
	result, err := s.store.InsertBackend(ctx, b)
	if err != nil {
		return nil, fmt.Errorf("storage service: register backend: %w", err)
	}
	s.logger.Info("storage backend registered", "id", result.ID, "name", result.Name, "driver", result.Driver)
	return result, nil
}

func (s *serviceImpl) GetBackend(ctx context.Context, id uuid.UUID) (*Backend, error) {
	return s.store.GetBackend(ctx, id)
}

func (s *serviceImpl) ListBackends(ctx context.Context) ([]Backend, error) {
	return s.store.ListBackends(ctx)
}

func (s *serviceImpl) DrainBackend(ctx context.Context, id uuid.UUID) error {
	if err := s.store.SetBackendState(ctx, id, BackendStateDraining); err != nil {
		return fmt.Errorf("storage service: drain backend: %w", err)
	}
	s.logger.Info("storage backend draining", "id", id)
	return nil
}

// --- VolumeType ---

func (s *serviceImpl) CreateVolumeType(ctx context.Context, name, description string, requiredCaps []string, qosPolicy map[string]any, isPublic bool) (*VolumeType, error) {
	caps, _ := json.Marshal(requiredCaps)
	qos, _ := json.Marshal(qosPolicy)
	vt := VolumeType{
		Name:                 name,
		Description:          description,
		RequiredCapabilities: json.RawMessage(caps),
		QoSPolicy:            json.RawMessage(qos),
		IsPublic:             isPublic,
	}
	result, err := s.store.InsertVolumeType(ctx, vt)
	if err != nil {
		return nil, fmt.Errorf("storage service: create volume type: %w", err)
	}
	return result, nil
}

func (s *serviceImpl) GetVolumeType(ctx context.Context, id uuid.UUID) (*VolumeType, error) {
	return s.store.GetVolumeType(ctx, id)
}

func (s *serviceImpl) ListVolumeTypes(ctx context.Context) ([]VolumeType, error) {
	return s.store.ListVolumeTypes(ctx)
}

// --- Volume ---

func (s *serviceImpl) CreateVolume(ctx context.Context, spec CreateVolumeSpec) (*Volume, error) {
	// Resolve required capabilities from volume type.
	var requiredCaps []string
	if spec.VolumeTypeID != nil {
		vt, err := s.store.GetVolumeType(ctx, *spec.VolumeTypeID)
		if err != nil {
			return nil, fmt.Errorf("storage service: create volume: get volume type: %w", err)
		}
		_ = json.Unmarshal(vt.RequiredCapabilities, &requiredCaps)
	}

	// Enumerate candidates: AZ-scoped if az_id is provided, otherwise all active backends.
	var (
		candidates []Backend
		err        error
	)
	if spec.AZID != nil {
		candidates, err = s.store.ListActiveBackendsForAZ(ctx, *spec.AZID)
		if err != nil {
			return nil, fmt.Errorf("storage service: create volume: list backends for az: %w", err)
		}
	} else {
		candidates, err = s.store.ListBackends(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage service: create volume: list backends: %w", err)
		}
	}

	// Select first backend that meets capability requirements.
	// Sprint 7 Scheduler will add capacity scoring and host affinity.
	var chosen *Backend
	for i := range candidates {
		b := &candidates[i]
		if b.State != BackendStateActive {
			continue
		}
		if !backendMeetsCaps(b, requiredCaps) {
			continue
		}
		chosen = b
		break
	}
	if chosen == nil {
		return nil, ErrNoMatchingBackend
	}

	// Insert volume record in creating state.
	vol := Volume{
		TenantID:     spec.TenantID,
		Name:         spec.Name,
		VolumeTypeID: spec.VolumeTypeID,
		BackendID:    &chosen.ID,
		SizeGB:       spec.SizeGB,
		AZID:         spec.AZID,
	}
	created, err := s.store.InsertVolume(ctx, vol)
	if err != nil {
		return nil, fmt.Errorf("storage service: create volume: insert: %w", err)
	}

	// Call driver to create the volume on the backend.
	driver, err := s.driverFor(chosen)
	if err != nil {
		_ = s.store.SetVolumeState(ctx, created.ID, VolumeStateError)
		return nil, fmt.Errorf("storage service: create volume: driver: %w", err)
	}

	_, err = driver.CreateVolume(ctx, DriverVolumeSpec{
		VolumeID:        created.ID.String(),
		SizeGB:          spec.SizeGB,
		ThinProvisioned: true,
	})
	if err != nil {
		_ = s.store.SetVolumeState(ctx, created.ID, VolumeStateError)
		return nil, fmt.Errorf("storage service: create volume: driver create: %w", err)
	}

	// Transition to available. If this DB write fails, the volume exists on the backend
	// but will be stuck in creating state. The Storage Reconciler (Sprint 8.5b) will
	// detect this drift and attempt recovery. As a best-effort compensating action we
	// try to delete the orphaned backend volume and mark the record as error.
	if err := s.store.SetVolumeState(ctx, created.ID, VolumeStateAvailable); err != nil {
		s.logger.Error("volume created on backend but DB state update failed; attempting rollback",
			"id", created.ID, "backend", chosen.Name, "error", err)
		if delErr := driver.DeleteVolume(ctx, created.ID.String()); delErr != nil {
			s.logger.Error("rollback delete on backend also failed — manual cleanup required",
				"id", created.ID, "backend", chosen.Name, "error", delErr)
		}
		_ = s.store.SetVolumeState(ctx, created.ID, VolumeStateError)
		return nil, fmt.Errorf("storage service: create volume: set available: %w", err)
	}
	created.State = VolumeStateAvailable
	s.logger.Info("volume created", "id", created.ID, "backend", chosen.Name, "size_gb", spec.SizeGB)
	return created, nil
}

func (s *serviceImpl) GetVolume(ctx context.Context, tenantID, volumeID uuid.UUID) (*Volume, error) {
	v, err := s.store.GetVolume(ctx, volumeID)
	if err != nil {
		return nil, err
	}
	if v.TenantID != tenantID {
		return nil, ErrVolumeNotFound
	}
	return v, nil
}

func (s *serviceImpl) ListVolumes(ctx context.Context, tenantID uuid.UUID) ([]Volume, error) {
	return s.store.ListVolumesByTenant(ctx, tenantID)
}

func (s *serviceImpl) DeleteVolume(ctx context.Context, tenantID, volumeID uuid.UUID) error {
	v, err := s.GetVolume(ctx, tenantID, volumeID)
	if err != nil {
		return err
	}
	if v.State == VolumeStateInUse {
		return ErrVolumeInUse
	}

	if err := s.store.SetVolumeState(ctx, volumeID, VolumeStateDeleting); err != nil {
		return fmt.Errorf("storage service: delete volume: set deleting: %w", err)
	}

	// Best-effort: delete from backend. Failures are logged but do not block the DB
	// record removal — the Reconciler will detect any orphaned backend volumes.
	if v.BackendID != nil {
		if b, err := s.store.GetBackend(ctx, *v.BackendID); err == nil {
			if driver, err := s.driverFor(b); err == nil {
				if err := driver.DeleteVolume(ctx, volumeID.String()); err != nil {
					s.logger.Warn("driver delete volume failed; orphaned backend volume may require manual cleanup",
						"volume_id", volumeID, "backend", b.Name, "error", err)
				}
			}
		}
	}

	if err := s.store.DeleteVolume(ctx, volumeID); err != nil {
		return fmt.Errorf("storage service: delete volume: %w", err)
	}
	s.logger.Info("volume deleted", "id", volumeID)
	return nil
}

func (s *serviceImpl) ResizeVolume(ctx context.Context, tenantID, volumeID uuid.UUID, newSizeGB int64) (*Volume, error) {
	v, err := s.GetVolume(ctx, tenantID, volumeID)
	if err != nil {
		return nil, err
	}
	if v.State == VolumeStateInUse {
		return nil, fmt.Errorf("storage service: resize volume: %w", ErrVolumeInUse)
	}
	if newSizeGB <= v.SizeGB {
		return nil, fmt.Errorf("storage service: resize volume: new size %d must be larger than current %d", newSizeGB, v.SizeGB)
	}

	if v.BackendID != nil {
		b, err := s.store.GetBackend(ctx, *v.BackendID)
		if err != nil {
			return nil, fmt.Errorf("storage service: resize volume: get backend: %w", err)
		}
		driver, err := s.driverFor(b)
		if err != nil {
			return nil, fmt.Errorf("storage service: resize volume: driver: %w", err)
		}
		if err := driver.ResizeVolume(ctx, volumeID.String(), newSizeGB); err != nil {
			return nil, fmt.Errorf("storage service: resize volume: driver: %w", err)
		}
	}

	// If this DB write fails after the backend was already resized, the sizes are out of
	// sync. The Reconciler (Sprint 8.5b) will detect this drift. Log at Error so it is
	// immediately visible in monitoring.
	result, err := s.store.ResizeVolume(ctx, volumeID, newSizeGB)
	if err != nil {
		s.logger.Error("backend resize succeeded but DB update failed — sizes are out of sync",
			"id", volumeID, "new_size_gb", newSizeGB, "error", err)
		return nil, fmt.Errorf("storage service: resize volume: %w", err)
	}
	s.logger.Info("volume resized", "id", volumeID, "new_size_gb", newSizeGB)
	return result, nil
}

func (s *serviceImpl) ExportVolume(ctx context.Context, volumeID, hostID uuid.UUID) (*ExportInfo, error) {
	v, err := s.store.GetVolume(ctx, volumeID)
	if err != nil {
		return nil, err
	}
	if v.State == VolumeStateInUse {
		return nil, ErrVolumeInUse
	}
	if v.BackendID == nil {
		return nil, fmt.Errorf("storage service: export volume: no backend assigned")
	}

	hostInfo, err := s.buildHostInfo(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("storage service: export volume: %w", err)
	}

	b, err := s.store.GetBackend(ctx, *v.BackendID)
	if err != nil {
		return nil, fmt.Errorf("storage service: export volume: get backend: %w", err)
	}
	driver, err := s.driverFor(b)
	if err != nil {
		return nil, fmt.Errorf("storage service: export volume: driver: %w", err)
	}

	info, err := driver.ExportVolume(ctx, volumeID.String(), hostInfo)
	if err != nil {
		return nil, fmt.Errorf("storage service: export volume: driver export: %w", err)
	}

	// Record the export in the DB. If this fails we roll back the backend export so
	// that the DB and the backend stay in sync.
	infoJSON, _ := json.Marshal(info)
	if err := s.store.SetVolumeExport(ctx, volumeID, hostID, infoJSON); err != nil {
		s.logger.Error("export recorded on backend but DB write failed; rolling back backend export",
			"volume_id", volumeID, "host_id", hostID, "error", err)
		if rbErr := driver.UnexportVolume(ctx, volumeID.String(), hostInfo); rbErr != nil {
			s.logger.Error("export rollback on backend also failed — manual cleanup required",
				"volume_id", volumeID, "host_id", hostID, "error", rbErr)
		}
		return nil, fmt.Errorf("storage service: export volume: record export: %w", err)
	}
	s.logger.Info("volume exported", "volume_id", volumeID, "host_id", hostID)
	return info, nil
}

func (s *serviceImpl) UnexportVolume(ctx context.Context, volumeID uuid.UUID) error {
	v, err := s.store.GetVolume(ctx, volumeID)
	if err != nil {
		return err
	}
	if v.State != VolumeStateInUse || v.BackendID == nil || v.ExportedHostID == nil {
		return nil // already unexported
	}

	hostInfo, err := s.buildHostInfo(ctx, *v.ExportedHostID)
	if err != nil {
		// Non-fatal: host info is used only to revoke backend ACLs. Log and proceed
		// with best-effort unexport using an empty HostInfo.
		s.logger.Warn("could not retrieve host info for unexport; proceeding with empty host info",
			"volume_id", volumeID, "host_id", v.ExportedHostID, "error", err)
		hostInfo = HostInfo{ID: v.ExportedHostID.String()}
	}

	if b, err := s.store.GetBackend(ctx, *v.BackendID); err == nil {
		if driver, err := s.driverFor(b); err == nil {
			if err := driver.UnexportVolume(ctx, volumeID.String(), hostInfo); err != nil {
				// The backend still has the export active. Do NOT clear the DB record —
				// keeping the volume in in_use prevents other VMs from attaching it while
				// the host may still have access. Return an error so the caller can retry.
				return fmt.Errorf("storage service: unexport volume: driver: %w", err)
			}
		}
	}

	if err := s.store.ClearVolumeExport(ctx, volumeID); err != nil {
		return fmt.Errorf("storage service: unexport volume: %w", err)
	}
	s.logger.Info("volume unexported", "volume_id", volumeID)
	return nil
}

func (s *serviceImpl) ListVolumesOnBackend(ctx context.Context, backendID uuid.UUID) ([]Volume, error) {
	return s.store.ListVolumesByBackend(ctx, backendID)
}

// --- helpers ---

func (s *serviceImpl) driverFor(b *Backend) (Driver, error) {
	factory, ok := s.drivers[b.Driver]
	if !ok {
		return nil, fmt.Errorf("no driver registered for type %q", b.Driver)
	}
	var cfg map[string]any
	_ = json.Unmarshal(b.DriverConfig, &cfg)
	return factory(b.Endpoint, b.ID.String(), cfg), nil
}

// buildHostInfo assembles a HostInfo for the given host from the DB.
func (s *serviceImpl) buildHostInfo(ctx context.Context, hostID uuid.UUID) (HostInfo, error) {
	props, err := s.store.GetHostStorageProperties(ctx, hostID)
	if err != nil {
		return HostInfo{}, fmt.Errorf("build host info: storage properties: %w", err)
	}
	ips, err := s.store.GetHostDataIPs(ctx, hostID)
	if err != nil {
		return HostInfo{}, fmt.Errorf("build host info: data ips: %w", err)
	}
	return HostInfo{ID: hostID.String(), DataIPs: ips, Properties: props}, nil
}

// backendMeetsCaps returns true if the backend's capabilities include all required caps.
func backendMeetsCaps(b *Backend, required []string) bool {
	if len(required) == 0 {
		return true
	}
	var avail []string
	_ = json.Unmarshal(b.Capabilities, &avail)
	availSet := make(map[string]struct{}, len(avail))
	for _, c := range avail {
		availSet[c] = struct{}{}
	}
	for _, r := range required {
		if _, ok := availSet[r]; !ok {
			return false
		}
	}
	return true
}
