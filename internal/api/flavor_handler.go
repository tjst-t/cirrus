package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/identity"
)

type flavorHandlers struct {
	svc   flavor.Service
	authz identity.Authorizer
}

func (h *flavorHandlers) createFlavor(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateFlavor, identity.Resource{}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Name     string `json:"name"`
		VCPUs    int    `json:"vcpus"`
		RAMMB    int64  `json:"ram_mb"`
		DiskGB   int64  `json:"disk_gb"`
		IsPublic *bool  `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" || req.VCPUs <= 0 || req.RAMMB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, vcpus, ram_mb are required"})
		return
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	f, err := h.svc.Create(r.Context(), flavor.CreateFlavorSpec{
		Name:     req.Name,
		VCPUs:    req.VCPUs,
		RAMMB:    req.RAMMB,
		DiskGB:   req.DiskGB,
		IsPublic: isPublic,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *flavorHandlers) listFlavors(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	res := identity.Resource{}
	if tenantID != nil {
		res.TenantID = tenantID
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionListFlavors, res); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	flavors, err := h.svc.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, flavors)
}

func (h *flavorHandlers) getFlavor(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	res := identity.Resource{}
	if tenantID != nil {
		res.TenantID = tenantID
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetFlavor, res); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "flavor_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid flavor_id"})
		return
	}

	f, err := h.svc.Get(r.Context(), id)
	if errors.Is(err, flavor.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "flavor not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *flavorHandlers) deleteFlavor(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionDeleteFlavor, identity.Resource{}); decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "flavor_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid flavor_id"})
		return
	}

	if err := h.svc.Delete(r.Context(), id); errors.Is(err, flavor.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "flavor not found"})
		return
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
