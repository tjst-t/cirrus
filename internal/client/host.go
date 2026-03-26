package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/host"
)

// ResolveHost resolves a host identifier (UUID or name) to an ID.
func (c *Client) ResolveHost(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	hosts, err := c.ListHosts(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve host %q: %w", idOrName, err)
	}
	var matches []host.Host
	for _, h := range hosts {
		if h.Name == idOrName {
			matches = append(matches, h)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("host %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple hosts named %q found, use ID instead", idOrName)
	}
}

// CreateHost registers a new host.
func (c *Client) CreateHost(ctx context.Context, name, address string) (*host.Host, error) {
	body := struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}{Name: name, Address: address}
	resp, err := c.do(ctx, "POST", "/api/v1/hosts", body)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*host.Host](resp)
}

// ListHosts returns all hosts.
func (c *Client) ListHosts(ctx context.Context) ([]host.Host, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/hosts", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]host.Host](resp)
}

// GetHost returns a host by ID.
func (c *Client) GetHost(ctx context.Context, id uuid.UUID) (*host.Host, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/hosts/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*host.Host](resp)
}

// HostAction performs an operational action on a host (maintenance, activate, drain, retire).
func (c *Client) HostAction(ctx context.Context, id uuid.UUID, action string) (*host.Host, error) {
	body := struct {
		Action string `json:"action"`
	}{Action: action}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/hosts/%s/actions", id), body)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*host.Host](resp)
}
