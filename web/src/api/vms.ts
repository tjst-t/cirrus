import { api } from './client'

export type VmStatus = 'running' | 'stopped' | 'starting' | 'stopping' | 'error' | 'pending'

export interface Vm {
  id: string
  name: string
  status: VmStatus
  flavor_id: string
  flavor_name?: string
  network_id: string
  network_name?: string
  ip_address?: string
  vcpu: number
  memory_mb: number
  created_at: string
}

export interface VmDetail extends Vm {
  volume_ids?: string[]
  host_id?: string
}

export interface CreateVmRequest {
  name: string
  flavor_id: string
  network_id: string
  volume_type_id?: string
  volume_size_gb?: number
}

export interface Flavor {
  id: string
  name: string
  vcpu: number
  memory_mb: number
  disk_gb: number
}

export interface VolumeType {
  id: string
  name: string
}

export type VmAction = 'start' | 'stop' | 'reboot'

export const vmsApi = {
  list: (limit?: number) =>
    api.list<Vm>(`/vms${limit ? `?limit=${limit}` : ''}`),
  get: (id: string) => api.get<VmDetail>(`/vms/${id}`),
  create: (req: CreateVmRequest) => api.post<Vm>('/vms', req),
  delete: (id: string) => api.delete<void>(`/vms/${id}`),
  action: (id: string, action: VmAction) =>
    api.post<void>(`/vms/${id}/actions`, { action }),
  listFlavors: () => api.list<Flavor>('/flavors'),
  listVolumeTypes: () => api.get<VolumeType[]>('/volume-types'),
}
