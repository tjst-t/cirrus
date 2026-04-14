import { api } from './client'

export interface IngressConfig {
  target_vm_id: string
  target_ip: string
}

export interface Ingress {
  id: string
  network_id: string
  type: string
  public_ip: string
  ip_pool_id?: string
  config: IngressConfig
  created_at: string
}

export interface CreateIngressRequest {
  type: string
  public_ip: string
  ip_pool_id: string
  config: {
    target_vm_id?: string
    target_ip?: string
  }
}

export interface IpPool {
  id: string
  name: string
  cidr: string
  description: string
  created_at: string
}

export const ingressApi = {
  // Ingress is scoped to network: /networks/{network_id}/ingresses
  list: (networkId: string) =>
    api.get<Ingress[]>(`/networks/${networkId}/ingresses`),
  create: (networkId: string, req: CreateIngressRequest) =>
    api.post<Ingress>(`/networks/${networkId}/ingresses`, req),
  delete: (networkId: string, id: string) =>
    api.delete<void>(`/networks/${networkId}/ingresses/${id}`),
  listIpPools: () =>
    api.get<IpPool[]>('/admin/ip-pools'),
}
