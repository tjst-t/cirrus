package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	if err := validate.Name(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	created, err := h.svc.CreateStorageDomain(r.Context(), req.Name)
	if err != nil {
		if errors.Is(err, topology.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "storage domain with this name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create storage domain"})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *topologyHandlers) listStorageDomains(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListStorageDomains, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	domains, err := h.svc.ListStorageDomains(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list storage domains"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid storage domain id"})
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetStorageDomain, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	domain, err := h.svc.GetStorageDomain(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "storage domain not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get storage domain"})
		return
	}
	writeJSON(w, http.StatusOK, domain)
}

// --- Locations ---

func (h *topologyHandlers) createLocation(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		ParentID        *uuid.UUID      `json:"parent_id,omitempty"`
		Name            string          `json:"name"`
		Type            string          `json:"type"`
		FaultAttributes json.RawMessage `json:"fault_attributes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	locType := topology.LocationType(req.Type)
	if !topology.IsValidLocationType(locType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid type: must be one of site, floor, row, rack, unit"})
		return
	}

	created, err := h.svc.CreateLocation(r.Context(), req.ParentID, req.Name, locType, req.FaultAttributes)
	if err != nil {
		if errors.Is(err, topology.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "location with this name already exists under the same parent"})
			return
		}
		if errors.Is(err, topology.ErrInvalidParent) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create location"})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *topologyHandlers) listLocations(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListLocations, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	locations, err := h.svc.ListLocations(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list locations"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid location id"})
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	loc, err := h.svc.GetLocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "location not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get location"})
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

func (h *topologyHandlers) getLocationPath(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "location_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid location id"})
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	path, err := h.svc.GetLocationPath(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "location not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get location path"})
		return
	}
	writeJSON(w, http.StatusOK, path)
}

func (h *topologyHandlers) getLocationTree(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "location_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid location id"})
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	tree, err := h.svc.GetLocationTree(r.Context(), id)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "location not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get location tree"})
		return
	}
	writeJSON(w, http.StatusOK, tree)
}

// --- Host-domain associations ---

func (h *topologyHandlers) associateHostStorageDomain(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid host id"})
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionManageHostTopology, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		StorageDomainID uuid.UUID `json:"storage_domain_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.svc.AssociateHostStorageDomain(r.Context(), hostID, req.StorageDomainID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "host or storage domain not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to associate host with storage domain"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *topologyHandlers) dissociateHostStorageDomain(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid host id"})
		return
	}
	storageDomainID, err := uuid.Parse(chi.URLParam(r, "storage_domain_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid storage domain id"})
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionManageHostTopology, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DissociateHostStorageDomain(r.Context(), hostID, storageDomainID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "association not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to dissociate host from storage domain"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *topologyHandlers) setHostLocation(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid host id"})
		return
	}

	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionManageHostTopology, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		LocationID uuid.UUID `json:"location_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.svc.SetHostLocation(r.Context(), hostID, req.LocationID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "host or location not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set host location"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Compute pools ---

func (h *topologyHandlers) getComputePool(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetComputePool, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	sdIDStr := r.URL.Query().Get("storage_domain_id")
	if sdIDStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "storage_domain_id query parameter is required"})
		return
	}

	sdID, err := uuid.Parse(sdIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid storage_domain_id"})
		return
	}

	pool, err := h.svc.GetComputePool(r.Context(), sdID)
	if err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compute pool"})
		return
	}
	writeJSON(w, http.StatusOK, pool)
}

// --- Fault Domains ---

func (h *topologyHandlers) getFaultDomains(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetLocation, identity.Resource{}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	levelStr := r.URL.Query().Get("level")
	if levelStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level query parameter is required (site, floor, row, rack, unit)"})
		return
	}

	level := topology.LocationType(levelStr)
	if !topology.IsValidLocationType(level) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid level: must be one of site, floor, row, rack, unit"})
		return
	}

	fds, err := h.svc.GetFaultDomains(r.Context(), level)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get fault domains"})
		return
	}
	if fds == nil {
		fds = []topology.FaultDomain{}
	}
	writeJSON(w, http.StatusOK, fds)
}
