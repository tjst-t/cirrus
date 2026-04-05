package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/storage"
)

type storageHandlers struct {
	svc   storage.Service
	authz identity.Authorizer
}

// --- Storage Backends (infra_admin) ---

func (h *storageHandlers) createStorageBackend(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateStorageBackend, identity.Resource{}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		StorageDomainID uuid.UUID      `json:"storage_domain_id"`
		Name            string         `json:"name"`
		Driver          string         `json:"driver"`
		Endpoint        string         `json:"endpoint"`
		TotalCapacityGB int64          `json:"total_capacity_gb"`
		TotalIOPS       int64          `json:"total_iops"`
		Capabilities    []string       `json:"capabilities"`
		DriverConfig    map[string]any `json:"driver_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validateName(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Driver == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "driver is required"})
		return
	}
	if req.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint is required"})
		return
	}

	b, err := h.svc.RegisterBackend(r.Context(), storage.RegisterBackendSpec{
		StorageDomainID: req.StorageDomainID,
		Name:            req.Name,
		Driver:          req.Driver,
		Endpoint:        req.Endpoint,
		TotalCapacityGB: req.TotalCapacityGB,
		TotalIOPS:       req.TotalIOPS,
		Capabilities:    req.Capabilities,
		DriverConfig:    req.DriverConfig,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *storageHandlers) listStorageBackends(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionListStorageBackends, identity.Resource{}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	backends, err := h.svc.ListBackends(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list backends"})
		return
	}
	writeJSON(w, http.StatusOK, backends)
}

func (h *storageHandlers) getStorageBackend(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetStorageBackend, identity.Resource{}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "backend_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid backend_id"})
		return
	}
	b, err := h.svc.GetBackend(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrBackendNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "backend not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get backend"})
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *storageHandlers) drainStorageBackend(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionDrainStorageBackend, identity.Resource{}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "backend_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid backend_id"})
		return
	}
	if err := h.svc.DrainBackend(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrBackendNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "backend not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to drain backend"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Volume Types ---

func (h *storageHandlers) createVolumeType(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateVolumeType, identity.Resource{}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var req struct {
		Name                 string         `json:"name"`
		Description          string         `json:"description"`
		RequiredCapabilities []string       `json:"required_capabilities"`
		QoSPolicy            map[string]any `json:"qos_policy"`
		IsPublic             *bool          `json:"is_public"` // pointer: nil means default (true)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validateName(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := validateDescription(req.Description); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	vt, err := h.svc.CreateVolumeType(r.Context(), req.Name, req.Description, req.RequiredCapabilities, req.QoSPolicy, isPublic)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, vt)
}

func (h *storageHandlers) listVolumeTypes(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	res := identity.Resource{}
	if tenantID != nil {
		res.TenantID = tenantID
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionListVolumeTypes, res); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	vts, err := h.svc.ListVolumeTypes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list volume types"})
		return
	}
	writeJSON(w, http.StatusOK, vts)
}

func (h *storageHandlers) getVolumeType(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	res := identity.Resource{}
	if tenantID != nil {
		res.TenantID = tenantID
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetVolumeType, res); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_type_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid volume_type_id"})
		return
	}
	vt, err := h.svc.GetVolumeType(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrVolumeTypeNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "volume type not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get volume type"})
		return
	}
	writeJSON(w, http.StatusOK, vt)
}

// --- Volumes (tenant-scoped) ---

func (h *storageHandlers) createVolume(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var req struct {
		Name         string     `json:"name"`
		VolumeTypeID *uuid.UUID `json:"volume_type_id"`
		SizeGB       int64      `json:"size_gb"`
		AZID         *uuid.UUID `json:"az_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validateName(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.SizeGB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "size_gb must be a positive integer"})
		return
	}
	v, err := h.svc.CreateVolume(r.Context(), storage.CreateVolumeSpec{
		TenantID:     *tenantID,
		Name:         req.Name,
		VolumeTypeID: req.VolumeTypeID,
		SizeGB:       req.SizeGB,
		AZID:         req.AZID,
	})
	if err != nil {
		if errors.Is(err, storage.ErrNoMatchingBackend) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no storage backend available for this volume type"})
			return
		}
		if errQuotaExceeded(err) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *storageHandlers) listVolumes(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionListVolumes, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	cursor, limit, pErr := parsePaginationParams(r)
	if pErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": pErr.Error()})
		return
	}

	afterAt, afterID := cursorValues(cursor)
	vs, err := h.svc.ListVolumesPage(r.Context(), *tenantID, afterAt, afterID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list volumes"})
		return
	}
	if vs == nil {
		vs = []storage.Volume{}
	}

	nextCursor := ""
	if len(vs) == limit {
		last := vs[len(vs)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, PagedResponse{Items: vs, NextCursor: nextCursor})
}

func (h *storageHandlers) getVolume(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid volume_id"})
		return
	}
	v, err := h.svc.GetVolume(r.Context(), *tenantID, id)
	if err != nil {
		if errors.Is(err, storage.ErrVolumeNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "volume not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get volume"})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *storageHandlers) resizeVolume(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionResizeVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid volume_id"})
		return
	}
	var req struct {
		NewSizeGB int64 `json:"new_size_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	v, err := h.svc.ResizeVolume(r.Context(), *tenantID, id, req.NewSizeGB)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrVolumeNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "volume not found"})
		case errors.Is(err, storage.ErrVolumeInUse):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "volume is in use"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *storageHandlers) deleteVolume(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionDeleteVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid volume_id"})
		return
	}
	if err := h.svc.DeleteVolume(r.Context(), *tenantID, id); err != nil {
		switch {
		case errors.Is(err, storage.ErrVolumeNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "volume not found"})
		case errors.Is(err, storage.ErrVolumeInUse):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "volume is in use"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete volume"})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
