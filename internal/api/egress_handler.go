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

// sanitizeEgress returns a copy of e with sensitive encrypted fields zeroed out
// so they are not leaked via the API.
func sanitizeEgress(e *network.Egress) network.Egress {
	out := *e
	if out.Config.VPNWireGuard != nil {
		wg := *out.Config.VPNWireGuard
		wg.PrivateKeyEnc = ""
		out.Config.VPNWireGuard = &wg
	}
	if out.Config.VPNIPsec != nil {
		ipsec := *out.Config.VPNIPsec
		ipsec.PreSharedKeyEnc = ""
		out.Config.VPNIPsec = &ipsec
	}
	return out
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

	n, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateEgress, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
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
	switch spec.Type {
	case network.EgressTypeNATGateway,
		network.EgressTypeVPNIPsec,
		network.EgressTypeVPNWireGuard,
		network.EgressTypeDirectConnect:
		// valid
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported egress type; supported types: nat_gateway, vpn_ipsec, vpn_wireguard, direct_connect"})
		return
	}

	e, err := h.svc.CreateEgress(r.Context(), n.ID, spec)
	if err != nil {
		if errors.Is(err, network.ErrInvalidState) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create egress", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create egress"})
		return
	}

	// The canonical public_key location is Config.VPNWireGuard.PublicKey, which is
	// already populated and not stripped by sanitizeEgress. No top-level duplication needed.
	writeJSON(w, http.StatusCreated, sanitizeEgress(e))
}

func (h *egressHandlers) listEgresses(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	n, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListEgresses, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	egresses, err := h.svc.ListEgresses(r.Context(), n.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list egresses"})
		return
	}
	if egresses == nil {
		egresses = []network.Egress{}
	}
	sanitized := make([]network.Egress, len(egresses))
	for i := range egresses {
		sanitized[i] = sanitizeEgress(&egresses[i])
	}
	writeJSON(w, http.StatusOK, sanitized)
}

func (h *egressHandlers) getEgress(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	n, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionGetEgress, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
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

	if e.NetworkID != n.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
		return
	}

	writeJSON(w, http.StatusOK, sanitizeEgress(e))
}

func (h *egressHandlers) deleteEgress(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	n, ok := h.networkFromEgressURL(w, r)
	if !ok {
		return
	}

	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDeleteEgress, identity.Resource{TenantID: &n.TenantID}); err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	egressID, err := uuid.Parse(chi.URLParam(r, "egress_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid egress ID"})
		return
	}

	// Verify egress belongs to this network before deleting.
	e, err := h.svc.GetEgress(r.Context(), egressID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get egress"})
		return
	}
	if e.NetworkID != n.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "egress not found"})
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
