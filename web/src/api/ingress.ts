import { api } from './client'

export interface IngressEndpoint {
  id: string
  name: string
  network_id: string
  network_name?: string
  vm_id?: string
  vm_name?: string
  port: number
  protocol: string
  public_ip?: string
  public_port?: number
  status: string
  created_at: string
}

export interface CreateIngressEndpointRequest {
  name: string
  network_id: string
  vm_id?: string
  port: number
  protocol: string
}

export const ingressApi = {
  // Ingress is scoped to network: /networks/{network_id}/ingresses
  list: (networkId: string) =>
    api.get<IngressEndpoint[]>(`/networks/${networkId}/ingresses`),
  create: (networkId: string, req: Omit<CreateIngressEndpointRequest, 'network_id'>) =>
    api.post<IngressEndpoint>(`/networks/${networkId}/ingresses`, req),
  delete: (networkId: string, id: string) =>
    api.delete<void>(`/networks/${networkId}/ingresses/${id}`),
}
