package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/identity"
)

type identityHandlers struct {
	svc   identity.Service
	authz identity.Authorizer
}

// --- Organizations ---

func (h *identityHandlers) createOrganization(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateOrganization, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	org, err := h.svc.CreateOrganization(r.Context(), req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

func (h *identityHandlers) listOrganizations(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListOrganizations, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	orgs, err := h.svc.ListOrganizations(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list organizations"})
		return
	}
	if orgs == nil {
		orgs = []identity.Organization{}
	}

	writeJSON(w, http.StatusOK, orgs)
}

func (h *identityHandlers) getOrganization(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "org_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid organization id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetOrganization, identity.Resource{OrganizationID: &id})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	org, err := h.svc.GetOrganization(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get organization"})
		return
	}

	writeJSON(w, http.StatusOK, org)
}

// --- Tenants ---

func (h *identityHandlers) createTenant(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "org_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid organization id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionCreateTenant, identity.Resource{OrganizationID: &orgID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	tenant, err := h.svc.CreateTenant(r.Context(), orgID, req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create tenant"})
		return
	}

	writeJSON(w, http.StatusCreated, tenant)
}

func (h *identityHandlers) listTenants(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "org_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid organization id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionListTenants, identity.Resource{OrganizationID: &orgID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	tenants, err := h.svc.ListTenants(r.Context(), orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tenants"})
		return
	}
	if tenants == nil {
		tenants = []identity.Tenant{}
	}

	writeJSON(w, http.StatusOK, tenants)
}

func (h *identityHandlers) getTenant(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	tenant, err := h.svc.GetTenant(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get tenant"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetTenant, identity.Resource{
		OrganizationID: &tenant.OrganizationID,
		TenantID:       &id,
	})
	if authErr != nil || decision == identity.Deny {
		// Return 404 to avoid leaking existence of tenant to unauthorized users
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	writeJSON(w, http.StatusOK, tenant)
}

// --- Role Assignments ---

func (h *identityHandlers) createRoleAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	// Look up the tenant to get the org ID for authorization
	tenant, err := h.svc.GetTenant(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get tenant"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionAssignRole, identity.Resource{
		OrganizationID: &tenant.OrganizationID,
		TenantID:       &tenantID,
	})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	var req struct {
		UserID uuid.UUID     `json:"user_id"`
		Role   identity.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if !isValidRole(req.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role: must be one of infra_admin, org_admin, tenant_admin, tenant_member"})
		return
	}

	ra, err := h.svc.AssignRole(r.Context(), req.UserID, identity.ScopeTenant, &tenantID, req.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign role"})
		return
	}

	writeJSON(w, http.StatusCreated, ra)
}

func (h *identityHandlers) listRoleAssignments(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	tenant, err := h.svc.GetTenant(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get tenant"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionListRoles, identity.Resource{
		OrganizationID: &tenant.OrganizationID,
		TenantID:       &tenantID,
	})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	assignments, err := h.svc.ListRoleAssignmentsByScope(r.Context(), identity.ScopeTenant, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list role assignments"})
		return
	}
	if assignments == nil {
		assignments = []identity.RoleAssignment{}
	}

	writeJSON(w, http.StatusOK, assignments)
}

func (h *identityHandlers) deleteRoleAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	assignmentID, err := uuid.Parse(chi.URLParam(r, "assignment_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid assignment id"})
		return
	}

	tenant, err := h.svc.GetTenant(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get tenant"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteRole, identity.Resource{
		OrganizationID: &tenant.OrganizationID,
		TenantID:       &tenantID,
	})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	if err := h.svc.DeleteRoleAssignment(r.Context(), assignmentID); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role assignment not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete role assignment"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func isValidRole(r identity.Role) bool {
	switch r {
	case identity.RoleInfraAdmin, identity.RoleOrgAdmin, identity.RoleTenantAdmin, identity.RoleTenantMember:
		return true
	}
	return false
}
