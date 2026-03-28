package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/az"
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
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Name            string    `json:"name"`
		Description     string    `json:"description"`
		LocationID      uuid.UUID `json:"location_id"`
		NetworkDomainID uuid.UUID `json:"network_domain_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.LocationID == uuid.Nil || req.NetworkDomainID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "location_id and network_domain_id are required"})
		return
	}

	created, err := h.svc.Create(r.Context(), req.Name, req.Description, req.LocationID, req.NetworkDomainID)
	if err != nil {
		if errors.Is(err, az.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "availability zone with this name or network domain already exists"})
			return
		}
		if errors.Is(err, az.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "location or network domain not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create availability zone"})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *azHandlers) listAZs(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListAZs, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	azs, err := h.svc.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list availability zones"})
		return
	}
	if azs == nil {
		azs = []az.AvailabilityZone{}
	}
	writeJSON(w, http.StatusOK, azs)
}

func (h *azHandlers) getAZ(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListAZs, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid availability zone ID"})
		return
	}

	a, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "availability zone not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get availability zone"})
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *azHandlers) updateAZ(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid availability zone ID"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updated, err := h.svc.Update(r.Context(), id, req.Name, req.Description, req.Enabled)
	if err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "availability zone not found"})
			return
		}
		if errors.Is(err, az.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "name already taken"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update availability zone"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *azHandlers) deleteAZ(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid availability zone ID"})
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "availability zone not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete availability zone"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Storage domain associations ---

func (h *azHandlers) addStorageDomain(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	azID, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid availability zone ID"})
		return
	}

	var req struct {
		StorageDomainID uuid.UUID `json:"storage_domain_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.svc.AddStorageDomain(r.Context(), azID, req.StorageDomainID); err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "availability zone or storage domain not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add storage domain"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *azHandlers) removeStorageDomain(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateAZ, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	azID, err := uuid.Parse(chi.URLParam(r, "az_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid availability zone ID"})
		return
	}

	sdID, err := uuid.Parse(chi.URLParam(r, "storage_domain_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid storage domain ID"})
		return
	}

	if err := h.svc.RemoveStorageDomain(r.Context(), azID, sdID); err != nil {
		if errors.Is(err, az.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "association not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove storage domain"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Tenant: read-only ---

func (h *azHandlers) listEnabledAZs(w http.ResponseWriter, r *http.Request) {
	// All authenticated users can list enabled AZs
	azs, err := h.svc.ListEnabled(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list availability zones"})
		return
	}
	if azs == nil {
		azs = []az.AvailabilityZone{}
	}
	writeJSON(w, http.StatusOK, azs)
}
