package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/identity"
)

type flavorHandlers struct {
	svc   flavor.Service
	authz identity.Authorizer
	debug bool
}

func (h *flavorHandlers) createFlavor(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionCreateFlavor, identity.Resource{}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validateName(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}
	if req.VCPUs <= 0 {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "vcpus must be a positive integer", nil)
		return
	}
	if req.RAMMB <= 0 {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "ram_mb must be a positive integer", nil)
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
		writeInternalError(w, err, h.debug)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	afterAt, afterID := cursorValues(cursor)
	flavors, err := h.svc.ListPage(r.Context(), afterAt, afterID, limit)
	if err != nil {
		writeInternalError(w, err, h.debug)
		return
	}
	if flavors == nil {
		flavors = []flavor.Flavor{}
	}

	nextCursor := ""
	if len(flavors) == limit {
		last := flavors[len(flavors)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, PagedResponse{Items: flavors, NextCursor: nextCursor})
}

func (h *flavorHandlers) getFlavor(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	res := identity.Resource{}
	if tenantID != nil {
		res.TenantID = tenantID
	}
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionGetFlavor, res); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "flavor_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid flavor_id", nil)
		return
	}

	f, err := h.svc.Get(r.Context(), id)
	if errors.Is(err, flavor.ErrNotFound) {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "flavor not found", nil)
		return
	}
	if err != nil {
		writeInternalError(w, err, h.debug)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *flavorHandlers) deleteFlavor(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, _ := h.authz.Authorize(r.Context(), user, identity.ActionDeleteFlavor, identity.Resource{}); decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "flavor_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid flavor_id", nil)
		return
	}

	if err := h.svc.Delete(r.Context(), id); errors.Is(err, flavor.ErrNotFound) {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "flavor not found", nil)
		return
	} else if err != nil {
		writeInternalError(w, err, h.debug)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
