package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/quota"
	"github.com/tjst-t/cirrus/internal/storage"
)

type storageHandlers struct {
	svc   storage.Service
	authz identity.Authorizer
	debug bool
}

// --- Storage Backends (infra_admin) ---

func (h *storageHandlers) createStorageBackend(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateStorageBackend, identity.Resource{}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validateName(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}
	if req.Driver == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "driver is required", nil)
		return
	}
	if req.Endpoint == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "endpoint is required", nil)
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
		writeInternalError(w, err, h.debug)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *storageHandlers) listStorageBackends(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionListStorageBackends, identity.Resource{}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	backends, err := h.svc.ListBackends(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list backends", nil)
		return
	}
	writeJSON(w, http.StatusOK, backends)
}

func (h *storageHandlers) getStorageBackend(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetStorageBackend, identity.Resource{}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "backend_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid backend_id", nil)
		return
	}
	b, err := h.svc.GetBackend(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrBackendNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "backend not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get backend", nil)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *storageHandlers) drainStorageBackend(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionDrainStorageBackend, identity.Resource{}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "backend_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid backend_id", nil)
		return
	}
	if err := h.svc.DrainBackend(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrBackendNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "backend not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to drain backend", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Volume Types ---

func (h *storageHandlers) createVolumeType(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateVolumeType, identity.Resource{}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validateName(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}
	if err := validateDescription(req.Description); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	vt, err := h.svc.CreateVolumeType(r.Context(), req.Name, req.Description, req.RequiredCapabilities, req.QoSPolicy, isPublic)
	if err != nil {
		writeInternalError(w, err, h.debug)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	vts, err := h.svc.ListVolumeTypes(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list volume types", nil)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_type_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid volume_type_id", nil)
		return
	}
	vt, err := h.svc.GetVolumeType(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrVolumeTypeNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "volume type not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get volume type", nil)
		return
	}
	writeJSON(w, http.StatusOK, vt)
}

// --- Volumes (tenant-scoped) ---

func (h *storageHandlers) createVolume(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	var req struct {
		Name         string     `json:"name"`
		VolumeTypeID *uuid.UUID `json:"volume_type_id"`
		SizeGB       int64      `json:"size_gb"`
		AZID         *uuid.UUID `json:"az_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validateName(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}
	if req.SizeGB <= 0 {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "size_gb must be a positive integer", nil)
		return
	}
	createdBy := callerID(user)
	resp, err := h.svc.CreateVolume(r.Context(), storage.CreateVolumeSpec{
		TenantID:     *tenantID,
		Name:         req.Name,
		VolumeTypeID: req.VolumeTypeID,
		SizeGB:       req.SizeGB,
		AZID:         req.AZID,
	}, createdBy)
	if err != nil {
		if errors.Is(err, storage.ErrNoMatchingBackend) {
			writeErrorCode(w, http.StatusUnprocessableEntity, apierror.CodeInsufficientResources, "no storage backend available for this volume type", nil)
			return
		}
		var violation *quota.ViolationError
		if errors.As(err, &violation) {
			writeQuotaError(w, violation)
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": resp.JobID.String()})
}

func (h *storageHandlers) listVolumes(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionListVolumes, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	cursor, limit, pErr := parsePaginationParams(r)
	if pErr != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, pErr.Error(), nil)
		return
	}

	afterAt, afterID := cursorValues(cursor)
	vs, err := h.svc.ListVolumesPage(r.Context(), *tenantID, afterAt, afterID, limit)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list volumes", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid volume_id", nil)
		return
	}
	v, err := h.svc.GetVolume(r.Context(), *tenantID, id)
	if err != nil {
		if errors.Is(err, storage.ErrVolumeNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "volume not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get volume", nil)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *storageHandlers) resizeVolume(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionResizeVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid volume_id", nil)
		return
	}
	var req struct {
		NewSizeGB int64 `json:"new_size_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	v, err := h.svc.ResizeVolume(r.Context(), *tenantID, id, req.NewSizeGB)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrVolumeNotFound):
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "volume not found", nil)
		case errors.Is(err, storage.ErrVolumeInUse):
			writeInvalidStateError(w, "volume is in use", apierror.ReasonVolumeAttached)
		default:
			writeInternalError(w, err, h.debug)
		}
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *storageHandlers) deleteVolume(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionDeleteVolume, identity.Resource{TenantID: tenantID}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "volume_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid volume_id", nil)
		return
	}
	createdBy := callerID(user)
	resp, err := h.svc.DeleteVolume(r.Context(), *tenantID, id, createdBy)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrVolumeNotFound):
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "volume not found", nil)
		case errors.Is(err, storage.ErrVolumeInUse):
			writeInvalidStateError(w, "volume is in use", apierror.ReasonVolumeAttached)
		default:
			writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete volume", nil)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": resp.JobID.String()})
}
