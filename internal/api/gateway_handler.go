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

type gatewayHandlers struct {
	svc   network.Service
	authz identity.Authorizer
}

func (h *gatewayHandlers) createGatewayNode(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateGatewayNode, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var spec network.GatewayNodeSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if spec.HostID == uuid.Nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "host_id is required", nil)
		return
	}
	if spec.ExternalIP == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "external_ip is required", nil)
		return
	}
	if spec.InternalIP == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "internal_ip is required", nil)
		return
	}

	gw, err := h.svc.CreateGatewayNode(r.Context(), spec)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "host not found", nil)
			return
		}
		if errors.Is(err, network.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "gateway node already exists", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create gateway node", nil)
		return
	}

	writeJSON(w, http.StatusCreated, gw)
}

func (h *gatewayHandlers) listGatewayNodes(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListGatewayNodes, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	nodes, err := h.svc.ListGatewayNodes(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list gateway nodes", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid gateway node id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	gw, err := h.svc.GetGatewayNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "gateway node not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get gateway node", nil)
		return
	}

	writeJSON(w, http.StatusOK, gw)
}

func (h *gatewayHandlers) deleteGatewayNode(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid gateway node id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if err := h.svc.DeleteGatewayNode(r.Context(), id); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "gateway node not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete gateway node", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *gatewayHandlers) assignGatewayNode(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionAssignGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		GatewayNodeID uuid.UUID `json:"gateway_node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if req.GatewayNodeID == uuid.Nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "gateway_node_id is required", nil)
		return
	}

	if err := h.svc.AssignGatewayNode(r.Context(), networkID, req.GatewayNodeID); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network or gateway node not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to assign gateway node", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *gatewayHandlers) getNetworkGatewayNode(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetNetworkGatewayNode, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	gw, err := h.svc.GetNetworkGatewayNode(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "no gateway node assigned to network", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network gateway node", nil)
		return
	}

	writeJSON(w, http.StatusOK, gw)
}
