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
	"github.com/tjst-t/cirrus/internal/quota"
	"github.com/tjst-t/cirrus/internal/validate"
)

type networkHandlers struct {
	svc    network.Service
	authz  identity.Authorizer
	logger *slog.Logger
	debug  bool
}

// --- Networks ---

func (h *networkHandlers) createNetwork(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateNetwork, identity.Resource{TenantID: tenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		Name string `json:"name"`
		CIDR string `json:"cidr,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	n, err := h.svc.CreateNetwork(r.Context(), *tenantID, network.NetworkSpec{
		Name: req.Name,
		CIDR: req.CIDR,
	})
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "network with this name already exists in tenant", nil)
			return
		}
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid tenant", nil)
			return
		}
		var violation *quota.ViolationError
		if errors.As(err, &violation) {
			writeQuotaError(w, violation)
			return
		}
		writeInternalError(w, err, h.debug)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (h *networkHandlers) listNetworks(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListNetworks, identity.Resource{TenantID: tenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	afterAt, afterID := cursorValues(cursor)
	networks, err := h.svc.ListNetworksPage(r.Context(), *tenantID, afterAt, afterID, limit)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list networks", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network ID", nil)
		return
	}

	n, err := h.svc.GetNetwork(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetNetwork, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	writeJSON(w, http.StatusOK, n)
}

func (h *networkHandlers) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network ID", nil)
		return
	}

	// Get network to check tenant ownership
	n, err := h.svc.GetNetwork(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteNetwork, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if err := h.svc.DeleteNetwork(r.Context(), id); err != nil {
		if errors.Is(err, network.ErrHasDependents) {
			writeInvalidStateError(w, err.Error(), apierror.ReasonHasDependents)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete network", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Groups ---

// networkFromURL retrieves the network from the URL parameter and authorizes the user.
func (h *networkHandlers) networkFromURL(w http.ResponseWriter, r *http.Request) (*network.Network, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network ID", nil)
		return nil, false
	}
	n, err := h.svc.GetNetwork(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return nil, false
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	g, err := h.svc.CreateGroup(r.Context(), net.ID, network.GroupSpec{Name: req.Name})
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "group with this name already exists in network", nil)
			return
		}
		h.logger.Error("failed to create group", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create group", nil)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	afterAt, afterID := cursorValues(cursor)
	groups, err := h.svc.ListGroupsPage(r.Context(), net.ID, afterAt, afterID, limit)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list groups", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid group ID", nil)
		return
	}

	g, err := h.svc.GetGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "group not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get group", nil)
		return
	}

	if g.NetworkID != net.ID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "group not found", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetGroup, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid group ID", nil)
		return
	}

	g, err := h.svc.GetGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "group not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get group", nil)
		return
	}

	if g.NetworkID != net.ID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "group not found", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteGroup, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if err := h.svc.DeleteGroup(r.Context(), groupID); err != nil {
		if errors.Is(err, network.ErrHasDependents) {
			writeInvalidStateError(w, err.Error(), apierror.ReasonHasDependents)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete group", nil)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req network.PolicySpec
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if req.SrcGroupID == uuid.Nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "src_group_id is required", nil)
		return
	}
	if req.DstGroupID == uuid.Nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "dst_group_id is required", nil)
		return
	}
	if req.Protocol == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "protocol is required", nil)
		return
	}

	p, err := h.svc.CreatePolicy(r.Context(), net.ID, req)
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "policy already exists", nil)
			return
		}
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "group not found", nil)
			return
		}
		if errors.Is(err, network.ErrInvalidState) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
			return
		}
		h.logger.Error("failed to create policy", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create policy", nil)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	afterAt, afterID := cursorValues(cursor)
	policies, err := h.svc.ListPoliciesPage(r.Context(), net.ID, afterAt, afterID, limit)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list policies", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid policy ID", nil)
		return
	}

	p, err := h.svc.GetPolicy(r.Context(), policyID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "policy not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get policy", nil)
		return
	}

	if p.NetworkID != net.ID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "policy not found", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeletePolicy, identity.Resource{TenantID: &net.TenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if err := h.svc.DeletePolicy(r.Context(), policyID); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete policy", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Ports (read-only) ---

func (h *networkHandlers) listPorts(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "X-Tenant-ID header required", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListPorts, identity.Resource{TenantID: tenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	networkIDStr := r.URL.Query().Get("network_id")
	if networkIDStr == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "network_id query parameter required", nil)
		return
	}
	networkID, err := uuid.Parse(networkIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network_id", nil)
		return
	}

	// Verify the network belongs to the authorized tenant
	net, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}
	if net.TenantID != *tenantID {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "network does not belong to this tenant", nil)
		return
	}

	ports, err := h.svc.ListPorts(r.Context(), networkID)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list ports", nil)
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	ports, err := h.svc.ListPorts(r.Context(), net.ID)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list ports", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid port ID", nil)
		return
	}

	p, err := h.svc.GetPort(r.Context(), id)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "port not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get port", nil)
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetPort, identity.Resource{TenantID: &p.TenantID}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	writeJSON(w, http.StatusOK, p)
}
