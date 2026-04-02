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
	if req.Name == "" || req.FlavorID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and flavor_id are required"})
		return
	}

	flavorID, err := uuid.Parse(req.FlavorID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid flavor_id"})
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

	vms, err := h.svc.ListVMs(r.Context(), tenantID)
	if err != nil {
		writeInternalError(w, err, h.debug)
		return
	}
	if vms == nil {
		vms = []compute.VM{}
	}
	writeJSON(w, http.StatusOK, vms)
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
