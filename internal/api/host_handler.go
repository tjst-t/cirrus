package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/validate"
)

type hostHandlers struct {
	svc   host.Service
	authz identity.Authorizer
}

func (h *hostHandlers) createHost(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionCreateHost, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		ID      *uuid.UUID `json:"id,omitempty"`
		Name    string     `json:"name"`
		Address string     `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	created, err := h.svc.Register(r.Context(), req.ID, req.Name, req.Address)
	if err != nil {
		if errors.Is(err, host.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "host with this name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create host"})
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *hostHandlers) listHosts(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListHosts, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	hosts, err := h.svc.ListHosts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list hosts"})
		return
	}
	if hosts == nil {
		hosts = []host.Host{}
	}

	writeJSON(w, http.StatusOK, hosts)
}

func (h *hostHandlers) getHost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid host id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetHost, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	result, err := h.svc.GetHost(r.Context(), id)
	if err != nil {
		if errors.Is(err, host.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "host not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get host"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *hostHandlers) hostAction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid host id"})
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionHostAction, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var state host.OperationalState
	switch req.Action {
	case "maintenance":
		state = host.StateMaintenance
	case "activate":
		state = host.StateActive
	case "drain":
		state = host.StateDraining
	case "retire":
		state = host.StateRetiring
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid action: must be one of maintenance, activate, drain, retire"})
		return
	}

	if err := h.svc.SetOperationalState(r.Context(), id, state); err != nil {
		if errors.Is(err, host.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "host not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update host state"})
		return
	}

	// Re-fetch to return the updated host (SetOperationalState already verified existence)
	result, _ := h.svc.GetHost(r.Context(), id)
	writeJSON(w, http.StatusOK, result)
}
