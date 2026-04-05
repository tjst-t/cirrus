package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/quota"
)

// QuotaResponse is the combined limits + usage response body from the API.
type QuotaResponse struct {
	Limits *quota.Limits `json:"limits"`
	Usage  *quota.Usage  `json:"usage,omitempty"`
}

// GetTenantQuota returns the quota limits and current usage for a tenant.
func (c *Client) GetTenantQuota(ctx context.Context, tenantID uuid.UUID) (*QuotaResponse, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/tenants/%s/quota", tenantID), nil)
	if err != nil {
		return nil, fmt.Errorf("get tenant quota: %w", err)
	}
	return decodeResponse[*QuotaResponse](resp)
}

// SetTenantQuota updates the quota limits for a tenant.
func (c *Client) SetTenantQuota(ctx context.Context, tenantID uuid.UUID, limits quota.Limits) (*QuotaResponse, error) {
	resp, err := c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/tenants/%s/quota", tenantID), limits)
	if err != nil {
		return nil, fmt.Errorf("set tenant quota: %w", err)
	}
	return decodeResponse[*QuotaResponse](resp)
}

// GetOrgQuota returns the quota limits for an organization.
func (c *Client) GetOrgQuota(ctx context.Context, orgID uuid.UUID) (*QuotaResponse, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/organizations/%s/quota", orgID), nil)
	if err != nil {
		return nil, fmt.Errorf("get org quota: %w", err)
	}
	return decodeResponse[*QuotaResponse](resp)
}

// SetOrgQuota updates the quota limits for an organization.
func (c *Client) SetOrgQuota(ctx context.Context, orgID uuid.UUID, limits quota.Limits) (*QuotaResponse, error) {
	resp, err := c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/organizations/%s/quota", orgID), limits)
	if err != nil {
		return nil, fmt.Errorf("set org quota: %w", err)
	}
	return decodeResponse[*QuotaResponse](resp)
}
