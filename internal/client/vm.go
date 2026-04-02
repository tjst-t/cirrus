package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/compute"
)

type CreateVMRequest struct {
	Name         string  `json:"name"`
	FlavorID     string  `json:"flavor_id"`
	AZID         string  `json:"az_id,omitempty"`
	NetworkID    string  `json:"network_id,omitempty"`
	VolumeTypeID *string `json:"volume_type_id,omitempty"`
}

func (c *Client) CreateVM(ctx context.Context, tenantID uuid.UUID, req CreateVMRequest) (*compute.VM, error) {
	resp, err := c.doWithTenant(ctx, "POST", "/api/v1/vms", req, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*compute.VM](resp)
}

func (c *Client) ListVMs(ctx context.Context, tenantID uuid.UUID) ([]compute.VM, error) {
	resp, err := c.doWithTenant(ctx, "GET", "/api/v1/vms", nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]compute.VM](resp)
}

func (c *Client) GetVM(ctx context.Context, tenantID, vmID uuid.UUID) (*compute.VM, error) {
	resp, err := c.doWithTenant(ctx, "GET", fmt.Sprintf("/api/v1/vms/%s", vmID), nil, tenantID)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*compute.VM](resp)
}

func (c *Client) DeleteVM(ctx context.Context, tenantID, vmID uuid.UUID) error {
	resp, err := c.doWithTenant(ctx, "DELETE", fmt.Sprintf("/api/v1/vms/%s", vmID), nil, tenantID)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ResolveVM resolves a VM by UUID or name within a tenant.
func (c *Client) ResolveVM(ctx context.Context, tenantID uuid.UUID, nameOrID string) (*compute.VM, error) {
	if id, err := uuid.Parse(nameOrID); err == nil {
		return c.GetVM(ctx, tenantID, id)
	}
	vms, err := c.ListVMs(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	var matches []compute.VM
	for _, vm := range vms {
		if vm.Name == nameOrID {
			matches = append(matches, vm)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("vm %q not found", nameOrID)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("multiple VMs named %q — specify by UUID", nameOrID)
	}
}
