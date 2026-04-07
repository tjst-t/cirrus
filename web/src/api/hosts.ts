import { api } from './client'

export type HostStatus = 'active' | 'draining' | 'maintenance' | 'retired' | 'provisioning'
export type HostAction = 'activate' | 'drain' | 'maintenance' | 'retire'

export interface Host {
  id: string
  name: string
  address: string
  operational_state: HostStatus
  vcpus_total: number
  vcpus_used: number
  memory_total_mb: number
  memory_used_mb: number
  created_at: string
  updated_at: string
}

export interface CreateHostRequest {
  name: string
  address: string
  vcpus_total: number
  memory_total_mb: number
}

export const hostsApi = {
  list: () => api.list<Host>('/hosts'),
  get: (id: string) => api.get<Host>(`/hosts/${id}`),
  create: (data: CreateHostRequest) => api.post<Host>('/hosts', data),
  action: (id: string, action: HostAction) =>
    api.post<Host>(`/hosts/${id}/actions`, { action }),
}
