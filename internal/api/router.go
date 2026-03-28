package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/topology"
)

// NewRouter creates the HTTP router with all middleware and routes.
func NewRouter(pool *pgxpool.Pool, logger *slog.Logger, authn identity.Authenticator, authz identity.Authorizer, identitySvc identity.Service, hostSvc host.Service, topologySvc topology.Service, networkSvc network.Service) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestID)
	r.Use(Recovery(logger))
	r.Use(Logger(logger))

	h := &handlers{pool: pool}
	r.Get("/healthz", h.healthz)

	// Identity routes (authenticated)
	ih := &identityHandlers{svc: identitySvc, authz: authz}
	hh := &hostHandlers{svc: hostSvc, authz: authz}
	th := &topologyHandlers{svc: topologySvc, authz: authz}
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

		// Host routes (infra_admin)
		r.Post("/hosts", hh.createHost)
		r.Get("/hosts", hh.listHosts)
		r.Get("/hosts/{host_id}", hh.getHost)
		r.Delete("/hosts/{host_id}", hh.deleteHost)
		r.Post("/hosts/{host_id}/actions", hh.hostAction)

		// Host topology associations (infra_admin)
		r.Post("/hosts/{host_id}/storage-domains", th.associateHostStorageDomain)
		r.Delete("/hosts/{host_id}/storage-domains/{storage_domain_id}", th.dissociateHostStorageDomain)
		r.Put("/hosts/{host_id}/network-domain", th.setHostNetworkDomain)
		r.Put("/hosts/{host_id}/location", th.setHostLocation)

		// Storage domains (infra_admin)
		r.Post("/storage-domains", th.createStorageDomain)
		r.Get("/storage-domains", th.listStorageDomains)
		r.Get("/storage-domains/{storage_domain_id}", th.getStorageDomain)

		// Network domains (infra_admin)
		r.Post("/network-domains", th.createNetworkDomain)
		r.Get("/network-domains", th.listNetworkDomains)
		r.Get("/network-domains/{network_domain_id}", th.getNetworkDomain)

		// Locations (infra_admin)
		r.Post("/locations", th.createLocation)
		r.Get("/locations", th.listLocations)
		r.Get("/locations/{location_id}", th.getLocation)
		r.Get("/locations/{location_id}/path", th.getLocationPath)
		r.Get("/locations/{location_id}/tree", th.getLocationTree)

		// Compute pools (derived, read-only)
		r.Get("/compute-pools", th.getComputePool)

		// Fault domains (derived, read-only)
		r.Get("/fault-domains", th.getFaultDomains)

		// Network routes (tenant-scoped)
		nh := &networkHandlers{svc: networkSvc, authz: authz, logger: logger}
		r.Post("/networks", nh.createNetwork)
		r.Get("/networks", nh.listNetworks)
		r.Get("/networks/{network_id}", nh.getNetwork)
		r.Delete("/networks/{network_id}", nh.deleteNetwork)

		r.Post("/networks/{network_id}/subnets", nh.createSubnet)
		r.Get("/networks/{network_id}/subnets", nh.listSubnets)
		r.Get("/subnets/{subnet_id}", nh.getSubnet)
		r.Delete("/subnets/{subnet_id}", nh.deleteSubnet)

		r.Post("/ports", nh.createPort)
		r.Get("/ports", nh.listPorts)
		r.Get("/ports/{port_id}", nh.getPort)
		r.Delete("/ports/{port_id}", nh.deletePort)
	})

	return r
}
