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

type egressHandlers struct {
	svc    network.Service
	authz  identity.Authorizer
	logger *slog.Logger
}

// networkFromEgressURL retrieves the network from the URL parameter {network_id} and verifies
// it belongs to the tenant in the URL {tenant_id}.
func (h *egressHandlers) networkFromEgressURL(w http.ResponseWriter, r *http.Request) (*network.Network, bool) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network ID"})
		return nil, false
	}
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tenant ID"})
		return nil, false
	}

	n, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return nil, false
	}
	if n.TenantID != tenantID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
		return nil, false
	}
	return n, true
}

func (h *egressHandlers) createEgress(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateEgress, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var spec network.EgressSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if spec.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type is required"})
		return
	}
	if spec.Type != "nat_gateway" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported egress type; only nat_gateway is supported"})
		return
	}
	if spec.Config.PublicIP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config.public_ip is required for nat_gateway"})
		return
	}

	e, err := h.svc.CreateEgress(r.Context(), net.ID, spec)
	if err != nil {
		if errors.Is(err, network.ErrInvalidState) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create egress", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create egress"})
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *egressHandlers) listEgresses(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListEgresses, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	egresses, err := h.svc.ListEgresses(r.Context(), net.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list egresses"})
		return
	}
	if egresses == nil {
		egresses = []network.Egress{}
	}
	writeJSON(w, http.StatusOK, egresses)
}

func (h *egressHandlers) getEgress(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	egressID, err := uuid.Parse(chi.URLParam(r, "egress_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid egress ID"})
		return
	}

	e, err := h.svc.GetEgress(r.Context(), egressID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get egress"})
		return
	}

	if e.NetworkID != net.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetEgress, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, e)
}

func (h *egressHandlers) deleteEgress(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	egressID, err := uuid.Parse(chi.URLParam(r, "egress_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid egress ID"})
		return
	}

	e, err := h.svc.GetEgress(r.Context(), egressID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get egress"})
		return
	}

	if e.NetworkID != net.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteEgress, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeleteEgress(r.Context(), egressID); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete egress"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
