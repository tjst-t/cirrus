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

type ingressHandlers struct {
	svc   network.Service
	authz identity.Authorizer
}

func (h *ingressHandlers) createIngress(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionCreateIngress, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var spec network.IngressSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if spec.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type is required"})
		return
	}
	if spec.PublicIP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "public_ip is required"})
		return
	}
	if spec.IPPoolID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ip_pool_id is required"})
		return
	}

	ingress, err := h.svc.CreateIngress(r.Context(), networkID, spec)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "ip pool not found"})
			return
		}
		if errors.Is(err, network.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "public ip already in use"})
			return
		}
		if errors.Is(err, network.ErrInvalidState) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create ingress"})
		return
	}

	writeJSON(w, http.StatusCreated, ingress)
}

func (h *ingressHandlers) listIngresses(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionListIngresses, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	ingresses, err := h.svc.ListIngresses(r.Context(), networkID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list ingresses"})
		return
	}
	if ingresses == nil {
		ingresses = []network.Ingress{}
	}
	writeJSON(w, http.StatusOK, ingresses)
}

func (h *ingressHandlers) getIngress(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	ingressID, err := uuid.Parse(chi.URLParam(r, "ingress_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ingress id"})
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetIngress, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	ingress, err := h.svc.GetIngress(r.Context(), ingressID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "ingress not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get ingress"})
		return
	}

	writeJSON(w, http.StatusOK, ingress)
}

func (h *ingressHandlers) deleteIngress(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid network id"})
		return
	}

	ingressID, err := uuid.Parse(chi.URLParam(r, "ingress_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ingress id"})
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "network not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get network"})
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteIngress, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.svc.DeleteIngress(r.Context(), ingressID); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "ingress not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete ingress"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
