package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/api"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/topology"
)

// --- mock authenticator / authorizer ---

type testAuthn struct{}

func (a *testAuthn) Authenticate(_ context.Context, _ string) (*identity.User, error) {
	return &identity.User{ID: uuid.New(), ExternalID: "test-admin"}, nil
}

type testAuthz struct{}

func (a *testAuthz) Authorize(_ context.Context, _ *identity.User, _ identity.Action, _ identity.Resource) (identity.Decision, error) {
	return identity.Allow, nil
}

// --- mock topology service (in-memory) ---

type mockTopologySvc struct {
	storageDomains map[uuid.UUID]*topology.StorageDomain
	networkDomains map[uuid.UUID]*topology.NetworkDomain
	locations      map[uuid.UUID]*topology.Location
	hostSD         map[uuid.UUID]map[uuid.UUID]bool
	hostND         map[uuid.UUID]uuid.UUID
	hostLoc        map[uuid.UUID]uuid.UUID
	sdNames        map[string]uuid.UUID
	ndNames        map[string]uuid.UUID
}

func newMockTopologySvc() *mockTopologySvc {
	return &mockTopologySvc{
		storageDomains: make(map[uuid.UUID]*topology.StorageDomain),
		networkDomains: make(map[uuid.UUID]*topology.NetworkDomain),
		locations:      make(map[uuid.UUID]*topology.Location),
		hostSD:         make(map[uuid.UUID]map[uuid.UUID]bool),
		hostND:         make(map[uuid.UUID]uuid.UUID),
		hostLoc:        make(map[uuid.UUID]uuid.UUID),
		sdNames:        make(map[string]uuid.UUID),
		ndNames:        make(map[string]uuid.UUID),
	}
}

func (m *mockTopologySvc) CreateStorageDomain(_ context.Context, name string) (*topology.StorageDomain, error) {
	if _, exists := m.sdNames[name]; exists {
		return nil, topology.ErrConflict
	}
	d := &topology.StorageDomain{ID: uuid.New(), Name: name}
	m.storageDomains[d.ID] = d
	m.sdNames[name] = d.ID
	return d, nil
}
func (m *mockTopologySvc) GetStorageDomain(_ context.Context, id uuid.UUID) (*topology.StorageDomain, error) {
	if d, ok := m.storageDomains[id]; ok {
		return d, nil
	}
	return nil, topology.ErrNotFound
}
func (m *mockTopologySvc) ListStorageDomains(_ context.Context) ([]topology.StorageDomain, error) {
	var out []topology.StorageDomain
	for _, d := range m.storageDomains {
		out = append(out, *d)
	}
	return out, nil
}
func (m *mockTopologySvc) CreateNetworkDomain(_ context.Context, name, ovn string) (*topology.NetworkDomain, error) {
	if _, exists := m.ndNames[name]; exists {
		return nil, topology.ErrConflict
	}
	d := &topology.NetworkDomain{ID: uuid.New(), Name: name, OVNNBConnection: ovn}
	m.networkDomains[d.ID] = d
	m.ndNames[name] = d.ID
	return d, nil
}
func (m *mockTopologySvc) GetNetworkDomain(_ context.Context, id uuid.UUID) (*topology.NetworkDomain, error) {
	if d, ok := m.networkDomains[id]; ok {
		return d, nil
	}
	return nil, topology.ErrNotFound
}
func (m *mockTopologySvc) ListNetworkDomains(_ context.Context) ([]topology.NetworkDomain, error) {
	var out []topology.NetworkDomain
	for _, d := range m.networkDomains {
		out = append(out, *d)
	}
	return out, nil
}
func (m *mockTopologySvc) CreateLocation(_ context.Context, parentID *uuid.UUID, name string, locType topology.LocationType, fa []byte) (*topology.Location, error) {
	loc := &topology.Location{ID: uuid.New(), ParentID: parentID, Name: name, Type: locType, FaultAttributes: fa}
	m.locations[loc.ID] = loc
	return loc, nil
}
func (m *mockTopologySvc) GetLocation(_ context.Context, id uuid.UUID) (*topology.Location, error) {
	if l, ok := m.locations[id]; ok {
		return l, nil
	}
	return nil, topology.ErrNotFound
}
func (m *mockTopologySvc) ListLocations(_ context.Context) ([]topology.Location, error) {
	var out []topology.Location
	for _, l := range m.locations {
		out = append(out, *l)
	}
	return out, nil
}
func (m *mockTopologySvc) GetLocationPath(_ context.Context, _ uuid.UUID) ([]topology.Location, error) {
	return nil, nil
}
func (m *mockTopologySvc) GetLocationTree(_ context.Context, _ uuid.UUID) (*topology.Location, error) {
	return nil, nil
}
func (m *mockTopologySvc) AssociateHostStorageDomain(_ context.Context, hostID, sdID uuid.UUID) error {
	if m.hostSD[hostID] == nil {
		m.hostSD[hostID] = make(map[uuid.UUID]bool)
	}
	m.hostSD[hostID][sdID] = true
	return nil
}
func (m *mockTopologySvc) DissociateHostStorageDomain(_ context.Context, hostID, sdID uuid.UUID) error {
	if s, ok := m.hostSD[hostID]; ok {
		if !s[sdID] {
			return topology.ErrNotFound
		}
		delete(s, sdID)
		return nil
	}
	return topology.ErrNotFound
}
func (m *mockTopologySvc) SetHostNetworkDomain(_ context.Context, hostID, ndID uuid.UUID) error {
	m.hostND[hostID] = ndID
	return nil
}
func (m *mockTopologySvc) SetHostLocation(_ context.Context, hostID, locID uuid.UUID) error {
	m.hostLoc[hostID] = locID
	return nil
}
func (m *mockTopologySvc) GetComputePool(_ context.Context, sdID, ndID uuid.UUID) (*topology.ComputePool, error) {
	sd := m.storageDomains[sdID]
	nd := m.networkDomains[ndID]
	if sd == nil || nd == nil {
		return nil, topology.ErrNotFound
	}
	var hostIDs []uuid.UUID
	for hID, sds := range m.hostSD {
		if sds[sdID] && m.hostND[hID] == ndID {
			hostIDs = append(hostIDs, hID)
		}
	}
	if hostIDs == nil {
		hostIDs = []uuid.UUID{}
	}
	return &topology.ComputePool{
		StorageDomainID: sdID, StorageDomainName: sd.Name,
		NetworkDomainID: ndID, NetworkDomainName: nd.Name,
		HostIDs: hostIDs, Count: len(hostIDs),
	}, nil
}
func (m *mockTopologySvc) GetZones(_ context.Context, level topology.LocationType) ([]topology.Zone, error) {
	var zones []topology.Zone
	for _, loc := range m.locations {
		if loc.Type == level {
			var hostIDs []uuid.UUID
			for hID, locID := range m.hostLoc {
				if locID == loc.ID {
					hostIDs = append(hostIDs, hID)
				}
			}
			if hostIDs == nil {
				hostIDs = []uuid.UUID{}
			}
			zones = append(zones, topology.Zone{
				LocationID: loc.ID, LocationName: loc.Name,
				Level: level, HostIDs: hostIDs, Count: len(hostIDs),
			})
		}
	}
	return zones, nil
}
func (m *mockTopologySvc) ListReachableHosts(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *mockTopologySvc) ListReachableBackends(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

// --- helpers ---

func setupTestRouter(svc topology.Service) http.Handler {
	return api.NewRouter(nil, slog.Default(), &testAuthn{}, &testAuthz{}, nil, nil, svc, nil)
}

func jsonReq(handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func decodeBody[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	return v
}

// --- test: full topology lifecycle ---

func TestTopology_FullFlow(t *testing.T) {
	svc := newMockTopologySvc()
	r := setupTestRouter(svc)

	// 1. Create storage domain
	w := jsonReq(r, "POST", "/api/v1/storage-domains", map[string]string{"name": "sd-ssd"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create SD: %d %s", w.Code, w.Body.String())
	}
	sd := decodeBody[topology.StorageDomain](t, w)
	if sd.Name != "sd-ssd" {
		t.Fatalf("SD name: got %s, want sd-ssd", sd.Name)
	}

	// 2. Create network domain
	w = jsonReq(r, "POST", "/api/v1/network-domains", map[string]any{
		"name": "nd-main", "ovn_nb_connection": "tcp:localhost:6641",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create ND: %d %s", w.Code, w.Body.String())
	}
	nd := decodeBody[topology.NetworkDomain](t, w)

	// 3. Create location hierarchy
	w = jsonReq(r, "POST", "/api/v1/locations", map[string]any{"name": "site-a", "type": "site"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create site: %d %s", w.Code, w.Body.String())
	}
	site := decodeBody[topology.Location](t, w)

	w = jsonReq(r, "POST", "/api/v1/locations", map[string]any{
		"name": "rack-a", "type": "rack", "parent_id": site.ID,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create rack: %d %s", w.Code, w.Body.String())
	}
	rack := decodeBody[topology.Location](t, w)

	// 4. Associate host with topology
	hostID := uuid.New()

	w = jsonReq(r, "POST", fmt.Sprintf("/api/v1/hosts/%s/storage-domains", hostID),
		map[string]string{"storage_domain_id": sd.ID.String()})
	if w.Code != http.StatusNoContent {
		t.Fatalf("associate host→SD: %d %s", w.Code, w.Body.String())
	}

	w = jsonReq(r, "PUT", fmt.Sprintf("/api/v1/hosts/%s/network-domain", hostID),
		map[string]string{"network_domain_id": nd.ID.String()})
	if w.Code != http.StatusNoContent {
		t.Fatalf("set host→ND: %d %s", w.Code, w.Body.String())
	}

	w = jsonReq(r, "PUT", fmt.Sprintf("/api/v1/hosts/%s/location", hostID),
		map[string]string{"location_id": rack.ID.String()})
	if w.Code != http.StatusNoContent {
		t.Fatalf("set host→location: %d %s", w.Code, w.Body.String())
	}

	// 5. Compute pool: host in intersection
	w = jsonReq(r, "GET", fmt.Sprintf("/api/v1/compute-pools?storage_domain_id=%s&network_domain_id=%s", sd.ID, nd.ID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("compute pool: %d %s", w.Code, w.Body.String())
	}
	pool := decodeBody[topology.ComputePool](t, w)
	if pool.Count != 1 || pool.HostIDs[0] != hostID {
		t.Fatalf("pool: want 1 host %s, got %d hosts %v", hostID, pool.Count, pool.HostIDs)
	}

	// 6. Zones at rack level
	w = jsonReq(r, "GET", "/api/v1/zones?level=rack", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("zones: %d %s", w.Code, w.Body.String())
	}
	zones := decodeBody[[]topology.Zone](t, w)
	foundZone := false
	for _, z := range zones {
		if z.LocationID == rack.ID && z.Count == 1 {
			foundZone = true
		}
	}
	if !foundZone {
		t.Fatalf("zone rack-a: want 1 host, got %+v", zones)
	}

	// 7. Dissociate → pool empty
	w = jsonReq(r, "DELETE", fmt.Sprintf("/api/v1/hosts/%s/storage-domains/%s", hostID, sd.ID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("dissociate: %d %s", w.Code, w.Body.String())
	}
	w = jsonReq(r, "GET", fmt.Sprintf("/api/v1/compute-pools?storage_domain_id=%s&network_domain_id=%s", sd.ID, nd.ID), nil)
	pool = decodeBody[topology.ComputePool](t, w)
	if pool.Count != 0 {
		t.Fatalf("pool after dissociate: want 0, got %d", pool.Count)
	}

	// 8. Duplicate → 409
	w = jsonReq(r, "POST", "/api/v1/storage-domains", map[string]string{"name": "sd-ssd"})
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate SD: want 409, got %d", w.Code)
	}
}
