import { api } from './client'

export interface StorageBackend {
  id: string
  storage_domain_id: string
  name: string
  driver: string
  endpoint: string
  total_capacity_gb: number
  total_iops: number
  capabilities: string[] | null
  driver_config: Record<string, unknown>
  state: string
  created_at: string
  updated_at: string
}

export interface CreateStorageBackendRequest {
  storage_domain_id: string
  name: string
  driver: string
  endpoint: string
  total_capacity_gb?: number
  total_iops?: number
  driver_config?: Record<string, unknown>
}

export interface StorageDomain {
  id: string
  name: string
  created_at: string
}

export interface AdminVolumeType {
  id: string
  name: string
  backend_id: string
  created_at: string
}

export interface CreateVolumeTypeRequest {
  name: string
  backend_id: string
}

export interface AdminFlavor {
  id: string
  name: string
  vcpus: number
  ram_mb: number
  disk_gb: number
  created_at: string
}

export interface CreateFlavorRequest {
  name: string
  vcpus: number
  ram_mb: number
  disk_gb: number
}

export const storageApi = {
  listDomains: () => api.get<StorageDomain[]>('/storage-domains'),

  listBackends: () => api.get<StorageBackend[]>('/admin/storage-backends'),
  createBackend: (data: CreateStorageBackendRequest) =>
    api.post<StorageBackend>('/admin/storage-backends', data),
  deleteBackend: (id: string) => api.delete<void>(`/admin/storage-backends/${id}`),

  // GET /admin/volume-types does not exist; shared /volume-types is used for listing
  listVolumeTypes: () => api.get<AdminVolumeType[]>('/volume-types'),
  createVolumeType: (data: CreateVolumeTypeRequest) =>
    api.post<AdminVolumeType>('/admin/volume-types', data),
  deleteVolumeType: (id: string) => api.delete<void>(`/admin/volume-types/${id}`),

  // GET /admin/flavors does not exist; shared /flavors is used for listing
  listFlavors: () => api.list<AdminFlavor>('/flavors'),
  createFlavor: (data: CreateFlavorRequest) =>
    api.post<AdminFlavor>('/admin/flavors', data),
  deleteFlavor: (id: string) => api.delete<void>(`/admin/flavors/${id}`),
}
