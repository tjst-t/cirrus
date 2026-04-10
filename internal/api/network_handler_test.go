package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/api"
	"github.com/tjst-t/cirrus/internal/network"
)

// mockNetworkSvc is a stub implementation of network.Service used in tests.
// Only CreateNetwork has meaningful logic; all other methods are no-ops.
type mockNetworkSvc struct {
	createNetworkErr error
}

func (m *mockNetworkSvc) CreateNetwork(_ context.Context, _ uuid.UUID, _ network.NetworkSpec) (*network.Network, error) {
	if m.createNetworkErr != nil {
		return nil, m.createNetworkErr
	}
	return &network.Network{ID: uuid.New(), Name: "test-net"}, nil
}

func (m *mockNetworkSvc) GetNetwork(_ context.Context, _ uuid.UUID) (*network.Network, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListNetworks(_ context.Context, _ uuid.UUID) ([]network.Network, error) {
	return nil, nil
}
func (m *mockNetworkSvc) ListNetworksPage(_ context.Context, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]network.Network, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeleteNetwork(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockNetworkSvc) CreateGroup(_ context.Context, _ uuid.UUID, _ network.GroupSpec) (*network.Group, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetGroup(_ context.Context, _ uuid.UUID) (*network.Group, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListGroups(_ context.Context, _ uuid.UUID) ([]network.Group, error) {
	return nil, nil
}
func (m *mockNetworkSvc) ListGroupsPage(_ context.Context, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]network.Group, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeleteGroup(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockNetworkSvc) CreatePolicy(_ context.Context, _ uuid.UUID, _ network.PolicySpec) (*network.Policy, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetPolicy(_ context.Context, _ uuid.UUID) (*network.Policy, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListPolicies(_ context.Context, _ uuid.UUID) ([]network.Policy, error) {
	return nil, nil
}
func (m *mockNetworkSvc) ListPoliciesPage(_ context.Context, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]network.Policy, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeletePolicy(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockNetworkSvc) CreatePort(_ context.Context, _ network.PortSpec) (*network.Port, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetPort(_ context.Context, _ uuid.UUID) (*network.Port, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) GetPortByVMID(_ context.Context, _ uuid.UUID) (*network.Port, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListPorts(_ context.Context, _ uuid.UUID) ([]network.Port, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeletePort(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockNetworkSvc) CreateGatewayNode(_ context.Context, _ network.GatewayNodeSpec) (*network.GatewayNode, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetGatewayNode(_ context.Context, _ uuid.UUID) (*network.GatewayNode, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListGatewayNodes(_ context.Context) ([]network.GatewayNode, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeleteGatewayNode(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockNetworkSvc) AssignGatewayNode(_ context.Context, _, _ uuid.UUID) error {
	return nil
}
func (m *mockNetworkSvc) GetNetworkGatewayNode(_ context.Context, _ uuid.UUID) (*network.GatewayNode, error) {
	return nil, network.ErrNotFound
}

func (m *mockNetworkSvc) CreateEgress(_ context.Context, _ uuid.UUID, _ network.EgressSpec) (*network.Egress, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetEgress(_ context.Context, _ uuid.UUID) (*network.Egress, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListEgresses(_ context.Context, _ uuid.UUID) ([]network.Egress, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeleteEgress(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockNetworkSvc) CreateIPPool(_ context.Context, _ network.IPPoolSpec) (*network.IPPool, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetIPPool(_ context.Context, _ uuid.UUID) (*network.IPPool, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListIPPools(_ context.Context) ([]network.IPPool, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeleteIPPool(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockNetworkSvc) CreateIngress(_ context.Context, _ uuid.UUID, _ network.IngressSpec) (*network.Ingress, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetIngress(_ context.Context, _ uuid.UUID) (*network.Ingress, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListIngresses(_ context.Context, _ uuid.UUID) ([]network.Ingress, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeleteIngress(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockNetworkSvc) UpdateBackendHealth(_ context.Context, _, _ uuid.UUID, _ bool) error {
	return nil
}

func (m *mockNetworkSvc) CreateLoadBalancer(_ context.Context, _, _ uuid.UUID, _ network.LoadBalancerSpec) (*network.LoadBalancer, error) {
	return nil, nil
}
func (m *mockNetworkSvc) GetLoadBalancer(_ context.Context, _ uuid.UUID) (*network.LoadBalancer, error) {
	return nil, network.ErrNotFound
}
func (m *mockNetworkSvc) ListLoadBalancers(_ context.Context, _ uuid.UUID) ([]network.LoadBalancer, error) {
	return nil, nil
}
func (m *mockNetworkSvc) DeleteLoadBalancer(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockNetworkSvc) UpdateLBBackendHealth(_ context.Context, _, _ uuid.UUID, _ bool) error {
	return nil
}

// networkTestRouter builds a router wired with the given network service.
func networkTestRouter(svc network.Service) http.Handler {
	return api.NewRouter(nil, slog.Default(), &testAuthn{}, &testAuthz{}, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, false)
}

// networkReq sends a JSON request to the network router with optional X-Tenant-ID header.
func networkReq(handler http.Handler, method, path string, body any, tenantID *uuid.UUID) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	if tenantID != nil {
		req.Header.Set("X-Tenant-ID", tenantID.String())
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestNetwork_CreateNetwork_InvalidTenant verifies that when the network service returns
// ErrNotFound (tenant does not exist), the handler responds with 400 "invalid tenant".
func TestNetwork_CreateNetwork_InvalidTenant(t *testing.T) {
	svc := &mockNetworkSvc{createNetworkErr: network.ErrNotFound}
	r := networkTestRouter(svc)

	tid := uuid.New()
	w := networkReq(r, http.MethodPost, "/api/v1/networks", map[string]string{"name": "test-net"}, &tid)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid tenant" {
		t.Fatalf("want error %q, got %q", "invalid tenant", resp["error"])
	}
}

// TestNetwork_CreateNetwork_Success verifies that a valid network name + tenant → 201.
func TestNetwork_CreateNetwork_Success(t *testing.T) {
	svc := &mockNetworkSvc{}
	r := networkTestRouter(svc)

	tid := uuid.New()
	w := networkReq(r, http.MethodPost, "/api/v1/networks", map[string]string{"name": "my-net"}, &tid)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
	}
}

// TestNetwork_CreateNetwork_NoTenant verifies that omitting X-Tenant-ID → 400.
func TestNetwork_CreateNetwork_NoTenant(t *testing.T) {
	svc := &mockNetworkSvc{}
	r := networkTestRouter(svc)

	w := networkReq(r, http.MethodPost, "/api/v1/networks", map[string]string{"name": "my-net"}, nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 (no tenant), got %d: %s", w.Code, w.Body.String())
	}
}
