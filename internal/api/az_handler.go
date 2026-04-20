package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/validate"
)

type azHandlers struct {
	svc   az.Service
	authz identity.Authorizer
}

// --- Admin: CRUD ---

func (h *azHandlers) createAZ(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		Name        string    `json:"name"`
		Description string    `json:"description"`
		LocationID  uuid.UUID `json:"location_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}
	if req.LocationID == uuid.Nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "location_id is required", nil)
		return
	}

	created, err := h.svc.Create(r.Context(), req.Name, req.Description, req.LocationID)
	if err != nil {
		if errors.Is(err, az.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "availability zone with this name already exists", nil)
			return
		}
		if errors.Is(err, az.ErrNotFound) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "location or network domain not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create availability zone", nil)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *azHandlers) listAZs(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListAZs, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	azs, err := h.svc.List(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list availability zones", nil)
		return
	}
	if azs == nil {
		azs = []az.AvailabilityZone{}
	}
	writeJSON(w, http.StatusOK, azs)
}

// getAZ is the tenant-facing endpoint. It returns only enabled AZs.
// All authenticated users can access it (no RBAC check beyond Auth middleware).
func (h *azHandlers) getAZ(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid availability zone ID", nil)
		return
	}

	a, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "availability zone not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get availability zone", nil)
		return
	}
	if !a.Enabled {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "availability zone not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// getAZAdmin is the admin-facing endpoint. Returns any AZ regardless of enabled status.
func (h *azHandlers) getAZAdmin(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListAZs, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid availability zone ID", nil)
		return
	}

	a, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "availability zone not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get availability zone", nil)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *azHandlers) updateAZ(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionUpdateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid availability zone ID", nil)
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if req.Name != nil {
		if err := validate.Name(*req.Name); err != nil {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
			return
		}
	}

	updated, err := h.svc.Update(r.Context(), id, req.Name, req.Description, req.Enabled)
	if err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "availability zone not found", nil)
			return
		}
		if errors.Is(err, az.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "name already taken", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to update availability zone", nil)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *azHandlers) deleteAZ(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid availability zone ID", nil)
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "availability zone not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete availability zone", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Storage domain associations ---

func (h *azHandlers) addStorageDomain(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionUpdateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	azID, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid availability zone ID", nil)
		return
	}

	var req struct {
		StorageDomainID uuid.UUID `json:"storage_domain_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}

	if err := h.svc.AddStorageDomain(r.Context(), azID, req.StorageDomainID); err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "availability zone or storage domain not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to add storage domain", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *azHandlers) removeStorageDomain(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionUpdateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	azID, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid availability zone ID", nil)
		return
	}

	sdID, err := uuid.Parse(chi.URLParam(r, "storage_domain_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid storage domain ID", nil)
		return
	}

	if err := h.svc.RemoveStorageDomain(r.Context(), azID, sdID); err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "association not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to remove storage domain", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Tenant: read-only ---

func (h *azHandlers) listEnabledAZs(w http.ResponseWriter, r *http.Request) {
	// All authenticated users can list enabled AZs
	azs, err := h.svc.ListEnabled(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list availability zones", nil)
		return
	}
	if azs == nil {
		azs = []az.AvailabilityZone{}
	}
	writeJSON(w, http.StatusOK, azs)
}
