import { api } from './client'

export interface Network {
  id: string
  name: string
  cidr: string
  status: string
  created_at: string
}

export interface CreateNetworkRequest {
  name: string
  cidr: string
}

export interface NetworkGroup {
  id: string
  network_id: string
  name: string
  description?: string
  created_at: string
}

export interface CreateNetworkGroupRequest {
  name: string
  description?: string
}

export interface NetworkPolicy {
  id: string
  network_id: string
  name: string
  direction: 'ingress' | 'egress'
  protocol: string
  port_range_min?: number
  port_range_max?: number
  remote_cidr?: string
  action: 'allow' | 'deny'
  created_at: string
}

export interface CreateNetworkPolicyRequest {
  name: string
  direction: 'ingress' | 'egress'
  protocol: string
  port_range_min?: number
  port_range_max?: number
  remote_cidr?: string
  action: 'allow' | 'deny'
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
