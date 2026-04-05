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
	"github.com/tjst-t/cirrus/internal/validate"
)

type networkHandlers struct {
	svc    network.Service
	authz  identity.Authorizer
	logger *slog.Logger
}

// --- Networks ---

func (h *networkHandlers) createNetwork(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateNetwork, identity.Resource{TenantID: tenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Name string `json:"name"`
		CIDR string `json:"cidr,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	n, err := h.svc.CreateNetwork(r.Context(), *tenantID, network.NetworkSpec{
		Name: req.Name,
		CIDR: req.CIDR,
	})
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "network with this name already exists in tenant"})
			return
		}
		if errQuotaExceeded(err) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create network", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create network"})
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (h *networkHandlers) listNetworks(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListNetworks, identity.Resource{TenantID: tenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	afterAt, afterID := cursorValues(cursor)
	networks, err := h.svc.ListNetworksPage(r.Context(), *tenantID, afterAt, afterID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list networks"})
		return
	}
	if networks == nil {
		networks = []network.Network{}
	}

	nextCursor := ""
	if len(networks) == limit {
		last := networks[len(networks)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, PagedResponse{Items: networks, NextCursor: nextCursor})
}

func (h *networkHandlers) getNetwork(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network ID"})
		return
	}

	n, err := h.svc.GetNetwork(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetNetwork, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, n)
}

func (h *networkHandlers) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network ID"})
		return
	}

	// Get network to check tenant ownership
	n, err := h.svc.GetNetwork(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteNetwork, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeleteNetwork(r.Context(), id); err != nil {
		if errors.Is(err, network.ErrHasDependents) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete network"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Groups ---

// networkFromURL retrieves the network from the URL parameter and authorizes the user.
func (h *networkHandlers) networkFromURL(w http.ResponseWriter, r *http.Request) (*network.Network, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network ID"})
		return nil, false
	}
	n, err := h.svc.GetNetwork(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return nil, false
	}
	return n, true
}

func (h *networkHandlers) createGroup(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateGroup, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	g, err := h.svc.CreateGroup(r.Context(), net.ID, network.GroupSpec{Name: req.Name})
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "group with this name already exists in network"})
			return
		}
		h.logger.Error("failed to create group", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create group"})
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *networkHandlers) listGroups(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListGroups, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	afterAt, afterID := cursorValues(cursor)
	groups, err := h.svc.ListGroupsPage(r.Context(), net.ID, afterAt, afterID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list groups"})
		return
	}
	if groups == nil {
		groups = []network.Group{}
	}

	nextCursor := ""
	if len(groups) == limit {
		last := groups[len(groups)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, PagedResponse{Items: groups, NextCursor: nextCursor})
}

func (h *networkHandlers) getGroup(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	groupID, err := uuid.Parse(chi.URLParam(r, "group_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid group ID"})
		return
	}

	g, err := h.svc.GetGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get group"})
		return
	}

	if g.NetworkID != net.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetGroup, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, g)
}

func (h *networkHandlers) deleteGroup(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	groupID, err := uuid.Parse(chi.URLParam(r, "group_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid group ID"})
		return
	}

	g, err := h.svc.GetGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get group"})
		return
	}

	if g.NetworkID != net.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteGroup, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeleteGroup(r.Context(), groupID); err != nil {
		if errors.Is(err, network.ErrHasDependents) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete group"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Policies ---

func (h *networkHandlers) createPolicy(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreatePolicy, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req network.PolicySpec
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.SrcGroupID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "src_group_id is required"})
		return
	}
	if req.DstGroupID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dst_group_id is required"})
		return
	}
	if req.Protocol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "protocol is required"})
		return
	}

	p, err := h.svc.CreatePolicy(r.Context(), net.ID, req)
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "policy already exists"})
			return
		}
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group not found"})
			return
		}
		if errors.Is(err, network.ErrInvalidState) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create policy", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create policy"})
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *networkHandlers) listPolicies(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListPolicies, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	afterAt, afterID := cursorValues(cursor)
	policies, err := h.svc.ListPoliciesPage(r.Context(), net.ID, afterAt, afterID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list policies"})
		return
	}
	if policies == nil {
		policies = []network.Policy{}
	}

	nextCursor := ""
	if len(policies) == limit {
		last := policies[len(policies)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, PagedResponse{Items: policies, NextCursor: nextCursor})
}

func (h *networkHandlers) deletePolicy(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	policyID, err := uuid.Parse(chi.URLParam(r, "policy_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid policy ID"})
		return
	}

	p, err := h.svc.GetPolicy(r.Context(), policyID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get policy"})
		return
	}

	if p.NetworkID != net.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeletePolicy, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeletePolicy(r.Context(), policyID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete policy"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Ports (read-only) ---

func (h *networkHandlers) listPorts(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListPorts, identity.Resource{TenantID: tenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	networkIDStr := r.URL.Query().Get("network_id")
	if networkIDStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "network_id query parameter required"})
		return
	}
	networkID, err := uuid.Parse(networkIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network_id"})
		return
	}

	// Verify the network belongs to the authorized tenant
	net, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}
	if net.TenantID != *tenantID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "network does not belong to this tenant"})
		return
	}

	ports, err := h.svc.ListPorts(r.Context(), networkID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list ports"})
		return
	}
	if ports == nil {
		ports = []network.Port{}
	}
	writeJSON(w, http.StatusOK, ports)
}

func (h *networkHandlers) listPortsNested(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	net, ok := h.networkFromURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListPorts, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	ports, err := h.svc.ListPorts(r.Context(), net.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list ports"})
		return
	}
	if ports == nil {
		ports = []network.Port{}
	}
	writeJSON(w, http.StatusOK, ports)
}

func (h *networkHandlers) getPort(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "port_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid port ID"})
		return
	}

	p, err := h.svc.GetPort(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "port not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get port"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetPort, identity.Resource{TenantID: &p.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, p)
}
