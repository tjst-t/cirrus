import { api } from './client'

export interface EgressConfig {
  public_ip?: string
}

export interface Egress {
  id: string
  network_id: string
  type: string
  config: EgressConfig
}

export interface CreateEgressRequest {
  type: string
  config?: EgressConfig
}

export const egressApi = {
  // Egress is scoped to tenant + network: /tenants/{tenant_id}/networks/{network_id}/egresses
  list: (tenantId: string, networkId: string) =>
    api.get<Egress[]>(`/tenants/${tenantId}/networks/${networkId}/egresses`),
  create: (tenantId: string, networkId: string, req: CreateEgressRequest) =>
    api.post<Egress>(`/tenants/${tenantId}/networks/${networkId}/egresses`, req),
  update: (tenantId: string, networkId: string, id: string, config: EgressConfig) =>
    api.patch<Egress>(`/tenants/${tenantId}/networks/${networkId}/egresses/${id}`, config),
  delete: (tenantId: string, networkId: string, id: string) =>
    api.delete<void>(`/tenants/${tenantId}/networks/${networkId}/egresses/${id}`),
}
