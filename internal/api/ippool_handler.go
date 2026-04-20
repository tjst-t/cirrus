package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network"
)

type ipPoolHandlers struct {
	svc   network.Service
	authz identity.Authorizer
}

func (h *ipPoolHandlers) createIPPool(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateIPPool, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var spec network.IPPoolSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if spec.Name == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "name is required", nil)
		return
	}
	if spec.CIDR == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "cidr is required", nil)
		return
	}

	pool, err := h.svc.CreateIPPool(r.Context(), spec)
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "ip pool with that name already exists", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create ip pool", nil)
		return
	}

	writeJSON(w, http.StatusCreated, pool)
}

func (h *ipPoolHandlers) listIPPools(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListIPPools, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	pools, err := h.svc.ListIPPools(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list ip pools", nil)
		return
	}
	if pools == nil {
		pools = []network.IPPool{}
	}
	writeJSON(w, http.StatusOK, pools)
}

func (h *ipPoolHandlers) getIPPool(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "pool_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid ip pool id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetIPPool, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	pool, err := h.svc.GetIPPool(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ip pool not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get ip pool", nil)
		return
	}

	writeJSON(w, http.StatusOK, pool)
}

func (h *ipPoolHandlers) deleteIPPool(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "pool_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid ip pool id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteIPPool, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if err := h.svc.DeleteIPPool(r.Context(), id); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ip pool not found", nil)
			return
		}
		if errors.Is(err, network.ErrConflict) {
			writeInvalidStateError(w, "ip pool is in use", apierror.ReasonIPInUse)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete ip pool", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
