import { api } from './client'

export interface QuotaLimits {
  Vcpus: number
  RAMMB: number
  VolumeGB: number
  VMs: number
  Volumes: number
  Snapshots: number
  Networks: number
  Egresses: number
  Ingresses: number
}

export interface QuotaUsage {
  TenantID: string
  VcpusUsed: number
  RAMMBUsed: number
  VolumeGBUsed: number
  VMsCount: number
  VolumesCount: number
  SnapshotsCount: number
  NetworksCount: number
  EgressesCount: number
  IngressesCount: number
}

export interface TenantQuota {
  limits: QuotaLimits
  usage: QuotaUsage
}

export const quotaApi = {
  get: (tenantId: string) =>
    api.get<TenantQuota>(`/tenants/${tenantId}/quota`),
}
