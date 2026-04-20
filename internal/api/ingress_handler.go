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

type ingressHandlers struct {
	svc   network.Service
	authz identity.Authorizer
}

func (h *ingressHandlers) createIngress(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionCreateIngress, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var spec network.IngressSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if spec.Type == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "type is required", nil)
		return
	}
	if spec.PublicIP == "" {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "public_ip is required", nil)
		return
	}
	if spec.IPPoolID == uuid.Nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "ip_pool_id is required", nil)
		return
	}

	ingress, err := h.svc.CreateIngress(r.Context(), networkID, spec)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ip pool not found", nil)
			return
		}
		if errors.Is(err, network.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "public ip already in use", nil)
			return
		}
		if errors.Is(err, network.ErrInvalidState) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create ingress", nil)
		return
	}

	writeJSON(w, http.StatusCreated, ingress)
}

func (h *ingressHandlers) listIngresses(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionListIngresses, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	ingresses, err := h.svc.ListIngresses(r.Context(), networkID)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list ingresses", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	ingressID, err := uuid.Parse(chi.URLParam(r, "ingress_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid ingress id", nil)
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetIngress, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	ingress, err := h.svc.GetIngress(r.Context(), ingressID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get ingress", nil)
		return
	}
	if ingress.NetworkID != networkID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
		return
	}

	writeJSON(w, http.StatusOK, ingress)
}

func (h *ingressHandlers) updateIngress(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	ingressID, err := uuid.Parse(chi.URLParam(r, "ingress_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid ingress id", nil)
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionCreateIngress, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	// Verify ingress belongs to this network.
	existing, err := h.svc.GetIngress(r.Context(), ingressID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get ingress", nil)
		return
	}
	if existing.NetworkID != networkID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
		return
	}

	var config network.IngressConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}

	updated, err := h.svc.UpdateIngressConfig(r.Context(), ingressID, config)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
			return
		}
		if errors.Is(err, network.ErrInvalidState) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to update ingress", nil)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *ingressHandlers) deleteIngress(w http.ResponseWriter, r *http.Request) {
	networkID, err := uuid.Parse(chi.URLParam(r, "network_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid network id", nil)
		return
	}

	ingressID, err := uuid.Parse(chi.URLParam(r, "ingress_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid ingress id", nil)
		return
	}

	nw, err := h.svc.GetNetwork(r.Context(), networkID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "network not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get network", nil)
		return
	}

	user := UserFromContext(r.Context())
	tenantID := nw.TenantID
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionDeleteIngress, identity.Resource{TenantID: &tenantID})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	ingress, err := h.svc.GetIngress(r.Context(), ingressID)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get ingress", nil)
		return
	}
	if ingress.NetworkID != networkID {
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
		return
	}

	if err := h.svc.DeleteIngress(r.Context(), ingressID); err != nil {
		if errors.Is(err, network.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "ingress not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete ingress", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
