package storage

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// --- fakeStore ---

// fakeStore is a minimal in-memory storageStore for unit tests.
// Only the methods exercised by each test need to return meaningful values;
// all others panic so accidental calls are immediately visible.
type fakeStore struct {
	backends    []Backend
	volumeTypes []VolumeType
	volumes     []Volume

	// Optional overrides injected per-test.
	listActiveBackendsForAZFn func(ctx context.Context, azID uuid.UUID) ([]Backend, error)
	insertVolumeFn            func(ctx context.Context, v Volume) (*Volume, error)
	setVolumeStateFn          func(ctx context.Context, id uuid.UUID, state VolumeState) error
	getVolumeFn               func(ctx context.Context, id uuid.UUID) (*Volume, error)
	resizeVolumeFn            func(ctx context.Context, id uuid.UUID, newSizeGB int64) (*Volume, error)
}

func (f *fakeStore) InsertBackend(_ context.Context, b Backend) (*Backend, error) {
	b.ID = uuid.New()
	f.backends = append(f.backends, b)
	return &b, nil
}
func (f *fakeStore) GetBackend(_ context.Context, id uuid.UUID) (*Backend, error) {
	for i := range f.backends {
		if f.backends[i].ID == id {
			return &f.backends[i], nil
		}
	}
	return nil, ErrBackendNotFound
}
func (f *fakeStore) ListBackends(_ context.Context) ([]Backend, error) { return f.backends, nil }
func (f *fakeStore) SetBackendState(_ context.Context, id uuid.UUID, state BackendState) error {
	for i := range f.backends {
		if f.backends[i].ID == id {
			f.backends[i].State = state
			return nil
		}
	}
	return ErrBackendNotFound
}
func (f *fakeStore) ListActiveBackendsForAZ(ctx context.Context, azID uuid.UUID) ([]Backend, error) {
	if f.listActiveBackendsForAZFn != nil {
		return f.listActiveBackendsForAZFn(ctx, azID)
	}
	var out []Backend
	for _, b := range f.backends {
		if b.State == BackendStateActive {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeStore) InsertVolumeType(_ context.Context, vt VolumeType) (*VolumeType, error) {
	vt.ID = uuid.New()
	f.volumeTypes = append(f.volumeTypes, vt)
	return &vt, nil
}
func (f *fakeStore) GetVolumeType(_ context.Context, id uuid.UUID) (*VolumeType, error) {
	for i := range f.volumeTypes {
		if f.volumeTypes[i].ID == id {
			return &f.volumeTypes[i], nil
		}
	}
	return nil, ErrVolumeTypeNotFound
}
func (f *fakeStore) ListVolumeTypes(_ context.Context) ([]VolumeType, error) {
	return f.volumeTypes, nil
}
func (f *fakeStore) InsertVolume(ctx context.Context, v Volume) (*Volume, error) {
	if f.insertVolumeFn != nil {
		return f.insertVolumeFn(ctx, v)
	}
	v.ID = uuid.New()
	v.State = VolumeStateCreating
	f.volumes = append(f.volumes, v)
	return &v, nil
}
func (f *fakeStore) GetVolume(ctx context.Context, id uuid.UUID) (*Volume, error) {
	if f.getVolumeFn != nil {
		return f.getVolumeFn(ctx, id)
	}
	for i := range f.volumes {
		if f.volumes[i].ID == id {
			return &f.volumes[i], nil
		}
	}
	return nil, ErrVolumeNotFound
}
func (f *fakeStore) ListVolumesByTenant(_ context.Context, tenantID uuid.UUID) ([]Volume, error) {
	var out []Volume
	for _, v := range f.volumes {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	return out, nil
}
func (f *fakeStore) ListVolumesByBackend(_ context.Context, backendID uuid.UUID) ([]Volume, error) {
	var out []Volume
	for _, v := range f.volumes {
		if v.BackendID != nil && *v.BackendID == backendID {
			out = append(out, v)
		}
	}
	return out, nil
}
func (f *fakeStore) SetVolumeState(ctx context.Context, id uuid.UUID, state VolumeState) error {
	if f.setVolumeStateFn != nil {
		return f.setVolumeStateFn(ctx, id, state)
	}
	for i := range f.volumes {
		if f.volumes[i].ID == id {
			f.volumes[i].State = state
			return nil
		}
	}
	return nil
}
func (f *fakeStore) SetVolumeExport(_ context.Context, id, _ uuid.UUID, _ json.RawMessage) error {
	for i := range f.volumes {
		if f.volumes[i].ID == id {
			f.volumes[i].State = VolumeStateInUse
			return nil
		}
	}
	return ErrVolumeNotFound
}
func (f *fakeStore) ClearVolumeExport(_ context.Context, id uuid.UUID) error {
	for i := range f.volumes {
		if f.volumes[i].ID == id {
			f.volumes[i].State = VolumeStateAvailable
			f.volumes[i].ExportedHostID = nil
			f.volumes[i].ExportInfo = nil
			return nil
		}
	}
	return ErrVolumeNotFound
}
func (f *fakeStore) ResizeVolume(ctx context.Context, id uuid.UUID, newSizeGB int64) (*Volume, error) {
	if f.resizeVolumeFn != nil {
		return f.resizeVolumeFn(ctx, id, newSizeGB)
	}
	for i := range f.volumes {
		if f.volumes[i].ID == id {
			f.volumes[i].SizeGB = newSizeGB
			v := f.volumes[i]
			return &v, nil
		}
	}
	return nil, ErrVolumeNotFound
}
func (f *fakeStore) DeleteVolume(_ context.Context, id uuid.UUID) error {
	for i, v := range f.volumes {
		if v.ID == id {
			f.volumes = append(f.volumes[:i], f.volumes[i+1:]...)
			return nil
		}
	}
	return ErrVolumeNotFound
}
func (f *fakeStore) GetHostStorageProperties(_ context.Context, _ uuid.UUID) (map[string]string, error) {
	return nil, nil
}
func (f *fakeStore) GetHostDataIPs(_ context.Context, _ uuid.UUID) ([]string, error) {
	return nil, nil
}

// --- fakeDriver ---

type fakeDriver struct{}

func (fakeDriver) CreateVolume(_ context.Context, spec DriverVolumeSpec) (*DriverVolume, error) {
	return &DriverVolume{VolumeID: spec.VolumeID, SizeGB: spec.SizeGB}, nil
}
func (fakeDriver) DeleteVolume(_ context.Context, _ string) error                       { return nil }
func (fakeDriver) ResizeVolume(_ context.Context, _ string, _ int64) error              { return nil }
func (fakeDriver) ExportVolume(_ context.Context, _ string, _ HostInfo) (*ExportInfo, error) {
	return &ExportInfo{Protocol: "fake"}, nil
}
func (fakeDriver) UnexportVolume(_ context.Context, _ string, _ HostInfo) error { return nil }
func (fakeDriver) Capabilities() DriverCapabilities                              { return DriverCapabilities{} }

// --- helpers ---

func mkBackend(caps []string) Backend {
	raw, _ := json.Marshal(caps)
	return Backend{
		ID:           uuid.New(),
		Driver:       "fake",
		State:        BackendStateActive,
		Capabilities: json.RawMessage(raw),
	}
}

func newTestService(store storageStore) *serviceImpl {
	return &serviceImpl{
		store: store,
		drivers: DriverRegistry{
			"fake": func(_ string, _ string, _ map[string]any) Driver { return fakeDriver{} },
		},
		logger: slog.Default(),
	}
}

// --- TestBackendMeetsCaps ---

func TestBackendMeetsCaps(t *testing.T) {
	mkB := func(caps []string) *Backend {
		raw, _ := json.Marshal(caps)
		return &Backend{Capabilities: json.RawMessage(raw)}
	}

	tests := []struct {
		name     string
		backend  []string
		required []string
		want     bool
	}{
		{
			name:     "no requirements always matches",
			backend:  []string{"ssd"},
			required: nil,
			want:     true,
		},
		{
			name:     "empty requirements always matches",
			backend:  []string{"ssd"},
			required: []string{},
			want:     true,
		},
		{
			name:     "backend meets single requirement",
			backend:  []string{"ssd", "encryption"},
			required: []string{"ssd"},
			want:     true,
		},
		{
			name:     "backend meets all requirements",
			backend:  []string{"ssd", "encryption", "replication"},
			required: []string{"ssd", "encryption"},
			want:     true,
		},
		{
			name:     "backend missing required capability",
			backend:  []string{"hdd"},
			required: []string{"ssd"},
			want:     false,
		},
		{
			name:     "backend meets some but not all requirements",
			backend:  []string{"ssd"},
			required: []string{"ssd", "encryption"},
			want:     false,
		},
		{
			name:     "empty backend cannot meet requirements",
			backend:  []string{},
			required: []string{"ssd"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := mkB(tt.backend)
			got := backendMeetsCaps(b, tt.required)
			if got != tt.want {
				t.Errorf("backendMeetsCaps(%v, %v) = %v, want %v", tt.backend, tt.required, got, tt.want)
			}
		})
	}
}

// --- TestCreateVolume ---

func TestCreateVolume_PicksFirstMatchingBackend(t *testing.T) {
	b1 := mkBackend([]string{"hdd"})          // no ssd → skipped
	b2 := mkBackend([]string{"ssd"})          // matches
	b3 := mkBackend([]string{"ssd", "extra"}) // also matches, but b2 comes first

	caps, _ := json.Marshal([]string{"ssd"})
	vtID := uuid.New()
	store := &fakeStore{
		backends:    []Backend{b1, b2, b3},
		volumeTypes: []VolumeType{{ID: vtID, RequiredCapabilities: json.RawMessage(caps)}},
	}
	svc := newTestService(store)

	v, err := svc.CreateVolume(context.Background(), CreateVolumeSpec{
		TenantID:     uuid.New(),
		Name:         "vol1",
		SizeGB:       10,
		VolumeTypeID: &vtID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *v.BackendID != b2.ID {
		t.Errorf("expected backend %s, got %s", b2.ID, *v.BackendID)
	}
}

func TestCreateVolume_SkipsDrainingBackend(t *testing.T) {
	b1 := mkBackend([]string{"ssd"})
	b1.State = BackendStateDraining
	b2 := mkBackend([]string{"ssd"})

	store := &fakeStore{backends: []Backend{b1, b2}}
	svc := newTestService(store)

	v, err := svc.CreateVolume(context.Background(), CreateVolumeSpec{
		TenantID: uuid.New(), Name: "vol1", SizeGB: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *v.BackendID != b2.ID {
		t.Errorf("expected backend %s, got %s", b2.ID, *v.BackendID)
	}
}

func TestCreateVolume_NoMatchingBackend(t *testing.T) {
	b := mkBackend([]string{"hdd"})
	store := &fakeStore{backends: []Backend{b}}
	svc := newTestService(store)

	caps, _ := json.Marshal([]string{"ssd"})
	vtID := uuid.New()
	store.volumeTypes = []VolumeType{
		{ID: vtID, RequiredCapabilities: json.RawMessage(caps)},
	}

	_, err := svc.CreateVolume(context.Background(), CreateVolumeSpec{
		TenantID: uuid.New(), Name: "vol1", SizeGB: 10, VolumeTypeID: &vtID,
	})
	if err == nil {
		t.Fatal("expected ErrNoMatchingBackend, got nil")
	}
	if !isErrNoMatchingBackend(err) {
		t.Errorf("expected ErrNoMatchingBackend, got %v", err)
	}
}

func TestCreateVolume_UsesAZFilter(t *testing.T) {
	azID := uuid.New()
	azBackend := mkBackend([]string{"ssd"})
	globalBackend := mkBackend([]string{"ssd"})

	azFilterCalled := false
	store := &fakeStore{
		backends: []Backend{globalBackend}, // ListBackends would return this
		listActiveBackendsForAZFn: func(_ context.Context, id uuid.UUID) ([]Backend, error) {
			azFilterCalled = true
			if id != azID {
				t.Errorf("ListActiveBackendsForAZ called with wrong AZ: %s", id)
			}
			return []Backend{azBackend}, nil
		},
	}
	svc := newTestService(store)

	v, err := svc.CreateVolume(context.Background(), CreateVolumeSpec{
		TenantID: uuid.New(), Name: "vol1", SizeGB: 10, AZID: &azID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !azFilterCalled {
		t.Error("ListActiveBackendsForAZ was not called when AZID is set")
	}
	if *v.BackendID != azBackend.ID {
		t.Errorf("expected AZ backend %s, got %s", azBackend.ID, *v.BackendID)
	}
}

// isErrNoMatchingBackend checks whether the error chain contains ErrNoMatchingBackend.
func isErrNoMatchingBackend(err error) bool {
	for err != nil {
		if err == ErrNoMatchingBackend {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

// --- TestResizeVolume ---

func TestResizeVolume_Validation(t *testing.T) {
	tenantID := uuid.New()
	volID := uuid.New()
	backendID := uuid.New()

	existingVol := Volume{
		ID:        volID,
		TenantID:  tenantID,
		SizeGB:    50,
		State:     VolumeStateAvailable,
		BackendID: &backendID,
	}

	tests := []struct {
		name      string
		newSizeGB int64
		wantErr   bool
		errContains string
	}{
		{
			name:        "shrink is rejected",
			newSizeGB:   30,
			wantErr:     true,
			errContains: "must be larger than current",
		},
		{
			name:        "same size is rejected",
			newSizeGB:   50,
			wantErr:     true,
			errContains: "must be larger than current",
		},
		{
			name:      "larger size is accepted",
			newSizeGB: 100,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				volumes:  []Volume{existingVol},
				backends: []Backend{{ID: backendID, Driver: "fake", State: BackendStateActive}},
			}
			svc := newTestService(store)

			_, err := svc.ResizeVolume(context.Background(), tenantID, volID, tt.newSizeGB)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" {
					if !containsString(err.Error(), tt.errContains) {
						t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestResizeVolume_InUseRejected(t *testing.T) {
	tenantID := uuid.New()
	volID := uuid.New()

	store := &fakeStore{
		volumes: []Volume{
			{ID: volID, TenantID: tenantID, SizeGB: 50, State: VolumeStateInUse},
		},
	}
	svc := newTestService(store)

	_, err := svc.ResizeVolume(context.Background(), tenantID, volID, 100)
	if err == nil {
		t.Fatal("expected error for in-use volume, got nil")
	}
}

func TestResizeVolume_TenantIsolation(t *testing.T) {
	ownerTenant := uuid.New()
	otherTenant := uuid.New()
	volID := uuid.New()

	store := &fakeStore{
		volumes: []Volume{
			{ID: volID, TenantID: ownerTenant, SizeGB: 50, State: VolumeStateAvailable},
		},
	}
	svc := newTestService(store)

	_, err := svc.ResizeVolume(context.Background(), otherTenant, volID, 100)
	if err == nil {
		t.Fatal("expected error when accessing another tenant's volume, got nil")
	}
}

// containsString returns true if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
