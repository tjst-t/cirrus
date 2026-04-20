package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/topology"
	"github.com/tjst-t/cirrus/internal/validate"
)

type topologyHandlers struct {
	svc   topology.Service
	authz identity.Authorizer
}

// --- Storage domains ---

func (h *topologyHandlers) createStorageDomain(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateStorageDomain, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	created, err := h.svc.CreateStorageDomain(r.Context(), req.Name)
	if err != nil {
		if errors.Is(err, topology.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "storage domain with this name already exists", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create storage domain", nil)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *topologyHandlers) listStorageDomains(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListStorageDomains, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	domains, err := h.svc.ListStorageDomains(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list storage domains", nil)
		return
	}
	if domains == nil {
		domains = []topology.StorageDomain{}
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *topologyHandlers) getStorageDomain(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "storage_domain_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid storage domain id", nil)
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetStorageDomain, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	domain, err := h.svc.GetStorageDomain(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "storage domain not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get storage domain", nil)
		return
	}
	writeJSON(w, http.StatusOK, domain)
}

// --- Locations ---

func (h *topologyHandlers) createLocation(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		ParentID        *uuid.UUID      `json:"parent_id,omitempty"`
		Name            string          `json:"name"`
		Type            string          `json:"type"`
		FaultAttributes json.RawMessage `json:"fault_attributes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	locType := topology.LocationType(req.Type)
	if !topology.IsValidLocationType(locType) {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid type: must be one of site, floor, row, rack, unit", nil)
		return
	}

	created, err := h.svc.CreateLocation(r.Context(), req.ParentID, req.Name, locType, req.FaultAttributes)
	if err != nil {
		if errors.Is(err, topology.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "location with this name already exists under the same parent", nil)
			return
		}
		if errors.Is(err, topology.ErrInvalidParent) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create location", nil)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *topologyHandlers) listLocations(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListLocations, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	locations, err := h.svc.ListLocations(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list locations", nil)
		return
	}
	if locations == nil {
		locations = []topology.Location{}
	}
	writeJSON(w, http.StatusOK, locations)
}

func (h *topologyHandlers) getLocation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "location_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid location id", nil)
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	loc, err := h.svc.GetLocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "location not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get location", nil)
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

func (h *topologyHandlers) getLocationPath(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "location_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid location id", nil)
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	path, err := h.svc.GetLocationPath(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "location not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get location path", nil)
		return
	}
	writeJSON(w, http.StatusOK, path)
}

func (h *topologyHandlers) getLocationTree(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "location_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid location id", nil)
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	tree, err := h.svc.GetLocationTree(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "location not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get location tree", nil)
		return
	}
	writeJSON(w, http.StatusOK, tree)
}

// --- Host-domain associations ---

func (h *topologyHandlers) associateHostStorageDomain(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid host id", nil)
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionManageHostTopology, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		StorageDomainID uuid.UUID `json:"storage_domain_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}

	if err := h.svc.AssociateHostStorageDomain(r.Context(), hostID, req.StorageDomainID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "host or storage domain not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to associate host with storage domain", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *topologyHandlers) dissociateHostStorageDomain(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid host id", nil)
		return
	}
	storageDomainID, err := uuid.Parse(chi.URLParam(r, "storage_domain_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid storage domain id", nil)
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionManageHostTopology, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if err := h.svc.DissociateHostStorageDomain(r.Context(), hostID, storageDomainID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "association not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to dissociate host from storage domain", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *topologyHandlers) setHostLocation(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid host id", nil)
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionManageHostTopology, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		LocationID uuid.UUID `json:"location_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}

	if err := h.svc.SetHostLocation(r.Context(), hostID, req.LocationID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "host or location not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to set host location", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Compute pools ---

func (h *topologyHandlers) getComputePool(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetComputePool, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	sdIDStr := r.URL.Query().Get("storage_domain_id")
	if sdIDStr == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "storage_domain_id query parameter is required", nil)
		return
	}

	sdID, err := uuid.Parse(sdIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid storage_domain_id", nil)
		return
	}

	pool, err := h.svc.GetComputePool(r.Context(), sdID)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "domain not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to compute pool", nil)
		return
	}
	writeJSON(w, http.StatusOK, pool)
}

// --- Fault Domains ---

func (h *topologyHandlers) getFaultDomains(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	levelStr := r.URL.Query().Get("level")
	if levelStr == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "level query parameter is required (site, floor, row, rack, unit)", nil)
		return
	}

	level := topology.LocationType(levelStr)
	if !topology.IsValidLocationType(level) {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid level: must be one of site, floor, row, rack, unit", nil)
		return
	}

	fds, err := h.svc.GetFaultDomains(r.Context(), level)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get fault domains", nil)
		return
	}
	if fds == nil {
		fds = []topology.FaultDomain{}
	}
	writeJSON(w, http.StatusOK, fds)
}
