import { api } from './client'

export type VolumeStatus = 'available' | 'in-use' | 'creating' | 'deleting' | 'error'

export interface Volume {
  id: string
  name: string
  size_gb: number
  status: VolumeStatus
  volume_type_id?: string
  volume_type_name?: string
  attached_vm_id?: string
  created_at: string
}

export interface CreateVolumeRequest {
  name: string
  size_gb: number
  volume_type_id?: string
}

export interface ResizeVolumeRequest {
  size_gb: number
}

export const volumesApi = {
  list: () => api.list<Volume>('/volumes'),
  create: (req: CreateVolumeRequest) => api.post<Volume>('/volumes', req),
  delete: (id: string) => api.delete<void>(`/volumes/${id}`),
  resize: (id: string, req: ResizeVolumeRequest) =>
    api.put<Volume>(`/volumes/${id}/resize`, req),
}
