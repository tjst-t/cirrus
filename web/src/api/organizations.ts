import { api } from './client'

export interface Organization {
  id: string
  name: string
  created_at: string
}

export interface Tenant {
  id: string
  name: string
  organization_id: string
  created_at: string
}

export interface RoleAssignment {
  user_id: string
  role: string
  tenant_id: string
}

export interface CreateOrganizationRequest {
  name: string
}

export interface CreateTenantRequest {
  name: string
}

export interface CreateRoleAssignmentRequest {
  user_id: string
  role: string
}

export const organizationsApi = {
  // Intentionally uses the shared /organizations endpoint (RBAC handles access control for both admin and tenant roles)
  list: () => api.list<Organization>('/organizations'),
  // Intentionally uses the shared /organizations endpoint (RBAC handles access control for both admin and tenant roles)
  get: (orgId: string) => api.get<Organization>(`/organizations/${orgId}`),
  create: (data: CreateOrganizationRequest) =>
    api.post<Organization>('/organizations', data),
  listTenants: (orgId: string) =>
    api.list<Tenant>(`/organizations/${orgId}/tenants`),
  createTenant: (orgId: string, data: CreateTenantRequest) =>
    api.post<Tenant>(`/organizations/${orgId}/tenants`, data),
  listRoleAssignments: (tenantId: string) =>
    api.get<RoleAssignment[]>(`/tenants/${tenantId}/role-assignments`),
  createRoleAssignment: (tenantId: string, data: CreateRoleAssignmentRequest) =>
    api.post<RoleAssignment>(`/tenants/${tenantId}/role-assignments`, data),
  deleteRoleAssignment: (tenantId: string, userId: string) =>
    api.delete<void>(`/tenants/${tenantId}/role-assignments/${userId}`),
}
