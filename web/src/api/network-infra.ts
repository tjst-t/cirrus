import { api } from './client'

export interface GatewayNode {
  id: string
  host_id: string
  external_ip: string
  internal_ip: string
  uplink_port?: string
  status: string
  created_at: string
}

export interface CreateGatewayNodeRequest {
  host_id: string
  external_ip: string
  internal_ip: string
}

export interface IPPool {
  id: string
  name: string
  cidr: string
  description: string
  created_at: string
}

export interface CreateIPPoolRequest {
  name: string
  cidr: string
}

export const networkInfraApi = {
  listGatewayNodes: () => api.get<GatewayNode[]>('/admin/gateway-nodes'),
  createGatewayNode: (data: CreateGatewayNodeRequest) => api.post<GatewayNode>('/admin/gateway-nodes', data),
  deleteGatewayNode: (id: string) => api.delete<void>(`/admin/gateway-nodes/${id}`),
  listIPPools: () => api.get<IPPool[]>('/admin/ip-pools'),
  createIPPool: (data: CreateIPPoolRequest) => api.post<IPPool>('/admin/ip-pools', data),
  deleteIPPool: (id: string) => api.delete<void>(`/admin/ip-pools/${id}`),
}
