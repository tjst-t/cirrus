import { api } from './client'

export interface QuotaLimits {
  vcpus: number
  memory_mb: number
  vm_count: number
  volume_gb: number
  volumes: number
  snapshots: number
  networks: number
  egresses: number
  ingresses: number
}

export interface QuotaUsage {
  vcpus_used: number
  memory_mb_used: number
  vm_count_used: number
  volume_gb_used: number
  volumes_used: number
  snapshots_used: number
  networks_used: number
  egresses_used: number
  ingresses_used: number
}

export interface TenantQuota {
  limits: QuotaLimits
  usage: QuotaUsage
}

export const quotaApi = {
  get: (tenantId: string) =>
    api.get<TenantQuota>(`/tenants/${tenantId}/quota`),
  update: (tenantId: string, limits: QuotaLimits) =>
    api.put<TenantQuota>(`/tenants/${tenantId}/quota`, limits),
}
