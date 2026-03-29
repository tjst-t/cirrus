package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/az"
)

// --- Availability Zones ---

func (c *Client) CreateAZ(ctx context.Context, name, description string, locationID uuid.UUID) (*az.AvailabilityZone, error) {
	body := struct {
		Name        string    `json:"name"`
		Description string    `json:"description,omitempty"`
		LocationID  uuid.UUID `json:"location_id"`
	}{Name: name, Description: description, LocationID: locationID}
	resp, err := c.do(ctx, "POST", "/api/v1/admin/availability-zones", body)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*az.AvailabilityZone](resp)
}

func (c *Client) ListAZs(ctx context.Context) ([]az.AvailabilityZone, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/availability-zones", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]az.AvailabilityZone](resp)
}

func (c *Client) ListAllAZs(ctx context.Context) ([]az.AvailabilityZone, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/admin/availability-zones", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]az.AvailabilityZone](resp)
}

func (c *Client) GetAZ(ctx context.Context, id uuid.UUID) (*az.AvailabilityZone, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/availability-zones/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*az.AvailabilityZone](resp)
}

func (c *Client) DeleteAZ(ctx context.Context, id uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/admin/availability-zones/%s", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ResolveAZ resolves an AZ by ID or name. Uses the admin endpoint to include disabled AZs.
func (c *Client) ResolveAZ(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	azs, err := c.ListAllAZs(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve az %q: %w", idOrName, err)
	}
	var matches []az.AvailabilityZone
	for _, a := range azs {
		if a.Name == idOrName {
			matches = append(matches, a)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("availability zone %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple availability zones named %q found, use ID instead", idOrName)
	}
}

func (c *Client) AddAZStorageDomain(ctx context.Context, azID, storageDomainID uuid.UUID) error {
	body := struct {
		StorageDomainID uuid.UUID `json:"storage_domain_id"`
	}{StorageDomainID: storageDomainID}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/admin/availability-zones/%s/storage-domains", azID), body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) RemoveAZStorageDomain(ctx context.Context, azID, storageDomainID uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/admin/availability-zones/%s/storage-domains/%s", azID, storageDomainID), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
