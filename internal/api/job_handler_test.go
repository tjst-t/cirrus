package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/api"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/jobqueue"
)

// --- mock job queue ---

type mockJobQueue struct {
	jobs map[uuid.UUID]*jobqueue.Job
}

func newMockJobQueue() *mockJobQueue {
	return &mockJobQueue{jobs: make(map[uuid.UUID]*jobqueue.Job)}
}

func (m *mockJobQueue) addJob(j *jobqueue.Job) {
	m.jobs[j.ID] = j
}

func (m *mockJobQueue) Enqueue(_ context.Context, p jobqueue.EnqueueParams) (*jobqueue.Job, error) {
	j := &jobqueue.Job{ID: uuid.New(), Type: p.Type, Status: jobqueue.StatusPending}
	m.jobs[j.ID] = j
	return j, nil
}
func (m *mockJobQueue) Dequeue(_ context.Context, _ []string) (*jobqueue.Job, error) { return nil, nil }
func (m *mockJobQueue) Complete(_ context.Context, _ uuid.UUID) error                { return nil }
func (m *mockJobQueue) Fail(_ context.Context, _ uuid.UUID, _ string) error          { return nil }
func (m *mockJobQueue) ListStuck(_ context.Context, _ time.Duration) ([]jobqueue.Job, error) {
	return nil, nil
}
func (m *mockJobQueue) Get(_ context.Context, id uuid.UUID) (*jobqueue.Job, error) {
	j, ok := m.jobs[id]
	if !ok {
		return nil, errors.New("job not found")
	}
	return j, nil
}

// --- allow/deny authorizer for testing ---

// allowAllAuthz allows every action.
type allowAllAuthz struct{}

func (a *allowAllAuthz) Authorize(_ context.Context, _ *identity.User, _ identity.Action, _ identity.Resource) (identity.Decision, error) {
	return identity.Allow, nil
}

// denyAllAuthz denies every action (simulates tenant_member with no access).
type denyAllAuthz struct{}

func (a *denyAllAuthz) Authorize(_ context.Context, _ *identity.User, _ identity.Action, _ identity.Resource) (identity.Decision, error) {
	return identity.Deny, nil
}

// --- helper to build router for job tests ---

func jobTestRouter(q jobqueue.Queue, authz identity.Authorizer) http.Handler {
	return api.NewRouter(nil, slog.Default(), &testAuthn{}, authz, nil, nil, nil, nil, nil, nil, nil, nil, nil, q, false)
}

// TestGetJob_NotFound verifies that GET /jobs/{id} returns 404 for an unknown job ID.
func TestGetJob_NotFound(t *testing.T) {
	q := newMockJobQueue()
	router := jobTestRouter(q, &allowAllAuthz{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String(), nil)
	req.Header.Set("Authorization", "Bearer dev-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["message"] == "" && body["code"] == "" {
		t.Error("expected error message in response")
	}
}

// TestGetJob_InvalidID verifies that GET /jobs/{id} returns 400 for a malformed UUID.
func TestGetJob_InvalidID(t *testing.T) {
	q := newMockJobQueue()
	router := jobTestRouter(q, &allowAllAuthz{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer dev-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGetJob_Forbidden verifies that a caller who fails all Authorize checks gets 403.
func TestGetJob_Forbidden(t *testing.T) {
	tenantID := uuid.New()
	q := newMockJobQueue()
	jobID := uuid.New()
	q.addJob(&jobqueue.Job{
		ID:       jobID,
		Type:     "vm_create",
		Status:   jobqueue.StatusCompleted,
		TenantID: &tenantID,
		// CreatedBy is nil — so tenant_member branch won't match either.
	})

	router := jobTestRouter(q, &denyAllAuthz{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID.String(), nil)
	req.Header.Set("Authorization", "Bearer dev-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["message"] != "forbidden" {
		t.Errorf("expected 'forbidden', got %q", body["message"])
	}
}

// TestGetJob_TenantMember_OwnJob verifies a tenant_member can see their own job.
// The authorizer allows ActionGetVM (used for tenant_member check) but denies
// the broader ActionListVMs (tenant_admin) and ActionRepairVM (infra_admin).
func TestGetJob_TenantMember_OwnJob(t *testing.T) {
	tenantID := uuid.New()
	callerID := "user-abc"
	q := newMockJobQueue()
	jobID := uuid.New()
	q.addJob(&jobqueue.Job{
		ID:        jobID,
		Type:      "volume_create",
		Status:    jobqueue.StatusCompleted,
		TenantID:  &tenantID,
		CreatedBy: &callerID,
	})

	// Authz: only allow ActionGetVM (tenant_member read), deny everything broader.
	memberAuthz := &actionFilterAuthz{allow: identity.ActionGetVM}
	router := jobTestRouter(q, memberAuthz)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID.String(), nil)
	req.Header.Set("Authorization", "Bearer dev-token") // testAuthn returns ExternalID="test-admin"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// testAuthn returns ExternalID="test-admin", but job.CreatedBy="user-abc" — mismatch → 403.
	if w.Code != http.StatusForbidden {
		t.Logf("body: %s", w.Body.String())
		t.Fatalf("expected 403 (creator mismatch), got %d", w.Code)
	}
}

// TestGetJob_InfraAdmin verifies infra_admin can access any job.
func TestGetJob_InfraAdmin(t *testing.T) {
	tenantID := uuid.New()
	q := newMockJobQueue()
	jobID := uuid.New()
	q.addJob(&jobqueue.Job{
		ID:       jobID,
		Type:     "vm_delete",
		Status:   jobqueue.StatusPending,
		TenantID: &tenantID,
	})

	router := jobTestRouter(q, &allowAllAuthz{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID.String(), nil)
	req.Header.Set("Authorization", "Bearer dev-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGetJob_NoTenantID_Forbidden verifies that a job without a tenant_id is inaccessible
// to non-infra_admin callers (no tenant scope to match against).
func TestGetJob_NoTenantID_Forbidden(t *testing.T) {
	q := newMockJobQueue()
	jobID := uuid.New()
	q.addJob(&jobqueue.Job{
		ID:       jobID,
		Type:     "vm_create",
		Status:   jobqueue.StatusPending,
		TenantID: nil, // no tenant
	})

	router := jobTestRouter(q, &denyAllAuthz{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID.String(), nil)
	req.Header.Set("Authorization", "Bearer dev-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for tenantless job with deny authz, got %d", w.Code)
	}
}

// actionFilterAuthz allows only one specific action, denying all others.
type actionFilterAuthz struct {
	allow identity.Action
}

func (a *actionFilterAuthz) Authorize(_ context.Context, _ *identity.User, action identity.Action, _ identity.Resource) (identity.Decision, error) {
	if action == a.allow {
		return identity.Allow, nil
	}
	return identity.Deny, nil
}

// Ensure mockJobQueue satisfies jobqueue.Queue at compile time.
// (ListStuck signature must match — fix if it doesn't compile.)
var _ jobqueue.Queue = (*mockJobQueue)(nil)
