import { api } from './client'

export interface EgressGateway {
  id: string
  name: string
  network_id: string
  network_name?: string
  public_ip?: string
  status: string
  created_at: string
}

export interface CreateEgressGatewayRequest {
  name: string
  network_id: string
}

export const egressApi = {
  // Egress is scoped to tenant + network: /tenants/{tenant_id}/networks/{network_id}/egresses
  list: (tenantId: string, networkId: string) =>
    api.get<EgressGateway[]>(`/tenants/${tenantId}/networks/${networkId}/egresses`),
  create: (tenantId: string, networkId: string, req: CreateEgressGatewayRequest) =>
    api.post<EgressGateway>(`/tenants/${tenantId}/networks/${networkId}/egresses`, req),
  delete: (tenantId: string, networkId: string, id: string) =>
    api.delete<void>(`/tenants/${tenantId}/networks/${networkId}/egresses/${id}`),
}
