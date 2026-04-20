import { api } from './client'

export type VmStatus = 'running' | 'stopped' | 'starting' | 'stopping' | 'error' | 'pending'

export interface Vm {
  id: string
  name: string
  status: VmStatus
  flavor_id?: string
  network_id?: string
  az_id?: string
  ip_address?: string
  error_message?: string
  created_at: string
  updated_at: string
}

export interface VmDetail extends Vm {
  host_id?: string
  error_message?: string
}

export interface CreateVmRequest {
  name: string
  flavor_id: string
  network_id: string
  az_id?: string
  volume_type_id?: string
  volume_size_gb?: number
}

export interface Flavor {
  id: string
  name: string
  vcpus: number
  ram_mb: number
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
