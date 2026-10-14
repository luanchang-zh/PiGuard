import request from './request'
import type { ApiResponse, ConfigView, ConfigUpdateRequest, ConfigUpdateResult } from '@/types'

export function getConfig(deviceId: string) {
  return request.get<ApiResponse<ConfigView>>(`/devices/${deviceId}/config`).then((r) => r.data.data!)
}

export function updateConfig(deviceId: string, body: ConfigUpdateRequest) {
  return request
    .put<ApiResponse<ConfigUpdateResult>>(`/devices/${deviceId}/config`, body)
    .then((r) => r.data.data!)
}
