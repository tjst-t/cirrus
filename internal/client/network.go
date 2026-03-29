package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/network"
)

// --- Networks ---

func (c *Client) CreateNetwork(ctx context.Context, tenantID uuid.UUID, name string) (*network.Network, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	resp, err := c.doWithTenant(ctx, "POST", "/api/v1/networks", body, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.Network](resp)
}

func (c *Client) ListNetworks(ctx context.Context, tenantID uuid.UUID) ([]network.Network, error) {
	resp, err := c.doWithTenant(ctx, "GET", "/api/v1/networks", nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]network.Network](resp)
}

func (c *Client) GetNetwork(ctx context.Context, id uuid.UUID) (*network.Network, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/networks/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.Network](resp)
}

func (c *Client) DeleteNetwork(ctx context.Context, id uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/networks/%s", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ResolveNetwork(ctx context.Context, idOrName string, tenantID uuid.UUID) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	networks, err := c.ListNetworks(ctx, tenantID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve network %q: %w", idOrName, err)
	}
	var matches []network.Network
	for _, n := range networks {
		if n.Name == idOrName {
			matches = append(matches, n)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("network %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple networks named %q found, use ID instead", idOrName)
	}
}

// --- Ports (read-only) ---

func (c *Client) ListPorts(ctx context.Context, tenantID, networkID uuid.UUID) ([]network.Port, error) {
	resp, err := c.doWithTenant(ctx, "GET", fmt.Sprintf("/api/v1/ports?network_id=%s", networkID), nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]network.Port](resp)
}

func (c *Client) GetPort(ctx context.Context, id uuid.UUID) (*network.Port, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/ports/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*network.Port](resp)
}
