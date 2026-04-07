import { api } from './client'

export type DriftEventStatus = 'open' | 'resolved'
export type DriftEventResourceType = 'vm' | 'host'

export interface DriftEvent {
  id: string
  resource_type: DriftEventResourceType
  resource_id: string
  description: string
  status: DriftEventStatus
  detected_at: string
  resolved_at: string | null
}

export interface ListDriftEventsParams {
  status?: DriftEventStatus
  resource_type?: DriftEventResourceType
  limit?: number
}

export const driftEventsApi = {
  list: (params?: ListDriftEventsParams) => {
    const query = new URLSearchParams()
    if (params?.status) query.set('status', params.status)
    if (params?.resource_type) query.set('resource_type', params.resource_type)
    if (params?.limit !== undefined) query.set('limit', String(params.limit))
    const qs = query.toString()
    return api.list<DriftEvent>(`/admin/drift-events${qs ? `?${qs}` : ''}`)
  },
  resolve: (id: string) =>
    api.patch<DriftEvent>(`/admin/drift-events/${id}`, { status: 'resolved' }),
}
