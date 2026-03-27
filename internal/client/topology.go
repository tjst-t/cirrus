package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/topology"
)

// --- Storage domains ---

func (c *Client) CreateStorageDomain(ctx context.Context, name string) (*topology.StorageDomain, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	resp, err := c.do(ctx, "POST", "/api/v1/storage-domains", body)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.StorageDomain](resp)
}

func (c *Client) ListStorageDomains(ctx context.Context) ([]topology.StorageDomain, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/storage-domains", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]topology.StorageDomain](resp)
}

func (c *Client) GetStorageDomain(ctx context.Context, id uuid.UUID) (*topology.StorageDomain, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/storage-domains/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.StorageDomain](resp)
}

func (c *Client) ResolveStorageDomain(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	domains, err := c.ListStorageDomains(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve storage domain %q: %w", idOrName, err)
	}
	var matches []topology.StorageDomain
	for _, d := range domains {
		if d.Name == idOrName {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("storage domain %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple storage domains named %q found, use ID instead", idOrName)
	}
}

// --- Network domains ---

func (c *Client) CreateNetworkDomain(ctx context.Context, name, ovnNBConnection string) (*topology.NetworkDomain, error) {
	body := struct {
		Name            string `json:"name"`
		OVNNBConnection string `json:"ovn_nb_connection"`
	}{Name: name, OVNNBConnection: ovnNBConnection}
	resp, err := c.do(ctx, "POST", "/api/v1/network-domains", body)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.NetworkDomain](resp)
}

func (c *Client) ListNetworkDomains(ctx context.Context) ([]topology.NetworkDomain, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/network-domains", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]topology.NetworkDomain](resp)
}

func (c *Client) GetNetworkDomain(ctx context.Context, id uuid.UUID) (*topology.NetworkDomain, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/network-domains/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.NetworkDomain](resp)
}

func (c *Client) ResolveNetworkDomain(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	domains, err := c.ListNetworkDomains(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve network domain %q: %w", idOrName, err)
	}
	var matches []topology.NetworkDomain
	for _, d := range domains {
		if d.Name == idOrName {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("network domain %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple network domains named %q found, use ID instead", idOrName)
	}
}

// --- Locations ---

func (c *Client) CreateLocation(ctx context.Context, parentID *uuid.UUID, name, locType string, faultAttrs []byte) (*topology.Location, error) {
	body := struct {
		ParentID        *uuid.UUID      `json:"parent_id,omitempty"`
		Name            string          `json:"name"`
		Type            string          `json:"type"`
		FaultAttributes json.RawMessage `json:"fault_attributes,omitempty"`
	}{ParentID: parentID, Name: name, Type: locType, FaultAttributes: faultAttrs}
	resp, err := c.do(ctx, "POST", "/api/v1/locations", body)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.Location](resp)
}

func (c *Client) ListLocations(ctx context.Context) ([]topology.Location, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/locations", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]topology.Location](resp)
}

func (c *Client) GetLocation(ctx context.Context, id uuid.UUID) (*topology.Location, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/locations/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.Location](resp)
}

func (c *Client) GetLocationPath(ctx context.Context, id uuid.UUID) ([]topology.Location, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/locations/%s/path", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]topology.Location](resp)
}

func (c *Client) GetLocationTree(ctx context.Context, id uuid.UUID) (*topology.Location, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/locations/%s/tree", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.Location](resp)
}

func (c *Client) ResolveLocation(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	locations, err := c.ListLocations(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve location %q: %w", idOrName, err)
	}
	var matches []topology.Location
	for _, l := range locations {
		if l.Name == idOrName {
			matches = append(matches, l)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("location %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple locations named %q found, use ID instead", idOrName)
	}
}

// --- Host topology associations ---

func (c *Client) AssociateHostStorageDomain(ctx context.Context, hostID, storageDomainID uuid.UUID) error {
	body := struct {
		StorageDomainID uuid.UUID `json:"storage_domain_id"`
	}{StorageDomainID: storageDomainID}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/hosts/%s/storage-domains", hostID), body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) DissociateHostStorageDomain(ctx context.Context, hostID, storageDomainID uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/hosts/%s/storage-domains/%s", hostID, storageDomainID), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) SetHostNetworkDomain(ctx context.Context, hostID, networkDomainID uuid.UUID) error {
	body := struct {
		NetworkDomainID uuid.UUID `json:"network_domain_id"`
	}{NetworkDomainID: networkDomainID}
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/hosts/%s/network-domain", hostID), body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) SetHostLocation(ctx context.Context, hostID, locationID uuid.UUID) error {
	body := struct {
		LocationID uuid.UUID `json:"location_id"`
	}{LocationID: locationID}
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/hosts/%s/location", hostID), body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// --- Compute pools ---

func (c *Client) GetComputePool(ctx context.Context, storageDomainID, networkDomainID uuid.UUID) (*topology.ComputePool, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/compute-pools?storage_domain_id=%s&network_domain_id=%s", storageDomainID, networkDomainID), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*topology.ComputePool](resp)
}
