package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/identity"
)

// ResolveOrganization resolves an org identifier (UUID or name) to an ID.
// TODO: サーバー側に ?name= フィルタが入ったら全件取得をやめて切り替える
func (c *Client) ResolveOrganization(ctx context.Context, idOrName string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	orgs, err := c.ListOrganizations(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve organization %q: %w", idOrName, err)
	}
	var matches []identity.Organization
	for _, o := range orgs {
		if o.Name == idOrName {
			matches = append(matches, o)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("organization %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple organizations named %q found, use ID instead", idOrName)
	}
}

// ResolveTenant resolves a tenant identifier (UUID or name) to an ID.
// When resolving by name, orgID is required to scope the search.
// TODO: サーバー側に ?name= フィルタが入ったら全件取得をやめて切り替える
func (c *Client) ResolveTenant(ctx context.Context, idOrName string, orgID *uuid.UUID) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	if orgID == nil {
		return uuid.Nil, fmt.Errorf("resolving tenant by name %q requires an organization (use --org flag or pass UUID)", idOrName)
	}
	tenants, err := c.ListTenants(ctx, *orgID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve tenant %q: %w", idOrName, err)
	}
	var matches []identity.Tenant
	for _, t := range tenants {
		if t.Name == idOrName {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.Nil, fmt.Errorf("tenant %q not found in organization %s", idOrName, orgID)
	case 1:
		return matches[0].ID, nil
	default:
		return uuid.Nil, fmt.Errorf("multiple tenants named %q found, use ID instead", idOrName)
	}
}

// CreateOrganization creates a new organization.
func (c *Client) CreateOrganization(ctx context.Context, name string) (*identity.Organization, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/organizations", map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	return decodeResponse[*identity.Organization](resp)
}

// ListOrganizations returns all organizations.
func (c *Client) ListOrganizations(ctx context.Context) ([]identity.Organization, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/organizations", nil)
	if err != nil {
		return nil, err
	}
	return decodePagedResponse[identity.Organization](resp)
}

// GetOrganization returns an organization by ID.
func (c *Client) GetOrganization(ctx context.Context, id uuid.UUID) (*identity.Organization, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/organizations/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*identity.Organization](resp)
}

// CreateTenant creates a new tenant within an organization.
func (c *Client) CreateTenant(ctx context.Context, orgID uuid.UUID, name string) (*identity.Tenant, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/organizations/%s/tenants", orgID), map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	return decodeResponse[*identity.Tenant](resp)
}

// ListTenants returns all tenants within an organization.
func (c *Client) ListTenants(ctx context.Context, orgID uuid.UUID) ([]identity.Tenant, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/organizations/%s/tenants", orgID), nil)
	if err != nil {
		return nil, err
	}
	return decodePagedResponse[identity.Tenant](resp)
}

// GetTenant returns a tenant by ID.
func (c *Client) GetTenant(ctx context.Context, id uuid.UUID) (*identity.Tenant, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*identity.Tenant](resp)
}

// AssignRole assigns a role to a user within a tenant.
func (c *Client) AssignRole(ctx context.Context, tenantID, userID uuid.UUID, role identity.Role) (*identity.RoleAssignment, error) {
	body := struct {
		UserID uuid.UUID     `json:"user_id"`
		Role   identity.Role `json:"role"`
	}{
		UserID: userID,
		Role:   role,
	}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/tenants/%s/role-assignments", tenantID), body)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*identity.RoleAssignment](resp)
}

// ListRoleAssignments returns all role assignments for a tenant.
func (c *Client) ListRoleAssignments(ctx context.Context, tenantID uuid.UUID) ([]identity.RoleAssignment, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s/role-assignments", tenantID), nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[[]identity.RoleAssignment](resp)
}

// DeleteRoleAssignment deletes a role assignment from a tenant.
func (c *Client) DeleteRoleAssignment(ctx context.Context, tenantID, assignmentID uuid.UUID) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/tenants/%s/role-assignments/%s", tenantID, assignmentID), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
