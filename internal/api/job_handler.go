package api

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/jobqueue"
)

type jobHandlers struct {
	queue jobqueue.Queue
	authz identity.Authorizer
}

func (h *jobHandlers) getJob(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		writeErrorCode(w, http.StatusUnauthorized, apierror.CodeUnauthorized, "unauthorized", nil)
		return
	}

	jobIDStr := r.PathValue("job_id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid job_id", nil)
		return
	}

	job, err := h.queue.Get(r.Context(), jobID)
	if err != nil {
		// Treat any "not found" or DB error as 404 to avoid leaking job existence.
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "job not found", nil)
		return
	}

	// Authorization: check if the caller is allowed to see this job.
	if !h.authorizeJobAccess(r, user, job) {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// authorizeJobAccess returns true if the caller is allowed to read the given job.
//
// Rules:
//   - infra_admin: can see all jobs
//   - tenant_admin: can see all jobs where job.TenantID matches caller's tenant scope
//   - tenant_member: can see jobs they created (job.CreatedBy == caller ExternalID)
func (h *jobHandlers) authorizeJobAccess(r *http.Request, user *identity.User, job *jobqueue.Job) bool {
	// infra_admin: RepairVM is an infra_admin-only action; use it as a proxy for
	// the global admin check without requiring a tenant resource.
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionRepairVM, identity.Resource{}); decision == identity.Allow {
		return true
	}

	if job.TenantID == nil {
		return false
	}
	res := identity.Resource{TenantID: job.TenantID}

	// tenant_admin can see all jobs in their tenant.
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionListVMs, res); decision == identity.Allow {
		return true
	}

	// tenant_member can see only jobs they personally created.
	if job.CreatedBy != nil && *job.CreatedBy == user.ExternalID {
		if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetVM, res); decision == identity.Allow {
			return true
		}
	}

	return false
}

