//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/network"
)

// helper returns a client, endpoint, token, and tenantID from env, skipping if not set.
func egressIngressEnv(t *testing.T) (*client.Client, uuid.UUID) {
	t.Helper()
	endpoint := os.Getenv("CIRRUS_ENDPOINT")
	if endpoint == "" {
		t.Skip("CIRRUS_ENDPOINT not set; skipping egress/ingress integration test")
	}
	token := os.Getenv("CIRRUS_TOKEN")
	if token == "" {
		t.Fatal("CIRRUS_TOKEN not set")
	}
	tenantIDStr := os.Getenv("CIRRUS_TENANT_ID")
	if tenantIDStr == "" {
		t.Fatal("CIRRUS_TENANT_ID not set")
	}
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		t.Fatalf("invalid CIRRUS_TENANT_ID: %v", err)
	}
	return client.New(endpoint, token), tenantID
}

// TestNATGatewayEgress verifies the full egress (nat_gateway) lifecycle.
func TestNATGatewayEgress(t *testing.T) {
	c, tenantID := egressIngressEnv(t)
	ctx := context.Background()

	// Create a network for this test.
	netName := fmt.Sprintf("egress-test-%d", time.Now().Unix())
	net, err := c.CreateNetwork(ctx, tenantID, netName, "")
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	t.Cleanup(func() {
		_ = c.DeleteNetwork(ctx, net.ID)
	})
	t.Logf("created network %s (%s)", net.Name, net.ID)

	// We need a gateway node — create a host first, then a GW node.
	hostIDStr := os.Getenv("CIRRUS_HOST_ID")
	if hostIDStr == "" {
		t.Skip("CIRRUS_HOST_ID not set; skipping GW-dependent test")
	}
	hostID, err := uuid.Parse(hostIDStr)
	if err != nil {
		t.Fatalf("invalid CIRRUS_HOST_ID: %v", err)
	}

	gw, err := c.CreateGatewayNode(ctx, network.GatewayNodeSpec{
		HostID:     hostID,
		ExternalIP: "203.0.113.254",
		InternalIP: "10.255.0.1",
	})
	if err != nil {
		t.Fatalf("create gateway node: %v", err)
	}
	t.Cleanup(func() {
		_ = c.DeleteGatewayNode(ctx, gw.ID)
	})
	t.Logf("created gateway node %s", gw.ID)

	// Assign gateway node to network.
	if err := c.AssignGatewayNodeToNetwork(ctx, net.ID, gw.ID); err != nil {
		t.Fatalf("assign gateway node: %v", err)
	}

	// Create egress.
	egress, err := c.CreateEgress(ctx, tenantID, net.ID, network.EgressSpec{
		Type:   "nat_gateway",
		Config: network.EgressConfig{PublicIP: "203.0.113.1"},
	})
	if err != nil {
		t.Fatalf("create egress: %v", err)
	}
	t.Logf("created egress %s with public_ip=%s", egress.ID, egress.Config.PublicIP)

	// GET egress — verify stored correctly.
	fetched, err := c.GetEgress(ctx, tenantID, net.ID, egress.ID)
	if err != nil {
		t.Fatalf("get egress: %v", err)
	}
	if fetched.Config.PublicIP != "203.0.113.1" {
		t.Errorf("expected public_ip=203.0.113.1, got %s", fetched.Config.PublicIP)
	}
	if fetched.Type != "nat_gateway" {
		t.Errorf("expected type=nat_gateway, got %s", fetched.Type)
	}

	// DELETE egress.
	if err := c.DeleteEgress(ctx, tenantID, net.ID, egress.ID); err != nil {
		t.Fatalf("delete egress: %v", err)
	}

	// Subsequent GET should return 404.
	_, err = c.GetEgress(ctx, tenantID, net.ID, egress.ID)
	if err == nil {
		t.Fatal("expected 404 after delete, got nil error")
	}
	t.Logf("correctly got error after delete: %v", err)
}

// TestDirectIPIngress verifies the full ingress (direct_ip) lifecycle.
func TestDirectIPIngress(t *testing.T) {
	c, tenantID := egressIngressEnv(t)
	ctx := context.Background()

	// Create a network.
	netName := fmt.Sprintf("ingress-test-%d", time.Now().Unix())
	net, err := c.CreateNetwork(ctx, tenantID, netName, "")
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	t.Cleanup(func() {
		_ = c.DeleteNetwork(ctx, net.ID)
	})

	// Admin: create IP pool.
	poolName := fmt.Sprintf("test-pool-%d", time.Now().Unix())
	pool, err := c.CreateIPPool(ctx, network.IPPoolSpec{
		Name: poolName,
		CIDR: "203.0.113.0/24",
	})
	if err != nil {
		t.Fatalf("create ip pool: %v", err)
	}
	t.Cleanup(func() {
		_ = c.DeleteIPPool(ctx, pool.ID)
	})
	t.Logf("created ip pool %s (%s)", pool.Name, pool.ID)

	// Need a GW node.
	hostIDStr := os.Getenv("CIRRUS_HOST_ID")
	if hostIDStr == "" {
		t.Skip("CIRRUS_HOST_ID not set; skipping GW-dependent test")
	}
	hostID, err := uuid.Parse(hostIDStr)
	if err != nil {
		t.Fatalf("invalid CIRRUS_HOST_ID: %v", err)
	}

	gw, err := c.CreateGatewayNode(ctx, network.GatewayNodeSpec{
		HostID:     hostID,
		ExternalIP: "203.0.113.253",
		InternalIP: "10.255.0.2",
	})
	if err != nil {
		t.Fatalf("create gateway node: %v", err)
	}
	t.Cleanup(func() {
		_ = c.DeleteGatewayNode(ctx, gw.ID)
	})

	if err := c.AssignGatewayNodeToNetwork(ctx, net.ID, gw.ID); err != nil {
		t.Fatalf("assign gateway node: %v", err)
	}

	// Use a fake VM ID for DNAT target.
	targetVMID := uuid.New()

	// Create ingress with public_ip inside pool CIDR.
	ingress, err := c.CreateIngress(ctx, tenantID, net.ID, network.IngressSpec{
		Type:     "direct_ip",
		PublicIP: "203.0.113.10",
		IPPoolID: pool.ID,
		Config: network.IngressConfig{
			TargetVMID: targetVMID.String(),
			TargetIP:   "10.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("create ingress: %v", err)
	}
	t.Logf("created ingress %s with public_ip=%s", ingress.ID, ingress.PublicIP)

	// GET ingress — verify.
	fetched, err := c.GetIngress(ctx, tenantID, net.ID, ingress.ID)
	if err != nil {
		t.Fatalf("get ingress: %v", err)
	}
	if fetched.PublicIP != "203.0.113.10" {
		t.Errorf("expected public_ip=203.0.113.10, got %s", fetched.PublicIP)
	}
	if fetched.Config.TargetIP != "10.0.0.1" {
		t.Errorf("expected target_ip=10.0.0.1, got %s", fetched.Config.TargetIP)
	}

	// Test with public_ip outside pool CIDR — should fail.
	_, err = c.CreateIngress(ctx, tenantID, net.ID, network.IngressSpec{
		Type:     "direct_ip",
		PublicIP: "198.51.100.1", // outside 203.0.113.0/24
		IPPoolID: pool.ID,
		Config: network.IngressConfig{
			TargetVMID: targetVMID.String(),
			TargetIP:   "10.0.0.2",
		},
	})
	if err == nil {
		t.Fatal("expected error for public_ip outside pool CIDR, got nil")
	}
	t.Logf("correctly rejected out-of-pool IP: %v", err)

	// DELETE ingress.
	if err := c.DeleteIngress(ctx, tenantID, net.ID, ingress.ID); err != nil {
		t.Fatalf("delete ingress: %v", err)
	}

	// Subsequent GET should return 404.
	_, err = c.GetIngress(ctx, tenantID, net.ID, ingress.ID)
	if err == nil {
		t.Fatal("expected 404 after delete, got nil error")
	}
	t.Logf("correctly got error after delete: %v", err)
}

// TestGatewayNodeCRUD verifies the GatewayNode admin CRUD lifecycle.
func TestGatewayNodeCRUD(t *testing.T) {
	c, _ := egressIngressEnv(t)
	ctx := context.Background()

	hostIDStr := os.Getenv("CIRRUS_HOST_ID")
	if hostIDStr == "" {
		t.Skip("CIRRUS_HOST_ID not set; skipping gateway node CRUD test")
	}
	hostID, err := uuid.Parse(hostIDStr)
	if err != nil {
		t.Fatalf("invalid CIRRUS_HOST_ID: %v", err)
	}

	// POST /admin/gateway-nodes
	gw, err := c.CreateGatewayNode(ctx, network.GatewayNodeSpec{
		HostID:     hostID,
		ExternalIP: "203.0.113.100",
		InternalIP: "10.255.1.1",
	})
	if err != nil {
		t.Fatalf("create gateway node: %v", err)
	}
	t.Logf("created gateway node %s", gw.ID)

	// GET /admin/gateway-nodes — list includes new node.
	nodes, err := c.ListGatewayNodes(ctx)
	if err != nil {
		t.Fatalf("list gateway nodes: %v", err)
	}
	found := false
	for _, n := range nodes {
		if n.ID == gw.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created gateway node %s not found in list", gw.ID)
	}

	// DELETE /admin/gateway-nodes/{id}
	if err := c.DeleteGatewayNode(ctx, gw.ID); err != nil {
		t.Fatalf("delete gateway node: %v", err)
	}

	// GET /admin/gateway-nodes/{id} — should return 404.
	_, err = c.GetGatewayNode(ctx, gw.ID)
	if err == nil {
		t.Fatal("expected 404 after delete, got nil error")
	}
	t.Logf("correctly got error after delete: %v", err)
}
