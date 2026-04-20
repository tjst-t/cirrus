import { api } from './client'

export interface AvailabilityZone {
  id: string
  name: string
  description?: string
  location_id?: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export const azApi = {
  list: async (): Promise<AvailabilityZone[]> => {
    const res = await api.get<AvailabilityZone[] | { items: AvailabilityZone[] }>('/availability-zones')
    if (Array.isArray(res)) return res
    return (res as { items: AvailabilityZone[] }).items ?? []
  },
}
