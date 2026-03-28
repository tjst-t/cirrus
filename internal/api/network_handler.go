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
		Name            string    `json:"name"`
		NetworkDomainID uuid.UUID `json:"network_domain_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.NetworkDomainID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "network_domain_id is required"})
		return
	}

	n, err := h.svc.CreateNetwork(r.Context(), *tenantID, network.NetworkSpec{
		Name:            req.Name,
		NetworkDomainID: req.NetworkDomainID,
	})
	if err != nil {
		if errors.Is(err, network.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "network with this name already exists in tenant"})
			return
		}
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "network domain not found"})
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

	networks, err := h.svc.ListNetworks(r.Context(), *tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list networks"})
		return
	}
	if networks == nil {
		networks = []network.Network{}
	}
	writeJSON(w, http.StatusOK, networks)
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

// --- Subnets ---

func (h *networkHandlers) createSubnet(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network ID"})
		return
	}

	// Verify network exists and check authorization
	n, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateNetwork, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req network.SubnetSpec
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.CIDR == "" || req.Gateway == "" || req.DHCPRangeStart == "" || req.DHCPRangeEnd == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cidr, gateway, dhcp_range_start, dhcp_range_end are required"})
		return
	}

	sub, err := h.svc.CreateSubnet(r.Context(), networkID, req)
	if err != nil {
		if errors.Is(err, network.ErrInvalidCIDR) || errors.Is(err, network.ErrInvalidGateway) || errors.Is(err, network.ErrInvalidRange) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create subnet", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create subnet"})
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (h *networkHandlers) listSubnets(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network ID"})
		return
	}

	n, err := h.svc.GetNetwork(r.Context(), networkID)
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

	subnets, err := h.svc.ListSubnets(r.Context(), networkID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list subnets"})
		return
	}
	if subnets == nil {
		subnets = []network.Subnet{}
	}
	writeJSON(w, http.StatusOK, subnets)
}

func (h *networkHandlers) getSubnet(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	subnetID, err := uuid.Parse(chi.URLParam(r, "subnet_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid subnet ID"})
		return
	}

	sub, err := h.svc.GetSubnet(r.Context(), subnetID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subnet not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get subnet"})
		return
	}

	n, err := h.svc.GetNetwork(r.Context(), sub.NetworkID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetNetwork, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

func (h *networkHandlers) deleteSubnet(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	subnetID, err := uuid.Parse(chi.URLParam(r, "subnet_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid subnet ID"})
		return
	}

	sub, err := h.svc.GetSubnet(r.Context(), subnetID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subnet not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get subnet"})
		return
	}

	// Check authorization via the parent network
	n, err := h.svc.GetNetwork(r.Context(), sub.NetworkID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteNetwork, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeleteSubnet(r.Context(), subnetID); err != nil {
		if errors.Is(err, network.ErrHasDependents) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete subnet"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Ports ---

func (h *networkHandlers) createPort(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	tenantID := TenantIDFromContext(r.Context())
	if tenantID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Tenant-ID header required"})
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreatePort, identity.Resource{TenantID: tenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		NetworkID uuid.UUID `json:"network_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.NetworkID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "network_id is required"})
		return
	}

	// Verify the network belongs to the authorized tenant
	net, err := h.svc.GetNetwork(r.Context(), req.NetworkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}
	if net.TenantID != *tenantID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "network does not belong to this tenant"})
		return
	}

	p, err := h.svc.CreatePort(r.Context(), *tenantID, network.PortSpec{NetworkID: req.NetworkID})
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create port", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create port"})
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

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

func (h *networkHandlers) deletePort(w http.ResponseWriter, r *http.Request) {
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

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeletePort, identity.Resource{TenantID: &p.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeletePort(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete port"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
