package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/quota"
)

const (
	userKey     contextKey = "user"
	tenantIDKey contextKey = "tenant_id"
)

// UserFromContext extracts the authenticated user from context.
func UserFromContext(ctx context.Context) *identity.User {
	u, _ := ctx.Value(userKey).(*identity.User)
	return u
}

// TenantIDFromContext extracts the tenant ID from context.
func TenantIDFromContext(ctx context.Context) *uuid.UUID {
	id, _ := ctx.Value(tenantIDKey).(*uuid.UUID)
	return id
}

// Auth is middleware that authenticates requests via Bearer token.
func Auth(authn identity.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authn == nil {
				writeErrorCode(w, http.StatusUnauthorized, apierror.CodeUnauthorized, "authentication not configured", nil)
				return
			}

			token := extractBearerToken(r)
			if token == "" {
				writeErrorCode(w, http.StatusUnauthorized, apierror.CodeUnauthorized, "missing authorization token", nil)
				return
			}

			user, err := authn.Authenticate(r.Context(), token)
			if err != nil {
				writeErrorCode(w, http.StatusUnauthorized, apierror.CodeUnauthorized, "invalid token", nil)
				return
			}

			ctx := context.WithValue(r.Context(), userKey, user)

			// Resolve optional tenant scope from X-Tenant-ID header
			if tid := r.Header.Get("X-Tenant-ID"); tid != "" {
				id, err := uuid.Parse(tid)
				if err != nil {
					writeErrorCode(w, http.StatusBadRequest, apierror.CodeBadRequest, "invalid X-Tenant-ID", nil)
					return
				}
				ctx = context.WithValue(ctx, tenantIDKey, &id)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeInternalError writes a 500 response. In debug mode the actual error
// message is included; otherwise a generic message is returned to avoid
// leaking internal details to API clients.
func writeInternalError(w http.ResponseWriter, err error, debug bool) {
	msg := "internal server error"
	if debug {
		msg = err.Error()
	}
	writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, msg, nil)
}

// callerID returns the external identity of the authenticated user,
// or an empty string if no user is present.
func callerID(user *identity.User) string {
	if user != nil {
		return user.ExternalID
	}
	return ""
}

func writeErrorCode(w http.ResponseWriter, status int, code string, message string, detail any) {
	writeJSON(w, status, apierror.ErrorResponse{
		Code:    code,
		Message: message,
		Detail:  detail,
	})
}

// writeInvalidStateError writes a 409 ERR_INVALID_STATE response with a machine-readable reason.
// reason は apierror.Reason* 定数を使うこと。
func writeInvalidStateError(w http.ResponseWriter, msg, reason string) {
	writeErrorCode(w, http.StatusConflict, apierror.CodeInvalidState, msg, map[string]string{"reason": reason})
}

// writeQuotaError writes a 422 response for quota-exceeded errors with resource detail.
func writeQuotaError(w http.ResponseWriter, v *quota.ViolationError) {
	writeErrorCode(w, http.StatusUnprocessableEntity, apierror.QuotaResourceToCode(v.Resource), "quota exceeded", map[string]any{
		"resource":  v.Resource,
		"limit":     v.Limit,
		"requested": v.Requested,
		"current":   v.Current,
	})
}
