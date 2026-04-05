package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/identity"
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	tenantID := *tenantIDPtr
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateVM, identity.Resource{TenantID: &tenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req createVMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validateName(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.FlavorID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "flavor_id is required"})
		return
	}

	flavorID, err := uuid.Parse(req.FlavorID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid flavor_id: must be a valid UUID"})
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
		if err == nil {
			spec.VolumeTypeID = &vtID
		}
	}

	vm, err := h.svc.CreateVM(r.Context(), spec)
	if err != nil {
		if errQuotaExceeded(err) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}

	writeJSON(w, http.StatusAccepted, vm)
}

func (h *vmHandlers) listVMs(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantIDPtr := TenantIDFromContext(r.Context())
	if tenantIDPtr == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	tenantID := *tenantIDPtr
	res := identity.Resource{TenantID: &tenantID}
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListVMs, res); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	tenantID := *tenantIDPtr

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid vm_id"})
		return
	}

	res := identity.Resource{TenantID: &tenantID}
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetVM, res); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	vm, err := h.svc.GetVM(r.Context(), tenantID, vmID)
	if err != nil {
		if errors.Is(err, compute.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vm not found"})
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}
	writeJSON(w, http.StatusOK, vm)
}

type vmActionRequest struct {
	Action string `json:"action"` // "start", "stop", "force-stop", "reboot"
}

func (h *vmHandlers) vmAction(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantIDPtr := TenantIDFromContext(r.Context())
	if tenantIDPtr == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	tenantID := *tenantIDPtr

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid vm_id"})
		return
	}

	var req vmActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
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
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action: " + req.Action})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, action, res); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
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
	}

	if opErr != nil {
		if errors.Is(opErr, compute.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vm not found"})
			return
		}
		if errors.Is(opErr, compute.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "operation not allowed in current vm state"})
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
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid vm_id"})
		return
	}

	if err := h.svc.RepairVM(r.Context(), vmID); err != nil {
		if errors.Is(err, compute.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vm not found"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}
	tenantID := *tenantIDPtr

	vmID, err := uuid.Parse(r.PathValue("vm_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid vm_id"})
		return
	}

	res := identity.Resource{TenantID: &tenantID}
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteVM, res); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeleteVM(r.Context(), tenantID, vmID); err != nil {
		if errors.Is(err, compute.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vm not found"})
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
