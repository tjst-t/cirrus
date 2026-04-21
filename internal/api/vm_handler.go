package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/quota"
	"github.com/tjst-t/cirrus/internal/scheduler"
)

type vmHandlers struct {
	svc   compute.Service
	authz identity.Authorizer
	debug bool
}

type createVMRequest struct {
	Name         string  `json:"name"`
	FlavorID     string  `json:"flavor_id"`
	AZID         string  `json:"az_id"`
	NetworkID    string  `json:"network_id"`
	VolumeTypeID *string `json:"volume_type_id,omitempty"`
}

func (h *vmHandlers) createVM(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantIDPtr := TenantIDFromContext(r.Context())
	if tenantIDPtr == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	tenantID := *tenantIDPtr
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateVM, identity.Resource{TenantID: &tenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req createVMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validateName(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}
	if req.FlavorID == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "flavor_id is required", nil)
		return
	}

	flavorID, err := uuid.Parse(req.FlavorID)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid flavor_id: must be a valid UUID", nil)
		return
	}

	azID, _ := uuid.Parse(req.AZID)
	networkID, _ := uuid.Parse(req.NetworkID)

	spec := compute.CreateVMSpec{
		TenantID:  tenantID,
		Name:      req.Name,
		FlavorID:  flavorID,
		AZID:      azID,
		NetworkID: networkID,
	}
	if req.VolumeTypeID != nil {
		vtID, err := uuid.Parse(*req.VolumeTypeID)
		if err != nil {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid volume_type_id: must be a valid UUID", nil)
			return
		}
		spec.VolumeTypeID = &vtID
	}

	resp, err := h.svc.CreateVM(r.Context(), spec)
	if err != nil {
		// スケジューラエラー
		if errors.Is(err, scheduler.ErrNoSuitableHost) {
			writeErrorCode(w, http.StatusUnprocessableEntity, apierror.CodeNoHost, "no suitable host available", nil)
			return
		}
		if errors.Is(err, scheduler.ErrNoSuitableBackend) {
			writeErrorCode(w, http.StatusUnprocessableEntity, apierror.CodeInsufficientResources, "no suitable storage backend available", nil)
			return
		}
		// クォータエラー
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

func (h *vmHandlers) listVMs(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantIDPtr := TenantIDFromContext(r.Context())
	if tenantIDPtr == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	tenantID := *tenantIDPtr
	res := identity.Resource{TenantID: &tenantID}
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListVMs, res); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	afterAt, afterID := cursorValues(cursor)
	vms, err := h.svc.ListVMsPage(r.Context(), tenantID, afterAt, afterID, limit)
	if err != nil {
		writeInternalError(w, err, h.debug)
		return
	}
	if vms == nil {
		vms = []compute.VM{}
	}

	nextCursor := ""
	if len(vms) == limit {
		last := vms[len(vms)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, PagedResponse{Items: vms, NextCursor: nextCursor})
}

func (h *vmHandlers) getVM(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantIDPtr := TenantIDFromContext(r.Context())
	if tenantIDPtr == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	tenantID := *tenantIDPtr

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid vm_id", nil)
		return
	}

	res := identity.Resource{TenantID: &tenantID}
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetVM, res); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	vm, err := h.svc.GetVM(r.Context(), tenantID, vmID)
	if err != nil {
		if errors.Is(err, compute.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "vm not found", nil)
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}
	writeJSON(w, http.StatusOK, vm)
}

type vmActionRequest struct {
	Action       string  `json:"action"`                  // "start", "stop", "force-stop", "reboot", "migrate"
	TargetHostID *string `json:"target_host_id,omitempty"` // optional: target host UUID for migrate
}

func (h *vmHandlers) vmAction(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantIDPtr := TenantIDFromContext(r.Context())
	if tenantIDPtr == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	tenantID := *tenantIDPtr

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid vm_id", nil)
		return
	}

	var req vmActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}

	res := identity.Resource{TenantID: &tenantID}
	var action identity.Action
	switch req.Action {
	case "start":
		action = identity.ActionStartVM
	case "stop":
		action = identity.ActionStopVM
	case "force-stop":
		action = identity.ActionForceStopVM
	case "reboot":
		action = identity.ActionRebootVM
	case "migrate":
		action = identity.ActionMigrateVM
	default:
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "unknown action: "+req.Action, nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, action, res); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var opErr error
	switch req.Action {
	case "start":
		opErr = h.svc.StartVM(r.Context(), tenantID, vmID)
	case "stop":
		opErr = h.svc.StopVM(r.Context(), tenantID, vmID)
	case "force-stop":
		opErr = h.svc.ForceStopVM(r.Context(), tenantID, vmID)
	case "reboot":
		opErr = h.svc.RebootVM(r.Context(), tenantID, vmID)
	case "migrate":
		var targetHostID *uuid.UUID
		if req.TargetHostID != nil {
			parsed, err := uuid.Parse(*req.TargetHostID)
			if err != nil {
				writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid target_host_id: must be a valid UUID", nil)
				return
			}
			targetHostID = &parsed
		}
		opErr = h.svc.MigrateVM(r.Context(), tenantID, vmID, targetHostID)
	}

	if opErr != nil {
		if errors.Is(opErr, compute.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "vm not found", nil)
			return
		}
		if errors.Is(opErr, compute.ErrConflict) {
			reason := apierror.ReasonVMNotRunning // stop/force-stop/reboot/migrate は running 必須
			if req.Action == "start" {
				reason = apierror.ReasonVMNotStopped // start は stopped 必須
			}
			writeInvalidStateError(w, "operation not allowed in current vm state", reason)
			return
		}
		if req.Action == "migrate" && errors.Is(opErr, scheduler.ErrNoSuitableHost) {
			writeErrorCode(w, http.StatusUnprocessableEntity, apierror.CodeNoHost, "no suitable host available", nil)
			return
		}
		writeInternalError(w, opErr, h.debug)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *vmHandlers) repairVM(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	res := identity.Resource{}
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionRepairVM, res); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid vm_id", nil)
		return
	}

	if err := h.svc.RepairVM(r.Context(), vmID); err != nil {
		if errors.Is(err, compute.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "vm not found", nil)
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *vmHandlers) deleteVM(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantIDPtr := TenantIDFromContext(r.Context())
	if tenantIDPtr == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}
	tenantID := *tenantIDPtr

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid vm_id", nil)
		return
	}

	res := identity.Resource{TenantID: &tenantID}
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteVM, res); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	resp, err := h.svc.DeleteVM(r.Context(), tenantID, vmID)
	if err != nil {
		if errors.Is(err, compute.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "vm not found", nil)
			return
		}
		if errors.Is(err, compute.ErrConflict) {
			writeInvalidStateError(w, "vm cannot be deleted in its current state", apierror.ReasonVMRunning)
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": resp.JobID.String()})
}

