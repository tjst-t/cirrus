import { api } from './client'

export interface Port {
  id: string
  tenant_id: string
  network_id: string
  vm_id?: string
  mac_address?: string
  ip_address?: string
  role?: string
  status?: string
  created_at: string
}

export interface Network {
  id: string
  tenant_id: string
  name: string
  cidr: string
  vni?: number
  status: string
  created_at: string
  updated_at: string
}

export interface CreateNetworkRequest {
  name: string
  cidr?: string
}

export interface NetworkGroup {
  id: string
  network_id: string
  name: string
  created_at: string
}

export interface CreateNetworkGroupRequest {
  name: string
}

export interface NetworkPolicy {
  id: string
  network_id: string
  src_group_id: string
  dst_group_id: string
  protocol: string
  dst_port?: number
  priority: number
  action: string
  created_at: string
}

export interface CreateNetworkPolicyRequest {
  src_group_id: string
  dst_group_id: string
  protocol: string
  dst_port?: number
  priority?: number
  action?: string
}

export const networksApi = {
  list: () => api.list<Network>('/networks'),
  create: (req: CreateNetworkRequest) => api.post<Network>('/networks', req),
  delete: (id: string) => api.delete<void>(`/networks/${id}`),

  listGroups: (networkId: string) =>
    api.list<NetworkGroup>(`/networks/${networkId}/groups`),
  createGroup: (networkId: string, req: CreateNetworkGroupRequest) =>
    api.post<NetworkGroup>(`/networks/${networkId}/groups`, req),
  deleteGroup: (networkId: string, groupId: string) =>
    api.delete<void>(`/networks/${networkId}/groups/${groupId}`),

  listPolicies: (networkId: string) =>
    api.list<NetworkPolicy>(`/networks/${networkId}/policies`),
  createPolicy: (networkId: string, req: CreateNetworkPolicyRequest) =>
    api.post<NetworkPolicy>(`/networks/${networkId}/policies`, req),
  deletePolicy: (networkId: string, policyId: string) =>
    api.delete<void>(`/networks/${networkId}/policies/${policyId}`),
}
