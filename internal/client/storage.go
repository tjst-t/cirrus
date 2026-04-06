package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/storage"
)

// --- Storage Backends (admin) ---

type RegisterBackendRequest struct {
	StorageDomainID uuid.UUID      `json:"storage_domain_id"`
	Name            string         `json:"name"`
	Driver          string         `json:"driver"`
	Endpoint        string         `json:"endpoint"`
	TotalCapacityGB int64          `json:"total_capacity_gb"`
	TotalIOPS       int64          `json:"total_iops"`
	Capabilities    []string       `json:"capabilities"`
	DriverConfig    map[string]any `json:"driver_config"`
}

func (c *Client) RegisterStorageBackend(ctx context.Context, req RegisterBackendRequest) (*storage.Backend, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/admin/storage-backends", req)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*storage.Backend](resp)
}

func (c *Client) ListStorageBackends(ctx context.Context) ([]storage.Backend, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/admin/storage-backends", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]storage.Backend](resp)
}

func (c *Client) GetStorageBackend(ctx context.Context, id uuid.UUID) (*storage.Backend, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/admin/storage-backends/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*storage.Backend](resp)
}

func (c *Client) DrainStorageBackend(ctx context.Context, id uuid.UUID) error {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/admin/storage-backends/%s/drain", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ResolveStorageBackend(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	backends, err := c.ListStorageBackends(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve storage backend %q: %w", idOrName, err)
	}
	var matches []storage.Backend
	for _, b := range backends {
		if b.Name == idOrName {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("storage backend %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("storage backend name %q matches multiple backends; specify UUID", idOrName)
	}
}

// --- Volume Types ---

type CreateVolumeTypeRequest struct {
	Name                 string         `json:"name"`
	Description          string         `json:"description,omitempty"`
	RequiredCapabilities []string       `json:"required_capabilities"`
	QoSPolicy            map[string]any `json:"qos_policy,omitempty"`
	IsPublic             bool           `json:"is_public"`
}

func (c *Client) CreateVolumeType(ctx context.Context, req CreateVolumeTypeRequest) (*storage.VolumeType, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/admin/volume-types", req)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*storage.VolumeType](resp)
}

func (c *Client) ListVolumeTypes(ctx context.Context) ([]storage.VolumeType, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/volume-types", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]storage.VolumeType](resp)
}

func (c *Client) GetVolumeType(ctx context.Context, id uuid.UUID) (*storage.VolumeType, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/volume-types/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*storage.VolumeType](resp)
}

func (c *Client) ResolveVolumeType(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	vts, err := c.ListVolumeTypes(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve volume type %q: %w", idOrName, err)
	}
	var matches []storage.VolumeType
	for _, vt := range vts {
		if vt.Name == idOrName {
			matches = append(matches, vt)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("volume type %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("volume type name %q matches multiple; specify UUID", idOrName)
	}
}

// --- Volumes (tenant-scoped) ---

type CreateVolumeRequest struct {
	Name         string     `json:"name"`
	VolumeTypeID *uuid.UUID `json:"volume_type_id,omitempty"`
	SizeGB       int64      `json:"size_gb"`
	AZID         *uuid.UUID `json:"az_id,omitempty"`
}

func (c *Client) CreateVolume(ctx context.Context, tenantID uuid.UUID, req CreateVolumeRequest) (*storage.Volume, error) {
	resp, err := c.doWithTenant(ctx, "POST", "/api/v1/volumes", req, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*storage.Volume](resp)
}

func (c *Client) ListVolumes(ctx context.Context, tenantID uuid.UUID) ([]storage.Volume, error) {
	resp, err := c.doWithTenant(ctx, "GET", "/api/v1/volumes", nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodePagedResponse[storage.Volume](resp)
}

func (c *Client) GetVolume(ctx context.Context, tenantID, volumeID uuid.UUID) (*storage.Volume, error) {
	resp, err := c.doWithTenant(ctx, "GET", fmt.Sprintf("/api/v1/volumes/%s", volumeID), nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*storage.Volume](resp)
}

func (c *Client) DeleteVolume(ctx context.Context, tenantID, volumeID uuid.UUID) error {
	resp, err := c.doWithTenant(ctx, "DELETE", fmt.Sprintf("/api/v1/volumes/%s", volumeID), nil, tenantID)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ResizeVolume(ctx context.Context, tenantID, volumeID uuid.UUID, newSizeGB int64) (*storage.Volume, error) {
	resp, err := c.doWithTenant(ctx, "POST", fmt.Sprintf("/api/v1/volumes/%s/resize", volumeID),
		map[string]int64{"new_size_gb": newSizeGB}, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*storage.Volume](resp)
}

func (c *Client) ResolveVolume(ctx context.Context, tenantID uuid.UUID, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	vs, err := c.ListVolumes(ctx, tenantID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve volume %q: %w", idOrName, err)
	}
	var matches []storage.Volume
	for _, v := range vs {
		if v.Name == idOrName {
			matches = append(matches, v)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("volume %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("volume name %q matches multiple; specify UUID", idOrName)
	}
}
