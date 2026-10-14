import request from './request'
import type { ApiResponse, TelemetryList } from '@/types'

export interface TelemetryQuery {
  start?: string
  end?: string
  limit?: number
}

export function listTelemetry(deviceId: string, query: TelemetryQuery = {}) {
  return request
    .get<ApiResponse<TelemetryList>>(`/devices/${deviceId}/telemetry`, { params: query })
    .then((r) => r.data.data!)
}
