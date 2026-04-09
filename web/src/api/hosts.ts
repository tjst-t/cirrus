import { api } from './client'

export type HostStatus = 'active' | 'draining' | 'maintenance' | 'retired' | 'provisioning'
export type HostAction = 'activate' | 'drain' | 'maintenance' | 'retire'

export interface HostResourcePhysical {
  vcpus?: number
  memory_mb?: number
}

export interface HostResourceUsed {
  vcpus?: number
  memory_mb?: number
}

export interface Host {
  id: string
  name: string
  address: string
  operational_state: HostStatus
  resource_physical: HostResourcePhysical
  resource_used: HostResourceUsed
  created_at: string
  updated_at: string
}

export interface CreateHostRequest {
  name: string
  address: string
}

export const hostsApi = {
  list: () => api.list<Host>('/hosts'),
  get: (id: string) => api.get<Host>(`/hosts/${id}`),
  create: (data: CreateHostRequest) => api.post<Host>('/hosts', data),
  action: (id: string, action: HostAction) =>
    api.post<Host>(`/hosts/${id}/actions`, { action }),
}
