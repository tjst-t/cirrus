package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/identity"
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
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication not configured"})
				return
			}

			token := extractBearerToken(r)
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization token"})
				return
			}

			user, err := authn.Authenticate(r.Context(), token)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}

			ctx := context.WithValue(r.Context(), userKey, user)

			// Resolve optional tenant scope from X-Tenant-ID header
			if tid := r.Header.Get("X-Tenant-ID"); tid != "" {
				id, err := uuid.Parse(tid)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid X-Tenant-ID"})
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
// callerID returns the external identity of the authenticated user,
// or an empty string if no user is present.
func callerID(user *identity.User) string {
	if user != nil {
		return user.ExternalID
	}
	return ""
}

func writeInternalError(w http.ResponseWriter, err error, debug bool) {
	msg := "internal server error"
	if debug {
		msg = err.Error()
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
}
