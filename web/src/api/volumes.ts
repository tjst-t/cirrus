import { api } from './client'

export type VolumeState = 'creating' | 'available' | 'in_use' | 'deleting' | 'error'

export interface Volume {
  id: string
  tenant_id: string
  name: string
  volume_type_id?: string
  size_gb: number
  state: VolumeState
  az_id?: string
  created_at: string
  updated_at: string
}

export interface CreateVolumeRequest {
  name: string
  size_gb: number
  volume_type_id?: string
  az_id?: string
}

export interface ResizeVolumeRequest {
  new_size_gb: number
}

export interface JobResponse {
  job_id: string
}

export const volumesApi = {
  list: () => api.list<Volume>('/volumes'),
  create: (req: CreateVolumeRequest) => api.post<JobResponse>('/volumes', req),
  delete: (id: string) => api.delete<JobResponse>(`/volumes/${id}`),
  resize: (id: string, req: ResizeVolumeRequest) =>
    api.post<Volume>(`/volumes/${id}/resize`, req),
}
