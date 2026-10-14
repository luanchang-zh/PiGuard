import request from './request'
import type { ApiResponse, EventList, EventType, EventLevel } from '@/types'

export interface EventQuery {
  type?: EventType
  level?: EventLevel
  start?: string
  end?: string
  limit?: number
}

export function listEvents(deviceId: string, query: EventQuery = {}) {
  return request
    .get<ApiResponse<EventList>>(`/devices/${deviceId}/events`, { params: query })
    .then((r) => r.data.data!)
}
