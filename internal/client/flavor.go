package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/flavor"
)

type CreateFlavorRequest struct {
	Name     string `json:"name"`
	VCPUs    int    `json:"vcpus"`
	RAMMB    int64  `json:"ram_mb"`
	DiskGB   int64  `json:"disk_gb"`
	IsPublic *bool  `json:"is_public,omitempty"`
}

func (c *Client) CreateFlavor(ctx context.Context, req CreateFlavorRequest) (*flavor.Flavor, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/admin/flavors", req)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*flavor.Flavor](resp)
}

func (c *Client) ListFlavors(ctx context.Context) ([]flavor.Flavor, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/flavors", nil)
	if err != nil {
		return nil, err
	}
	return decodePagedResponse[flavor.Flavor](resp)
}

func (c *Client) GetFlavor(ctx context.Context, id uuid.UUID) (*flavor.Flavor, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/flavors/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*flavor.Flavor](resp)
}

func (c *Client) DeleteFlavor(ctx context.Context, id uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/admin/flavors/%s", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ResolveFlavor(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	flavors, err := c.ListFlavors(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var matches []flavor.Flavor
	for _, f := range flavors {
		if f.Name == idOrName {
			matches = append(matches, f)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("flavor %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple flavors named %q; specify UUID", idOrName)
	}
}
