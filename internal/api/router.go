package api

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/jobqueue"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/quota"
	"github.com/tjst-t/cirrus/internal/storage"
	"github.com/tjst-t/cirrus/internal/topology"
)

// spaHandler serves a Single Page Application from the given directory.
// Any path that does not correspond to a real file is served as index.html.
type spaHandler struct {
	fs   http.Handler
	dist string
}

func newSPAHandler(staticPath string) *spaHandler {
	return &spaHandler{
		fs:   http.FileServer(http.Dir(staticPath)),
		dist: staticPath,
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if the requested file exists in dist
	path := filepath.Join(h.dist, filepath.Clean("/"+r.URL.Path))
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// SPA fallback: serve index.html
		http.ServeFile(w, r, filepath.Join(h.dist, "index.html"))
		return
	}
	h.fs.ServeHTTP(w, r)
}

// staticDistHandler returns a handler that serves web/dist if it exists, otherwise nil.
func staticDistHandler() http.Handler {
	// Resolve relative to executable or cwd
	candidates := []string{"web/dist", "../web/dist"}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return newSPAHandler(p)
		}
	}
	return nil
}


// NewRouterOptions holds optional/advanced parameters for NewRouter.
type NewRouterOptions struct {
	// DRSRunner is the DRS Runner instance shared between the periodic ticker
	// and the admin API handler.  May be nil if DRS is disabled.
	DRSRunner       DRSRunner
	DRSEnabled      bool
	DRSIntervalSecs int
}

// NewRouter creates the HTTP router with all middleware and routes.
// debug controls whether internal error details are included in 500 responses.
func NewRouter(pool *pgxpool.Pool, logger *slog.Logger, authn identity.Authenticator, authz identity.Authorizer, identitySvc identity.Service, hostSvc host.Service, topologySvc topology.Service, networkSvc network.Service, azSvc az.Service, storageSvc storage.Service, flavorSvc flavor.Service, computeSvc compute.Service, quotaSvc quota.Service, jobQueue jobqueue.Queue, debug bool, opts ...NewRouterOptions) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestID)
	r.Use(Recovery(logger))
	r.Use(Logger(logger))

	// Merge variadic options (only first element used, variadic is for backwards compat).
	var routerOpts NewRouterOptions
	if len(opts) > 0 {
		routerOpts = opts[0]
	}

	h := &handlers{pool: pool}
	r.Get("/healthz", h.healthz)

	// Identity routes (authenticated)
	ih := &identityHandlers{svc: identitySvc, authz: authz}
	hh := &hostHandlers{svc: hostSvc, authz: authz}
	th := &topologyHandlers{svc: topologySvc, authz: authz}
	qh := &quotaHandlers{svc: quotaSvc, authz: authz}
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(Auth(authn))

		// Token verification — returns 200 if the Bearer token is valid.
		r.Post("/auth/verify", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})

		r.Get("/me/tenants", ih.listMyTenants)

		r.Post("/organizations", ih.createOrganization)
		r.Get("/organizations", ih.listOrganizations)
		r.Get("/organizations/{org_id}", ih.getOrganization)

		r.Post("/organizations/{org_id}/tenants", ih.createTenant)
		r.Get("/organizations/{org_id}/tenants", ih.listTenants)
		r.Get("/tenants/{tenant_id}", ih.getTenant)

		r.Post("/tenants/{tenant_id}/role-assignments", ih.createRoleAssignment)
		r.Get("/tenants/{tenant_id}/role-assignments", ih.listRoleAssignments)
		r.Delete("/tenants/{tenant_id}/role-assignments/{assignment_id}", ih.deleteRoleAssignment)

		// Quota routes
		r.Get("/tenants/{tenant_id}/quota", qh.getTenantQuota)
		r.Put("/tenants/{tenant_id}/quota", qh.setTenantQuota)
		r.Get("/organizations/{org_id}/quota", qh.getOrgQuota)
		r.Put("/organizations/{org_id}/quota", qh.setOrgQuota)

		// Host routes (infra_admin)
		r.Post("/hosts", hh.createHost)
		r.Get("/hosts", hh.listHosts)
		r.Get("/hosts/{host_id}", hh.getHost)
		r.Delete("/hosts/{host_id}", hh.deleteHost)
		r.Post("/hosts/{host_id}/actions", hh.hostAction)

		// Host topology associations (infra_admin)
		r.Post("/hosts/{host_id}/storage-domains", th.associateHostStorageDomain)
		r.Delete("/hosts/{host_id}/storage-domains/{storage_domain_id}", th.dissociateHostStorageDomain)
		r.Put("/hosts/{host_id}/location", th.setHostLocation)

		// Storage domains (infra_admin)
		r.Post("/storage-domains", th.createStorageDomain)
		r.Get("/storage-domains", th.listStorageDomains)
		r.Get("/storage-domains/{storage_domain_id}", th.getStorageDomain)

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

		// Availability zones (admin: CRUD, tenant: read-only)
		ah := &azHandlers{svc: azSvc, authz: authz}
		r.Get("/availability-zones", ah.listEnabledAZs) // tenant: enabled AZs only
		r.Get("/availability-zones/{az_id}", ah.getAZ)
		r.Post("/admin/availability-zones", ah.createAZ)
		r.Get("/admin/availability-zones", ah.listAZs)  // admin: all AZs
		r.Get("/admin/availability-zones/{az_id}", ah.getAZAdmin) // admin: any AZ
		r.Put("/admin/availability-zones/{az_id}", ah.updateAZ)
		r.Delete("/admin/availability-zones/{az_id}", ah.deleteAZ)
		r.Post("/admin/availability-zones/{az_id}/storage-domains", ah.addStorageDomain)
		r.Delete("/admin/availability-zones/{az_id}/storage-domains/{storage_domain_id}", ah.removeStorageDomain)

		// Network routes (tenant-scoped)
		nh := &networkHandlers{svc: networkSvc, authz: authz, logger: logger, debug: debug}
		r.Post("/networks", nh.createNetwork)
		r.Get("/networks", nh.listNetworks)
		r.Route("/networks/{network_id}", func(r chi.Router) {
			r.Get("/", nh.getNetwork)
			r.Delete("/", nh.deleteNetwork)

			r.Post("/groups", nh.createGroup)
			r.Get("/groups", nh.listGroups)
			r.Get("/groups/{group_id}", nh.getGroup)
			r.Delete("/groups/{group_id}", nh.deleteGroup)

			r.Post("/policies", nh.createPolicy)
			r.Get("/policies", nh.listPolicies)
			r.Delete("/policies/{policy_id}", nh.deletePolicy)

			r.Get("/ports", nh.listPortsNested)
		})

		r.Get("/ports", nh.listPorts)
		r.Get("/ports/{port_id}", nh.getPort)

		// Storage backends (infra_admin)
		sh := &storageHandlers{svc: storageSvc, authz: authz, debug: debug}
		r.Post("/admin/storage-backends", sh.createStorageBackend)
		r.Get("/admin/storage-backends", sh.listStorageBackends)
		r.Get("/admin/storage-backends/{backend_id}", sh.getStorageBackend)
		r.Post("/admin/storage-backends/{backend_id}/drain", sh.drainStorageBackend)

		// Volume types (infra_admin: create; all authenticated: list/get)
		r.Post("/admin/volume-types", sh.createVolumeType)
		r.Get("/volume-types", sh.listVolumeTypes)
		r.Get("/volume-types/{volume_type_id}", sh.getVolumeType)

		// Volumes (tenant-scoped)
		r.Post("/volumes", sh.createVolume)
		r.Get("/volumes", sh.listVolumes)
		r.Get("/volumes/{volume_id}", sh.getVolume)
		r.Delete("/volumes/{volume_id}", sh.deleteVolume)
		r.Post("/volumes/{volume_id}/resize", sh.resizeVolume)

		// Flavors (infra_admin: create/delete; all authenticated: list/get)
		fh := &flavorHandlers{svc: flavorSvc, authz: authz, debug: debug}
		r.Post("/admin/flavors", fh.createFlavor)
		r.Delete("/admin/flavors/{flavor_id}", fh.deleteFlavor)
		r.Get("/flavors", fh.listFlavors)
		r.Get("/flavors/{flavor_id}", fh.getFlavor)

		// Jobs
		jh := &jobHandlers{queue: jobQueue, authz: authz}
		r.Get("/jobs/{job_id}", jh.getJob)

		// VMs
		vh := &vmHandlers{svc: computeSvc, authz: authz, debug: debug}
		r.Post("/vms", vh.createVM)
		r.Get("/vms", vh.listVMs)
		r.Get("/vms/{vm_id}", vh.getVM)
		r.Delete("/vms/{vm_id}", vh.deleteVM)
		r.Post("/vms/{vm_id}/actions", vh.vmAction)
		r.Post("/admin/vms/{vm_id}/repair", vh.repairVM)

		// Gateway nodes (infra_admin)
		gwh := &gatewayHandlers{svc: networkSvc, authz: authz}
		r.Post("/admin/gateway-nodes", gwh.createGatewayNode)
		r.Get("/admin/gateway-nodes", gwh.listGatewayNodes)
		r.Get("/admin/gateway-nodes/{id}", gwh.getGatewayNode)
		r.Delete("/admin/gateway-nodes/{id}", gwh.deleteGatewayNode)
		r.Put("/admin/networks/{network_id}/gateway", gwh.assignGatewayNode)
		r.Get("/admin/networks/{network_id}/gateway", gwh.getNetworkGatewayNode)

		// Egress routes (tenant-scoped, nested under tenant/network)
		eh := &egressHandlers{svc: networkSvc, authz: authz, logger: logger}
		r.Route("/tenants/{tenant_id}/networks/{network_id}/egresses", func(r chi.Router) {
			r.Post("/", eh.createEgress)
			r.Get("/", eh.listEgresses)
			r.Get("/{egress_id}", eh.getEgress)
			r.Patch("/{egress_id}", eh.updateEgress)
			r.Delete("/{egress_id}", eh.deleteEgress)
		})

		// IP Pools (infra_admin)
		iph := &ipPoolHandlers{svc: networkSvc, authz: authz}
		r.Post("/admin/ip-pools", iph.createIPPool)
		r.Get("/admin/ip-pools", iph.listIPPools)
		r.Get("/admin/ip-pools/{pool_id}", iph.getIPPool)
		r.Delete("/admin/ip-pools/{pool_id}", iph.deleteIPPool)

		// Ingresses (tenant-scoped, nested under network)
		ingh := &ingressHandlers{svc: networkSvc, authz: authz}
		r.Post("/networks/{network_id}/ingresses", ingh.createIngress)
		r.Get("/networks/{network_id}/ingresses", ingh.listIngresses)
		r.Get("/networks/{network_id}/ingresses/{ingress_id}", ingh.getIngress)
		r.Patch("/networks/{network_id}/ingresses/{ingress_id}", ingh.updateIngress)
		r.Delete("/networks/{network_id}/ingresses/{ingress_id}", ingh.deleteIngress)

		// Internal Load Balancers (tenant-scoped, nested under tenant/network)
		lbh := &lbHandlers{svc: networkSvc, authz: authz, logger: logger}
		r.Post("/tenants/{tenant_id}/networks/{network_id}/load-balancers", lbh.createLoadBalancer)
		r.Get("/tenants/{tenant_id}/networks/{network_id}/load-balancers", lbh.listLoadBalancers)
		r.Get("/tenants/{tenant_id}/networks/{network_id}/load-balancers/{lb_id}", lbh.getLoadBalancer)
		r.Delete("/tenants/{tenant_id}/networks/{network_id}/load-balancers/{lb_id}", lbh.deleteLoadBalancer)

		// Drift events (infra_admin)
		dh := &driftHandlers{pool: pool, authz: authz}
		r.Get("/admin/drift-events", dh.listDriftEvents)
		r.Patch("/admin/drift-events/{id}", dh.resolveDriftEvent)

		// DRS admin endpoints (infra_admin)
		drsh := &drsHandlers{
			runner:          routerOpts.DRSRunner,
			authz:           authz,
			drsEnabled:      routerOpts.DRSEnabled,
			intervalSeconds: routerOpts.DRSIntervalSecs,
		}
		r.Post("/admin/drs/run", drsh.run)
		r.Get("/admin/drs/status", drsh.status)
	})

	// SPA static files — serve web/dist if present.
	// Must be registered AFTER /api/* routes so API takes priority.
	if spa := staticDistHandler(); spa != nil {
		serveSPA := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Guard against future /api/* paths that are not yet registered.
			// /api/v1/* is already handled above and chi would match it first,
			// but this prevents SPA from swallowing unrecognised /api/* paths.
			if strings.HasPrefix(req.URL.Path, "/api/") {
				http.NotFound(w, req)
				return
			}
			spa.ServeHTTP(w, req)
		})
		r.Get("/*", serveSPA)
		r.Get("/", serveSPA)
	}

	return r
}
