import { api } from './client'

export interface Quota {
  tenant_id: string
  vcpus: number
  memory_mb: number
  vm_count: number
  volume_gb: number
}

export interface UpdateQuotaRequest {
  vcpus: number
  memory_mb: number
  vm_count: number
  volume_gb: number
}

export const quotasApi = {
  get: (tenantId: string) => api.get<Quota>(`/admin/tenants/${tenantId}/quota`),
  update: (tenantId: string, data: UpdateQuotaRequest) =>
    api.put<Quota>(`/admin/tenants/${tenantId}/quota`, data),
}
