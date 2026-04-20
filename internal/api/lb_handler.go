package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network"
)

type lbHandlers struct {
	svc    network.Service
	authz  identity.Authorizer
	logger *slog.Logger
}

func (h *lbHandlers) createLoadBalancer(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid tenant id", nil)
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionCreateLoadBalancer, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var spec network.LoadBalancerSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if spec.Name == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "name is required", nil)
		return
	}

	lb, err := h.svc.CreateLoadBalancer(r.Context(), tenantID, networkID, spec)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		if errors.Is(err, network.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "load balancer name already in use", nil)
			return
		}
		if errors.Is(err, network.ErrInvalidState) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
			return
		}
		h.logger.Error("failed to create load balancer", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create load balancer", nil)
		return
	}

	writeJSON(w, http.StatusCreated, lb)
}

func (h *lbHandlers) listLoadBalancers(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid tenant id", nil)
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionListLoadBalancers, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	lbs, err := h.svc.ListLoadBalancers(r.Context(), networkID)
	if err != nil {
		h.logger.Error("failed to list load balancers", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list load balancers", nil)
		return
	}
	if lbs == nil {
		lbs = []network.LoadBalancer{}
	}
	writeJSON(w, http.StatusOK, lbs)
}

func (h *lbHandlers) getLoadBalancer(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid tenant id", nil)
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	lbID, err := uuid.Parse(chi.URLParam(r, "lb_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid load balancer id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetLoadBalancer, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	lb, err := h.svc.GetLoadBalancer(r.Context(), lbID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "load balancer not found", nil)
			return
		}
		h.logger.Error("failed to get load balancer", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get load balancer", nil)
		return
	}
	if lb.NetworkID != networkID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "load balancer not found", nil)
		return
	}

	writeJSON(w, http.StatusOK, lb)
}

func (h *lbHandlers) deleteLoadBalancer(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid tenant id", nil)
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	lbID, err := uuid.Parse(chi.URLParam(r, "lb_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid load balancer id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteLoadBalancer, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	lb, err := h.svc.GetLoadBalancer(r.Context(), lbID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "load balancer not found", nil)
			return
		}
		h.logger.Error("failed to get load balancer for delete", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get load balancer", nil)
		return
	}
	if lb.NetworkID != networkID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "load balancer not found", nil)
		return
	}

	if err := h.svc.DeleteLoadBalancer(r.Context(), lbID); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "load balancer not found", nil)
			return
		}
		h.logger.Error("failed to delete load balancer", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete load balancer", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
