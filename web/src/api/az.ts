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
  list: () => api.get<AvailabilityZone[]>('/availability-zones'),
}
