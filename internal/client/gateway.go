package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/network"
)

// --- Gateway Nodes ---

func (c *Client) CreateGatewayNode(ctx context.Context, spec network.GatewayNodeSpec) (*network.GatewayNode, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/admin/gateway-nodes", spec)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.GatewayNode](resp)
}

func (c *Client) ListGatewayNodes(ctx context.Context) ([]network.GatewayNode, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/admin/gateway-nodes", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]network.GatewayNode](resp)
}

func (c *Client) GetGatewayNode(ctx context.Context, id uuid.UUID) (*network.GatewayNode, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/admin/gateway-nodes/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.GatewayNode](resp)
}

func (c *Client) DeleteGatewayNode(ctx context.Context, id uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/admin/gateway-nodes/%s", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) AssignGatewayNodeToNetwork(ctx context.Context, networkID, gatewayNodeID uuid.UUID) error {
	body := struct {
		GatewayNodeID uuid.UUID `json:"gateway_node_id"`
	}{GatewayNodeID: gatewayNodeID}
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/admin/networks/%s/gateway", networkID), body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ResolveGatewayNode(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	nodes, err := c.ListGatewayNodes(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve gateway-node %q: %w", idOrName, err)
	}
	// Gateway nodes don't have names, so we look up by external_ip as a fallback.
	var matches []network.GatewayNode
	for _, n := range nodes {
		if n.ExternalIP == idOrName || n.InternalIP == idOrName {
			matches = append(matches, n)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("gateway-node %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple gateway-nodes match %q, use ID instead", idOrName)
	}
}

// --- IP Pools ---

func (c *Client) CreateIPPool(ctx context.Context, spec network.IPPoolSpec) (*network.IPPool, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/admin/ip-pools", spec)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.IPPool](resp)
}

func (c *Client) ListIPPools(ctx context.Context) ([]network.IPPool, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/admin/ip-pools", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]network.IPPool](resp)
}

func (c *Client) GetIPPool(ctx context.Context, id uuid.UUID) (*network.IPPool, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/admin/ip-pools/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.IPPool](resp)
}

func (c *Client) DeleteIPPool(ctx context.Context, id uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/admin/ip-pools/%s", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ResolveIPPool(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	pools, err := c.ListIPPools(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve ip-pool %q: %w", idOrName, err)
	}
	var matches []network.IPPool
	for _, p := range pools {
		if p.Name == idOrName {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("ip-pool %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple ip-pools named %q found, use ID instead", idOrName)
	}
}

// --- Egresses ---

func (c *Client) CreateEgress(ctx context.Context, tenantID, networkID uuid.UUID, spec network.EgressSpec) (*network.Egress, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/tenants/%s/networks/%s/egresses", tenantID, networkID), spec)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.Egress](resp)
}

func (c *Client) ListEgresses(ctx context.Context, tenantID, networkID uuid.UUID) ([]network.Egress, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s/networks/%s/egresses", tenantID, networkID), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]network.Egress](resp)
}

func (c *Client) GetEgress(ctx context.Context, tenantID, networkID, egressID uuid.UUID) (*network.Egress, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s/networks/%s/egresses/%s", tenantID, networkID, egressID), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.Egress](resp)
}

func (c *Client) DeleteEgress(ctx context.Context, tenantID, networkID, egressID uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/tenants/%s/networks/%s/egresses/%s", tenantID, networkID, egressID), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// --- Ingresses ---

func (c *Client) CreateIngress(ctx context.Context, tenantID, networkID uuid.UUID, spec network.IngressSpec) (*network.Ingress, error) {
	resp, err := c.doWithTenant(ctx, "POST", fmt.Sprintf("/api/v1/networks/%s/ingresses", networkID), spec, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.Ingress](resp)
}

func (c *Client) ListIngresses(ctx context.Context, tenantID, networkID uuid.UUID) ([]network.Ingress, error) {
	resp, err := c.doWithTenant(ctx, "GET", fmt.Sprintf("/api/v1/networks/%s/ingresses", networkID), nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]network.Ingress](resp)
}

func (c *Client) GetIngress(ctx context.Context, tenantID, networkID, ingressID uuid.UUID) (*network.Ingress, error) {
	resp, err := c.doWithTenant(ctx, "GET", fmt.Sprintf("/api/v1/networks/%s/ingresses/%s", networkID, ingressID), nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.Ingress](resp)
}

func (c *Client) DeleteIngress(ctx context.Context, tenantID, networkID, ingressID uuid.UUID) error {
	resp, err := c.doWithTenant(ctx, "DELETE", fmt.Sprintf("/api/v1/networks/%s/ingresses/%s", networkID, ingressID), nil, tenantID)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
