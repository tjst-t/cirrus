package state

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestStore() *Store {
	return NewStore(nil)
}

func addTestBackend(t *testing.T, s *Store, id string, capacity int64, ratio float64) {
	t.Helper()
	err := s.AddBackend(context.Background(), Backend{
		BackendID:          id,
		TotalCapacityGB:    capacity,
		TotalIOPS:          500000,
		Capabilities:       []string{"ssd", "snapshot"},
		OverprovisionRatio: ratio,
	})
	if err != nil {
		t.Fatalf("failed to add backend: %v", err)
	}
}

func TestAddBackend(t *testing.T) {
	tests := []struct {
		name    string
		backend Backend
		wantErr error
	}{
		{
			name: "success",
			backend: Backend{
				BackendID:          "b1",
				TotalCapacityGB:    1000,
				TotalIOPS:          100000,
				Capabilities:       []string{"ssd"},
				OverprovisionRatio: 2.0,
			},
			wantErr: nil,
		},
		{
			name:    "empty id",
			backend: Backend{},
			wantErr: ErrEmptyBackendID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			err := s.AddBackend(context.Background(), tt.backend)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAddBackendDuplicate(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)

	err := s.AddBackend(context.Background(), Backend{BackendID: "b1", TotalCapacityGB: 500})
	if !errors.Is(err, ErrBackendExists) {
		t.Errorf("got error %v, want %v", err, ErrBackendExists)
	}
}

func TestListBackends(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	addTestBackend(t, s, "b2", 2000, 1.5)

	list := s.ListBackends(context.Background())
	if len(list) != 2 {
		t.Errorf("got %d backends, want 2", len(list))
	}
}

func TestSetBackendState(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		state   BackendState
		wantErr error
	}{
		{name: "draining", id: "b1", state: BackendDraining, wantErr: nil},
		{name: "read_only", id: "b1", state: BackendReadOnly, wantErr: nil},
		{name: "not found", id: "missing", state: BackendDraining, wantErr: ErrBackendNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			addTestBackend(t, s, "b1", 1000, 2.0)

			err := s.SetBackendState(context.Background(), tt.id, tt.state)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			b, _ := s.GetBackend(context.Background(), tt.id)
			if b.State != tt.state {
				t.Errorf("got state %s, want %s", b.State, tt.state)
			}
		})
	}
}

func TestCreateVolume(t *testing.T) {
	tests := []struct {
		name    string
		vol     Volume
		wantErr error
	}{
		{
			name: "thin provisioned",
			vol: Volume{
				VolumeID:        "vol-1",
				BackendID:       "b1",
				SizeGB:          100,
				ThinProvisioned: true,
			},
			wantErr: nil,
		},
		{
			name: "thick provisioned",
			vol: Volume{
				VolumeID:        "vol-2",
				BackendID:       "b1",
				SizeGB:          100,
				ThinProvisioned: false,
			},
			wantErr: nil,
		},
		{
			name:    "empty id",
			vol:     Volume{BackendID: "b1", SizeGB: 10},
			wantErr: ErrEmptyVolumeID,
		},
		{
			name:    "backend not found",
			vol:     Volume{VolumeID: "vol-x", BackendID: "missing", SizeGB: 10},
			wantErr: ErrBackendNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			addTestBackend(t, s, "b1", 1000, 2.0)

			v, err := s.CreateVolume(context.Background(), tt.vol)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.State != VolumeAvailable {
				t.Errorf("got state %s, want %s", v.State, VolumeAvailable)
			}
			if v.ConsumedGB != 0 {
				t.Errorf("got consumed_gb %d, want 0", v.ConsumedGB)
			}
			if v.CreatedAt.IsZero() {
				t.Error("created_at should not be zero")
			}
		})
	}
}

func TestCreateVolumeDuplicate(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)

	_, err := s.CreateVolume(context.Background(), Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateVolume(context.Background(), Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true})
	if !errors.Is(err, ErrVolumeExists) {
		t.Errorf("got error %v, want %v", err, ErrVolumeExists)
	}
}

func TestCapacityTrackingThin(t *testing.T) {
	s := newTestStore()
	// capacity=100, ratio=2.0 => max allocated = 200
	addTestBackend(t, s, "b1", 100, 2.0)

	ctx := context.Background()

	// Should succeed: 150 <= 200
	_, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 150, ThinProvisioned: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, _ := s.GetBackend(ctx, "b1")
	if b.AllocatedCapacityGB != 150 {
		t.Errorf("allocated = %d, want 150", b.AllocatedCapacityGB)
	}
	if b.UsedCapacityGB != 0 {
		t.Errorf("used = %d, want 0", b.UsedCapacityGB)
	}

	// Should fail: 150 + 60 = 210 > 200
	_, err = s.CreateVolume(ctx, Volume{VolumeID: "v2", BackendID: "b1", SizeGB: 60, ThinProvisioned: true})
	if !errors.Is(err, ErrInsufficientCapacity) {
		t.Errorf("got error %v, want %v", err, ErrInsufficientCapacity)
	}
}

func TestCapacityTrackingThick(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 100, 2.0)

	ctx := context.Background()

	_, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 80, ThinProvisioned: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, _ := s.GetBackend(ctx, "b1")
	if b.UsedCapacityGB != 80 {
		t.Errorf("used = %d, want 80", b.UsedCapacityGB)
	}
	if b.AllocatedCapacityGB != 80 {
		t.Errorf("allocated = %d, want 80", b.AllocatedCapacityGB)
	}

	// Should fail: 80 + 30 = 110 > 100
	_, err = s.CreateVolume(ctx, Volume{VolumeID: "v2", BackendID: "b1", SizeGB: 30, ThinProvisioned: false})
	if !errors.Is(err, ErrInsufficientCapacity) {
		t.Errorf("got error %v, want %v", err, ErrInsufficientCapacity)
	}
}

func TestCreateVolumeOnNonActiveBackend(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	_ = s.SetBackendState(context.Background(), "b1", BackendDraining)

	_, err := s.CreateVolume(context.Background(), Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true})
	if !errors.Is(err, ErrBackendNotActive) {
		t.Errorf("got error %v, want %v", err, ErrBackendNotActive)
	}
}

func TestDeleteVolume(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(s *Store)
		volumeID  string
		wantErr   error
	}{
		{
			name: "success",
			setup: func(s *Store) {
				_ = s.AddBackend(context.Background(), Backend{
					BackendID: "b1", TotalCapacityGB: 1000, TotalIOPS: 500000,
					Capabilities: []string{"ssd", "snapshot"}, OverprovisionRatio: 2.0,
				})
				_, _ = s.CreateVolume(context.Background(), Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 50, ThinProvisioned: true})
			},
			volumeID: "v1",
			wantErr:  nil,
		},
		{
			name:     "not found",
			setup:    func(s *Store) {},
			volumeID: "missing",
			wantErr:  ErrVolumeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			tt.setup(s)
			err := s.DeleteVolume(context.Background(), tt.volumeID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDeleteVolumeInUse(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 50, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExportVolume(ctx, "v1", "host-1", "rbd"); err != nil {
		t.Fatal(err)
	}

	err := s.DeleteVolume(ctx, "v1")
	if !errors.Is(err, ErrVolumeInUse) {
		t.Errorf("got error %v, want %v", err, ErrVolumeInUse)
	}
}

func TestDeleteVolumeWithSnapshots(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 50, ThinProvisioned: true, Snapshots: []string{"snap-1"}}); err != nil {
		t.Fatal(err)
	}

	err := s.DeleteVolume(ctx, "v1")
	if !errors.Is(err, ErrVolumeHasSnapshots) {
		t.Errorf("got error %v, want %v", err, ErrVolumeHasSnapshots)
	}
}

func TestDeleteVolumeCapacityReturn(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()

	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 50, ThinProvisioned: false}); err != nil {
		t.Fatal(err)
	}

	b, _ := s.GetBackend(ctx, "b1")
	if b.UsedCapacityGB != 50 {
		t.Fatalf("used = %d, want 50", b.UsedCapacityGB)
	}

	if err := s.DeleteVolume(ctx, "v1"); err != nil {
		t.Fatal(err)
	}

	b, _ = s.GetBackend(ctx, "b1")
	if b.UsedCapacityGB != 0 {
		t.Errorf("used after delete = %d, want 0", b.UsedCapacityGB)
	}
	if b.AllocatedCapacityGB != 0 {
		t.Errorf("allocated after delete = %d, want 0", b.AllocatedCapacityGB)
	}
}

func TestExtendVolume(t *testing.T) {
	tests := []struct {
		name      string
		newSizeGB int64
		wantErr   error
	}{
		{name: "success", newSizeGB: 200, wantErr: nil},
		{name: "shrink", newSizeGB: 50, wantErr: ErrShrinkNotAllowed},
		{name: "same size", newSizeGB: 100, wantErr: ErrShrinkNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			addTestBackend(t, s, "b1", 1000, 2.0)
			ctx := context.Background()
			if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}

			v, err := s.ExtendVolume(ctx, "v1", tt.newSizeGB)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.SizeGB != tt.newSizeGB {
				t.Errorf("got size %d, want %d", v.SizeGB, tt.newSizeGB)
			}
		})
	}
}

func TestExportUnexport(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 50, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}

	// Export
	v, err := s.ExportVolume(ctx, "v1", "host-1", "rbd")
	if err != nil {
		t.Fatalf("export error: %v", err)
	}
	if v.State != VolumeInUse {
		t.Errorf("state = %s, want %s", v.State, VolumeInUse)
	}
	if v.ExportInfo == nil || v.ExportInfo.HostID != "host-1" {
		t.Error("export info not set correctly")
	}

	// Double export should fail
	_, err = s.ExportVolume(ctx, "v1", "host-2", "rbd")
	if !errors.Is(err, ErrVolumeAlreadyExported) {
		t.Errorf("got error %v, want %v", err, ErrVolumeAlreadyExported)
	}

	// Unexport
	v, err = s.UnexportVolume(ctx, "v1")
	if err != nil {
		t.Fatalf("unexport error: %v", err)
	}
	if v.State != VolumeAvailable {
		t.Errorf("state = %s, want %s", v.State, VolumeAvailable)
	}
	if v.ExportInfo != nil {
		t.Error("export info should be nil after unexport")
	}

	// Double unexport should fail
	_, err = s.UnexportVolume(ctx, "v1")
	if !errors.Is(err, ErrVolumeNotExported) {
		t.Errorf("got error %v, want %v", err, ErrVolumeNotExported)
	}
}

func TestListVolumesFilter(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	addTestBackend(t, s, "b2", 1000, 2.0)
	ctx := context.Background()

	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v2", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v3", BackendID: "b2", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExportVolume(ctx, "v1", "host-1", "rbd"); err != nil {
		t.Fatal(err)
	}

	// All volumes
	if got := s.ListVolumes(ctx, "", ""); len(got) != 3 {
		t.Errorf("all volumes: got %d, want 3", len(got))
	}

	// Filter by backend
	if got := s.ListVolumes(ctx, "b1", ""); len(got) != 2 {
		t.Errorf("b1 volumes: got %d, want 2", len(got))
	}

	// Filter by state
	if got := s.ListVolumes(ctx, "", VolumeInUse); len(got) != 1 {
		t.Errorf("in_use volumes: got %d, want 1", len(got))
	}
}

func TestGetStats(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()

	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v2", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExportVolume(ctx, "v1", "host-1", "rbd"); err != nil {
		t.Fatal(err)
	}

	stats := s.GetStats(ctx)
	if stats.BackendCount != 1 {
		t.Errorf("backend_count = %d, want 1", stats.BackendCount)
	}
	if stats.VolumeCount != 2 {
		t.Errorf("volume_count = %d, want 2", stats.VolumeCount)
	}
	if stats.ExportCount != 1 {
		t.Errorf("export_count = %d, want 1", stats.ExportCount)
	}
}

func TestReset(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}

	s.Reset(ctx)

	if stats := s.GetStats(ctx); stats.BackendCount != 0 || stats.VolumeCount != 0 {
		t.Errorf("after reset: backends=%d, volumes=%d", stats.BackendCount, stats.VolumeCount)
	}
}

func TestCreateSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		volumeID   string
		snapshotID string
		wantErr    error
	}{
		{name: "success", volumeID: "v1", snapshotID: "snap-001", wantErr: nil},
		{name: "empty snapshot id", volumeID: "v1", snapshotID: "", wantErr: ErrEmptySnapshotID},
		{name: "volume not found", volumeID: "missing", snapshotID: "snap-002", wantErr: ErrVolumeNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			addTestBackend(t, s, "b1", 1000, 2.0)
			ctx := context.Background()
			if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}

			snap, err := s.CreateSnapshot(ctx, tt.volumeID, tt.snapshotID, map[string]string{"desc": "test"})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if snap.SnapshotID != tt.snapshotID {
				t.Errorf("snapshot_id = %s, want %s", snap.SnapshotID, tt.snapshotID)
			}
			if snap.VolumeID != tt.volumeID {
				t.Errorf("volume_id = %s, want %s", snap.VolumeID, tt.volumeID)
			}
			if snap.SizeGB != 100 {
				t.Errorf("size_gb = %d, want 100", snap.SizeGB)
			}
			if snap.State != "available" {
				t.Errorf("state = %s, want available", snap.State)
			}
			if len(snap.ChildClones) != 0 {
				t.Errorf("child_clones = %v, want empty", snap.ChildClones)
			}

			// Verify volume's Snapshots list is updated
			v, _ := s.GetVolume(ctx, "v1")
			if len(v.Snapshots) != 1 || v.Snapshots[0] != tt.snapshotID {
				t.Errorf("volume snapshots = %v, want [%s]", v.Snapshots, tt.snapshotID)
			}
		})
	}
}

func TestCreateSnapshotDuplicate(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}

	_, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil)
	if !errors.Is(err, ErrSnapshotExists) {
		t.Errorf("got error %v, want %v", err, ErrSnapshotExists)
	}
}

func TestListSnapshots(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-002", nil); err != nil {
		t.Fatal(err)
	}

	snaps, err := s.ListSnapshots(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Errorf("got %d snapshots, want 2", len(snaps))
	}

	// Volume not found
	_, err = s.ListSnapshots(ctx, "missing")
	if !errors.Is(err, ErrVolumeNotFound) {
		t.Errorf("got error %v, want %v", err, ErrVolumeNotFound)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}

	// Successful delete
	err := s.DeleteSnapshot(ctx, "snap-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify removed from volume's snapshot list
	v, _ := s.GetVolume(ctx, "v1")
	if len(v.Snapshots) != 0 {
		t.Errorf("volume snapshots = %v, want empty", v.Snapshots)
	}

	// Not found
	err = s.DeleteSnapshot(ctx, "snap-001")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("got error %v, want %v", err, ErrSnapshotNotFound)
	}
}

func TestDeleteSnapshotWithClones(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloneFromSnapshot(ctx, "snap-001", "clone-001", nil); err != nil {
		t.Fatal(err)
	}

	err := s.DeleteSnapshot(ctx, "snap-001")
	if !errors.Is(err, ErrSnapshotHasClones) {
		t.Errorf("got error %v, want %v", err, ErrSnapshotHasClones)
	}
}

func TestCloneFromSnapshot(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}

	clone, err := s.CloneFromSnapshot(ctx, "snap-001", "clone-001", map[string]string{"tenant": "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if clone.VolumeID != "clone-001" {
		t.Errorf("volume_id = %s, want clone-001", clone.VolumeID)
	}
	if clone.ParentSnapshotID != "snap-001" {
		t.Errorf("parent_snapshot_id = %s, want snap-001", clone.ParentSnapshotID)
	}
	if clone.SizeGB != 100 {
		t.Errorf("size_gb = %d, want 100", clone.SizeGB)
	}
	if clone.State != VolumeAvailable {
		t.Errorf("state = %s, want available", clone.State)
	}
	if !clone.ThinProvisioned {
		t.Error("clone should be thin provisioned")
	}
	if clone.BackendID != "b1" {
		t.Errorf("backend_id = %s, want b1", clone.BackendID)
	}

	// Verify snapshot's child_clones updated
	snap, _ := s.GetSnapshot(ctx, "snap-001")
	if len(snap.ChildClones) != 1 || snap.ChildClones[0] != "clone-001" {
		t.Errorf("child_clones = %v, want [clone-001]", snap.ChildClones)
	}

	// Verify capacity tracking
	b, _ := s.GetBackend(ctx, "b1")
	if b.AllocatedCapacityGB != 200 {
		t.Errorf("allocated = %d, want 200 (100 for vol + 100 for clone)", b.AllocatedCapacityGB)
	}
}

func TestCloneFromSnapshotErrors(t *testing.T) {
	tests := []struct {
		name       string
		snapID     string
		volumeID   string
		wantErr    error
	}{
		{name: "snapshot not found", snapID: "missing", volumeID: "clone-1", wantErr: ErrSnapshotNotFound},
		{name: "empty volume id", snapID: "snap-001", volumeID: "", wantErr: ErrEmptyVolumeID},
		{name: "volume exists", snapID: "snap-001", volumeID: "v1", wantErr: ErrVolumeExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			addTestBackend(t, s, "b1", 1000, 2.0)
			ctx := context.Background()
			if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
				t.Fatal(err)
			}

			_, err := s.CloneFromSnapshot(ctx, tt.snapID, tt.volumeID, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCloneCapacityExceeded(t *testing.T) {
	s := newTestStore()
	// Small backend: capacity=100, ratio=1.5 => max=150
	addTestBackend(t, s, "b1", 100, 1.5)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}

	// Clone needs 100 more, total would be 200 > 150
	_, err := s.CloneFromSnapshot(ctx, "snap-001", "clone-001", nil)
	if !errors.Is(err, ErrInsufficientCapacity) {
		t.Errorf("got error %v, want %v", err, ErrInsufficientCapacity)
	}
}

func TestDeleteVolumeWithSnapshotsViaStore(t *testing.T) {
	s := newTestStore()
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()
	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}

	err := s.DeleteVolume(ctx, "v1")
	if !errors.Is(err, ErrVolumeHasSnapshots) {
		t.Errorf("got error %v, want %v", err, ErrVolumeHasSnapshots)
	}
}

func TestStartFlatten(t *testing.T) {
	s := newTestStore()
	// Use very fast flatten for testing
	s.config.FlattenPerGBMs = 1
	addTestBackend(t, s, "b1", 1000, 2.0)
	ctx := context.Background()

	if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 10, ThinProvisioned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, "v1", "snap-001", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloneFromSnapshot(ctx, "snap-001", "clone-001", nil); err != nil {
		t.Fatal(err)
	}

	op, err := s.StartFlatten(ctx, "clone-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op.Type != "flatten" {
		t.Errorf("type = %s, want flatten", op.Type)
	}
	if op.State != OperationRunning {
		t.Errorf("state = %s, want running", op.State)
	}
	if op.VolumeID != "clone-001" {
		t.Errorf("volume_id = %s, want clone-001", op.VolumeID)
	}

	// Wait for flatten to complete (10GB * 1ms/GB = 10ms, with overhead)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, err := s.GetOperation(ctx, op.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State == OperationCompleted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify operation completed
	finalOp, err := s.GetOperation(ctx, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if finalOp.State != OperationCompleted {
		t.Errorf("operation state = %s, want completed", finalOp.State)
	}
	if finalOp.ProgressPercent != 100 {
		t.Errorf("progress = %d, want 100", finalOp.ProgressPercent)
	}

	// Verify volume state after flatten
	vol, err := s.GetVolume(ctx, "clone-001")
	if err != nil {
		t.Fatal(err)
	}
	if vol.ParentSnapshotID != "" {
		t.Errorf("parent_snapshot_id = %s, want empty", vol.ParentSnapshotID)
	}
	if vol.ConsumedGB != vol.SizeGB {
		t.Errorf("consumed_gb = %d, want %d", vol.ConsumedGB, vol.SizeGB)
	}

	// Verify snapshot's child_clones no longer has clone-001
	snap, err := s.GetSnapshot(ctx, "snap-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.ChildClones) != 0 {
		t.Errorf("child_clones = %v, want empty", snap.ChildClones)
	}
}

func TestStartFlattenErrors(t *testing.T) {
	tests := []struct {
		name     string
		volumeID string
		wantErr  error
	}{
		{name: "volume not found", volumeID: "missing", wantErr: ErrVolumeNotFound},
		{name: "no parent snapshot", volumeID: "v1", wantErr: ErrVolumeNoParent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()
			addTestBackend(t, s, "b1", 1000, 2.0)
			ctx := context.Background()
			if _, err := s.CreateVolume(ctx, Volume{VolumeID: "v1", BackendID: "b1", SizeGB: 100, ThinProvisioned: true}); err != nil {
				t.Fatal(err)
			}

			_, err := s.StartFlatten(ctx, tt.volumeID)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetOperationNotFound(t *testing.T) {
	s := newTestStore()
	_, err := s.GetOperation(context.Background(), "missing")
	if !errors.Is(err, ErrOperationNotFound) {
		t.Errorf("got error %v, want %v", err, ErrOperationNotFound)
	}
}
