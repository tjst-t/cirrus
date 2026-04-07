package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionCreateLoadBalancer, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var spec network.LoadBalancerSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if spec.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	lb, err := h.svc.CreateLoadBalancer(r.Context(), tenantID, networkID, spec)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		if errors.Is(err, network.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "load balancer name already in use"})
			return
		}
		if errors.Is(err, network.ErrInvalidState) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create load balancer", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create load balancer"})
		return
	}

	writeJSON(w, http.StatusCreated, lb)
}

func (h *lbHandlers) listLoadBalancers(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionListLoadBalancers, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	lbs, err := h.svc.ListLoadBalancers(r.Context(), networkID)
	if err != nil {
		h.logger.Error("failed to list load balancers", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list load balancers"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	lbID, err := uuid.Parse(chi.URLParam(r, "lb_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid load balancer id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetLoadBalancer, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	lb, err := h.svc.GetLoadBalancer(r.Context(), lbID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "load balancer not found"})
			return
		}
		h.logger.Error("failed to get load balancer", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get load balancer"})
		return
	}
	if lb.NetworkID != networkID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "load balancer not found"})
		return
	}

	writeJSON(w, http.StatusOK, lb)
}

func (h *lbHandlers) deleteLoadBalancer(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant id"})
		return
	}

	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	lbID, err := uuid.Parse(chi.URLParam(r, "lb_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid load balancer id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteLoadBalancer, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	lb, err := h.svc.GetLoadBalancer(r.Context(), lbID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "load balancer not found"})
			return
		}
		h.logger.Error("failed to get load balancer for delete", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get load balancer"})
		return
	}
	if lb.NetworkID != networkID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "load balancer not found"})
		return
	}

	if err := h.svc.DeleteLoadBalancer(r.Context(), lbID); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "load balancer not found"})
			return
		}
		h.logger.Error("failed to delete load balancer", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete load balancer"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
