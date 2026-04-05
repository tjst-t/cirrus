package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network"
)

type gatewayHandlers struct {
	svc   network.Service
	authz identity.Authorizer
}

func (h *gatewayHandlers) createGatewayNode(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateGatewayNode, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var spec network.GatewayNodeSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if spec.HostID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id is required"})
		return
	}
	if spec.ExternalIP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "external_ip is required"})
		return
	}
	if spec.InternalIP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "internal_ip is required"})
		return
	}

	gw, err := h.svc.CreateGatewayNode(r.Context(), spec)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "host not found"})
			return
		}
		if errors.Is(err, network.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "gateway node already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create gateway node"})
		return
	}

	writeJSON(w, http.StatusCreated, gw)
}

func (h *gatewayHandlers) listGatewayNodes(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListGatewayNodes, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	nodes, err := h.svc.ListGatewayNodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list gateway nodes"})
		return
	}
	if nodes == nil {
		nodes = []network.GatewayNode{}
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (h *gatewayHandlers) getGatewayNode(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid gateway node id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	gw, err := h.svc.GetGatewayNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "gateway node not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get gateway node"})
		return
	}

	writeJSON(w, http.StatusOK, gw)
}

func (h *gatewayHandlers) deleteGatewayNode(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid gateway node id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeleteGatewayNode(r.Context(), id); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "gateway node not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete gateway node"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *gatewayHandlers) assignGatewayNode(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionAssignGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		GatewayNodeID uuid.UUID `json:"gateway_node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.GatewayNodeID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gateway_node_id is required"})
		return
	}

	if err := h.svc.AssignGatewayNode(r.Context(), networkID, req.GatewayNodeID); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network or gateway node not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign gateway node"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *gatewayHandlers) getNetworkGatewayNode(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetNetworkGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	gw, err := h.svc.GetNetworkGatewayNode(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no gateway node assigned to network"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network gateway node"})
		return
	}

	writeJSON(w, http.StatusOK, gw)
}
