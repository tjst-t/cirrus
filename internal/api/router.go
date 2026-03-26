package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/internal/identity"
)

// NewRouter creates the HTTP router with all middleware and routes.
func NewRouter(pool *pgxpool.Pool, logger *slog.Logger, authn identity.Authenticator, authz identity.Authorizer, identitySvc identity.Service) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestID)
	r.Use(Recovery(logger))
	r.Use(Logger(logger))

	h := &handlers{pool: pool}
	r.Get("/healthz", h.healthz)

	// Identity routes (authenticated)
	ih := &identityHandlers{svc: identitySvc, authz: authz}
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(Auth(authn))

		r.Post("/organizations", ih.createOrganization)
		r.Get("/organizations", ih.listOrganizations)
		r.Get("/organizations/{org_id}", ih.getOrganization)

		r.Post("/organizations/{org_id}/tenants", ih.createTenant)
		r.Get("/organizations/{org_id}/tenants", ih.listTenants)
		r.Get("/tenants/{tenant_id}", ih.getTenant)

		r.Post("/tenants/{tenant_id}/role-assignments", ih.createRoleAssignment)
		r.Get("/tenants/{tenant_id}/role-assignments", ih.listRoleAssignments)
		r.Delete("/tenants/{tenant_id}/role-assignments/{assignment_id}", ih.deleteRoleAssignment)
	})

	return r
}
