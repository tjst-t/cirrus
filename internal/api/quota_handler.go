package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/quota"
)

type quotaHandlers struct {
	svc   quota.Service
	authz identity.Authorizer
}

// quotaResponse is the combined limits + usage response body.
type quotaResponse struct {
	Limits *quota.Limits `json:"limits"`
	Usage  *quota.Usage  `json:"usage"`
}

// GET /api/v1/tenants/{tenant_id}/quota
func (h *quotaHandlers) getTenantQuota(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant_id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetQuota, identity.Resource{TenantID: &tenantID})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	limits, err := h.svc.GetTenantLimits(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, quota.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get quota limits"})
		return
	}
	usage, err := h.svc.GetUsage(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get quota usage"})
		return
	}

	writeJSON(w, http.StatusOK, quotaResponse{Limits: limits, Usage: usage})
}

// PUT /api/v1/tenants/{tenant_id}/quota
func (h *quotaHandlers) setTenantQuota(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant_id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionSetQuota, identity.Resource{TenantID: &tenantID})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req quota.Limits
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.svc.SetTenantLimits(r.Context(), tenantID, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set quota limits"})
		return
	}

	limits, err := h.svc.GetTenantLimits(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get updated quota limits"})
		return
	}

	writeJSON(w, http.StatusOK, quotaResponse{Limits: limits})
}

// GET /api/v1/organizations/{org_id}/quota
func (h *quotaHandlers) getOrgQuota(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "org_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetQuota, identity.Resource{OrganizationID: &orgID})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	limits, err := h.svc.GetOrgLimits(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, quota.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org quota limits"})
		return
	}

	writeJSON(w, http.StatusOK, quotaResponse{Limits: limits})
}

// PUT /api/v1/organizations/{org_id}/quota
func (h *quotaHandlers) setOrgQuota(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "org_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionSetQuota, identity.Resource{OrganizationID: &orgID})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req quota.Limits
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.svc.SetOrgLimits(r.Context(), orgID, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set org quota limits"})
		return
	}

	limits, err := h.svc.GetOrgLimits(r.Context(), orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get updated org quota limits"})
		return
	}

	writeJSON(w, http.StatusOK, quotaResponse{Limits: limits})
}

// errQuotaExceeded checks if an error is a quota-exceeded error for HTTP response mapping.
func errQuotaExceeded(err error) bool {
	return errors.Is(err, quota.ErrQuotaExceeded)
}
