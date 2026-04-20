package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/apierror"
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
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
		return
	}
	if err := validate.Name(req.Name); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	created, isNew, err := h.svc.RegisterOrGet(r.Context(), req.Name, req.Address, "", "", "")
	if err != nil {
		if errors.Is(err, host.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "host with this name already exists", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to create host", nil)
		return
	}

	status := http.StatusCreated
	if !isNew {
		status = http.StatusOK
	}
	writeJSON(w, status, created)
}

func (h *hostHandlers) listHosts(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListHosts, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	// State filter bypasses cursor pagination (simple filter, typically small result).
	if stateParam := r.URL.Query().Get("state"); stateParam != "" {
		state := host.OperationalState(stateParam)
		if !host.IsValidOperationalState(state) {
			writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid state: must be one of registering, active, maintenance, draining, faulty, retiring", nil)
			return
		}
		hosts, err := h.svc.ListHostsByState(r.Context(), state)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list hosts", nil)
			return
		}
		if hosts == nil {
			hosts = []host.Host{}
		}
		writeJSON(w, http.StatusOK, hosts)
		return
	}

	cursor, limit, err := parsePaginationParams(r)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, err.Error(), nil)
		return
	}

	afterAt, afterID := cursorValues(cursor)
	hosts, err := h.svc.ListHostsPage(r.Context(), afterAt, afterID, limit)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list hosts", nil)
		return
	}
	if hosts == nil {
		hosts = []host.Host{}
	}

	nextCursor := ""
	if len(hosts) == limit {
		last := hosts[len(hosts)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, PagedResponse{Items: hosts, NextCursor: nextCursor})
}

func (h *hostHandlers) deleteHost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid host id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionHostAction, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if err := h.svc.DeleteHost(r.Context(), id); err != nil {
		if errors.Is(err, host.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "host not found", nil)
			return
		}
		if errors.Is(err, host.ErrInvalidState) {
			writeInvalidStateError(w, err.Error(), apierror.ReasonHostNotOperable)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to delete host", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *hostHandlers) getHost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid host id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionGetHost, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	result, err := h.svc.GetHost(r.Context(), id)
	if err != nil {
		if errors.Is(err, host.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "host not found", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to get host", nil)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *hostHandlers) hostAction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "host_id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid host id", nil)
		return
	}

	user := UserFromContext(r.Context())
	decision, authErr := h.authz.Authorize(r.Context(), user, identity.ActionHostAction, identity.Resource{})
	if authErr != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid request body", nil)
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
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid action: must be one of maintenance, activate, drain, retire", nil)
		return
	}

	if err := h.svc.SetOperationalState(r.Context(), id, state); err != nil {
		if errors.Is(err, host.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "host not found", nil)
			return
		}
		if errors.Is(err, host.ErrInvalidState) {
			writeInvalidStateError(w, err.Error(), apierror.ReasonHostNotOperable)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to update host state", nil)
		return
	}

	// Re-fetch to return the updated host (SetOperationalState already verified existence)
	result, _ := h.svc.GetHost(r.Context(), id)
	writeJSON(w, http.StatusOK, result)
}
