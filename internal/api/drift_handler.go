package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
)

const (
	driftStatusOpen     = "open"
	driftStatusResolved = "resolved"
)

type driftHandlers struct {
	pool  *pgxpool.Pool
	authz identity.Authorizer
}

type driftEventResponse struct {
	ID           string  `json:"id"`
	ResourceType string  `json:"resource_type"`
	ResourceID   string  `json:"resource_id"`
	Description  string  `json:"description"`
	Status       string  `json:"status"`
	DetectedAt   string  `json:"detected_at"`
	ResolvedAt   *string `json:"resolved_at"`
}

type driftEventsListResponse struct {
	Items      []driftEventResponse `json:"items"`
	NextCursor string               `json:"next_cursor"`
}

// GET /api/v1/admin/drift-events
func (h *driftHandlers) listDriftEvents(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionListDriftEvents, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	status := r.URL.Query().Get("status")
	resourceType := r.URL.Query().Get("resource_type")

	// Build query dynamically based on filters.
	// resource_type maps to the "resource" column (e.g. "vm", "host").
	query := `SELECT id, resource, resource_id,
	                 COALESCE(type, '') AS description,
	                 COALESCE(status, 'open') AS status,
	                 created_at,
	                 resolved_at
	          FROM drift_events
	          WHERE 1=1`
	args := []any{}
	argIdx := 1

	if status != "" {
		query += ` AND COALESCE(status, 'open') = $` + strconv.Itoa(argIdx)
		args = append(args, status)
		argIdx++
	}
	if resourceType != "" {
		query += ` AND resource = $` + strconv.Itoa(argIdx)
		args = append(args, resourceType)
		argIdx++
	}

	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to list drift events", nil)
		return
	}
	defer rows.Close()

	items := []driftEventResponse{}
	for rows.Next() {
		var (
			id           uuid.UUID
			resource     string
			resourceID   string
			description  string
			evStatus     string
			detectedAt   time.Time
			resolvedAt   *time.Time
		)
		if err := rows.Scan(&id, &resource, &resourceID, &description, &evStatus, &detectedAt, &resolvedAt); err != nil {
			writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to scan drift event", nil)
			return
		}
		ev := driftEventResponse{
			ID:           id.String(),
			ResourceType: resource,
			ResourceID:   resourceID,
			Description:  description,
			Status:       evStatus,
			DetectedAt:   detectedAt.Format(time.RFC3339),
		}
		if resolvedAt != nil {
			s := resolvedAt.Format(time.RFC3339)
			ev.ResolvedAt = &s
		}
		items = append(items, ev)
	}
	if err := rows.Err(); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "failed to iterate drift events", nil)
		return
	}

	writeJSON(w, http.StatusOK, driftEventsListResponse{Items: items, NextCursor: ""})
}

// PATCH /api/v1/admin/drift-events/{id}
func (h *driftHandlers) resolveDriftEvent(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if decision, err := h.authz.Authorize(r.Context(), user, identity.ActionResolveDriftEvent, identity.Resource{}); err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid id", nil)
		return
	}

	now := time.Now()
	var (
		resource    string
		resourceID  string
		description string
		detectedAt  time.Time
	)
	err = h.pool.QueryRow(r.Context(),
		`UPDATE drift_events
		 SET status = $1, resolved_at = $2
		 WHERE id = $3
		 RETURNING resource, resource_id, COALESCE(type, ''), created_at`,
		driftStatusResolved, now, id,
	).Scan(&resource, &resourceID, &description, &detectedAt)
	if err != nil {
		// pgx returns pgx.ErrNoRows when the WHERE matched nothing.
		writeErrorCode(w, http.StatusNotFound, apierror.CodeNotFound, "drift event not found", nil)
		return
	}

	resolvedAt := now.Format(time.RFC3339)
	writeJSON(w, http.StatusOK, driftEventResponse{
		ID:           id.String(),
		ResourceType: resource,
		ResourceID:   resourceID,
		Description:  description,
		Status:       driftStatusResolved,
		DetectedAt:   detectedAt.Format(time.RFC3339),
		ResolvedAt:   &resolvedAt,
	})
}

