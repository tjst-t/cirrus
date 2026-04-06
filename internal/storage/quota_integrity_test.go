package storage

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/quota"
)

// --- fakeQuotaSvc ---

type fakeQuotaAction struct {
	action  string // "reserve", "commit", "release", "decommit"
	resType quota.ResourceType
	resID   uuid.UUID
}

type fakeQuotaSvc struct {
	actions []fakeQuotaAction
	// reserveErr makes Reserve return an error.
	reserveErr error
}

func (q *fakeQuotaSvc) Check(_ context.Context, _ uuid.UUID, _ quota.ResourceDelta) error {
	return nil
}

func (q *fakeQuotaSvc) Reserve(_ context.Context, _ uuid.UUID, rt quota.ResourceType, id uuid.UUID, _ quota.ResourceDelta) error {
	if q.reserveErr != nil {
		return q.reserveErr
	}
	q.actions = append(q.actions, fakeQuotaAction{"reserve", rt, id})
	return nil
}

func (q *fakeQuotaSvc) Commit(_ context.Context, rt quota.ResourceType, id uuid.UUID) error {
	q.actions = append(q.actions, fakeQuotaAction{"commit", rt, id})
	return nil
}

func (q *fakeQuotaSvc) Release(_ context.Context, rt quota.ResourceType, id uuid.UUID) error {
	q.actions = append(q.actions, fakeQuotaAction{"release", rt, id})
	return nil
}

func (q *fakeQuotaSvc) Decommit(_ context.Context, _ uuid.UUID, _ quota.ResourceDelta) error {
	q.actions = append(q.actions, fakeQuotaAction{"decommit", quota.ResourceTypeVolume, uuid.Nil})
	return nil
}

func (q *fakeQuotaSvc) GetUsage(_ context.Context, _ uuid.UUID) (*quota.Usage, error) {
	return &quota.Usage{}, nil
}

func (q *fakeQuotaSvc) SetTenantLimits(_ context.Context, _ uuid.UUID, _ quota.Limits) error {
	return nil
}

func (q *fakeQuotaSvc) GetTenantLimits(_ context.Context, _ uuid.UUID) (*quota.Limits, error) {
	return &quota.Limits{}, nil
}

func (q *fakeQuotaSvc) SetOrgLimits(_ context.Context, _ uuid.UUID, _ quota.Limits) error {
	return nil
}

func (q *fakeQuotaSvc) GetOrgLimits(_ context.Context, _ uuid.UUID) (*quota.Limits, error) {
	return &quota.Limits{}, nil
}

// hasAction checks whether the fake quota service recorded a specific action.
func (q *fakeQuotaSvc) hasAction(action string, rt quota.ResourceType) bool {
	for _, a := range q.actions {
		if a.action == action && a.resType == rt {
			return true
		}
	}
	return false
}

// --- errDriver ---

// errDriver is a Driver whose CreateVolume always returns the configured error.
type errDriver struct {
	createErr error
}

func (d errDriver) CreateVolume(_ context.Context, _ DriverVolumeSpec) (*DriverVolume, error) {
	return nil, d.createErr
}
func (d errDriver) DeleteVolume(_ context.Context, _ string) error { return nil }
func (d errDriver) ResizeVolume(_ context.Context, _ string, _ int64) error {
	return nil
}
func (d errDriver) ExportVolume(_ context.Context, _ string, _ HostInfo) (*ExportInfo, error) {
	return &ExportInfo{Protocol: "fake"}, nil
}
func (d errDriver) UnexportVolume(_ context.Context, _ string, _ HostInfo) error { return nil }
func (d errDriver) Capabilities() DriverCapabilities                              { return DriverCapabilities{} }

// --- helpers ---

// newQuotaTestService builds a serviceImpl wired with the given quota service.
// The "fake" driver factory uses fakeDriver (success path).
func newQuotaTestService(store storageStore, qs quota.Service) *serviceImpl {
	return &serviceImpl{
		store: store,
		drivers: DriverRegistry{
			"fake": func(_ string, _ string, _ map[string]any) Driver { return fakeDriver{} },
		},
		quotaSvc: qs,
		logger:   slog.Default(),
	}
}

// newErrDriverTestService builds a serviceImpl whose "fake" driver always
// returns createErr from CreateVolume.
func newErrDriverTestService(store storageStore, qs quota.Service, createErr error) *serviceImpl {
	return &serviceImpl{
		store: store,
		drivers: DriverRegistry{
			"fake": func(_ string, _ string, _ map[string]any) Driver {
				return errDriver{createErr: createErr}
			},
		},
		quotaSvc: qs,
		logger:   slog.Default(),
	}
}

// --- Tests ---

// TestVolumeQuotaIntegrity_CreateSuccess verifies that on a successful volume
// creation: Reserve is called before driver.CreateVolume, Commit is called
// after success, and Release is NOT called.
func TestVolumeQuotaIntegrity_CreateSuccess(t *testing.T) {
	quotaSvc := &fakeQuotaSvc{}
	tenantID := uuid.New()
	backend := mkBackend(nil)

	store := &fakeStore{backends: []Backend{backend}}
	svc := newQuotaTestService(store, quotaSvc)

	v, err := svc.syncCreateVolume(context.Background(), CreateVolumeSpec{
		TenantID: tenantID,
		Name:     "vol-success",
		SizeGB:   20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == nil {
		t.Fatal("expected volume, got nil")
	}

	if !quotaSvc.hasAction("reserve", quota.ResourceTypeVolume) {
		t.Error("expected quota Reserve to be called")
	}
	if !quotaSvc.hasAction("commit", quota.ResourceTypeVolume) {
		t.Error("expected quota Commit to be called on success")
	}
	if quotaSvc.hasAction("release", quota.ResourceTypeVolume) {
		t.Error("expected quota Release NOT to be called on success")
	}

	// Verify Reserve happened before Commit in the recorded action sequence.
	reserveIdx, commitIdx := -1, -1
	for i, a := range quotaSvc.actions {
		if a.action == "reserve" {
			reserveIdx = i
		}
		if a.action == "commit" {
			commitIdx = i
		}
	}
	if reserveIdx < 0 || commitIdx < 0 {
		t.Fatal("reserve or commit action missing from recorded actions")
	}
	if reserveIdx >= commitIdx {
		t.Errorf("expected Reserve (idx %d) before Commit (idx %d)", reserveIdx, commitIdx)
	}
}

// TestVolumeQuotaIntegrity_CreateFailure verifies that when driver.CreateVolume
// fails, Release is called and Commit is NOT called.
func TestVolumeQuotaIntegrity_CreateFailure(t *testing.T) {
	quotaSvc := &fakeQuotaSvc{}
	tenantID := uuid.New()
	backend := mkBackend(nil)

	store := &fakeStore{backends: []Backend{backend}}
	svc := newErrDriverTestService(store, quotaSvc, errors.New("backend unavailable"))

	_, err := svc.syncCreateVolume(context.Background(), CreateVolumeSpec{
		TenantID: tenantID,
		Name:     "vol-fail",
		SizeGB:   20,
	})
	if err == nil {
		t.Fatal("expected error from driver, got nil")
	}

	if !quotaSvc.hasAction("reserve", quota.ResourceTypeVolume) {
		t.Error("expected quota Reserve to be called before driver attempt")
	}
	if !quotaSvc.hasAction("release", quota.ResourceTypeVolume) {
		t.Error("expected quota Release to be called after driver failure")
	}
	if quotaSvc.hasAction("commit", quota.ResourceTypeVolume) {
		t.Error("expected quota Commit NOT to be called after driver failure")
	}
}

// TestVolumeQuotaIntegrity_DeleteDecommit verifies that Decommit is called on
// a successful volume deletion.
func TestVolumeQuotaIntegrity_DeleteDecommit(t *testing.T) {
	quotaSvc := &fakeQuotaSvc{}
	tenantID := uuid.New()
	volID := uuid.New()
	backend := mkBackend(nil)

	store := &fakeStore{
		backends: []Backend{backend},
		volumes: []Volume{
			{
				ID:        volID,
				TenantID:  tenantID,
				SizeGB:    20,
				State:     VolumeStateAvailable,
				BackendID: &backend.ID,
			},
		},
	}
	svc := newQuotaTestService(store, quotaSvc)

	err := svc.syncDeleteVolume(context.Background(), tenantID, volID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !quotaSvc.hasAction("decommit", quota.ResourceTypeVolume) {
		t.Error("expected quota Decommit to be called after successful delete")
	}
	// Commit and Release should not be called on the delete path.
	if quotaSvc.hasAction("commit", quota.ResourceTypeVolume) {
		t.Error("expected quota Commit NOT to be called on delete path")
	}
	if quotaSvc.hasAction("release", quota.ResourceTypeVolume) {
		t.Error("expected quota Release NOT to be called on delete path")
	}
}
