import request from './request'
import type { ApiResponse, Device, DeviceState } from '@/types'

export function listDevices(silent = false) {
  return request.get<ApiResponse<Device[]>>('/devices', { silent }).then((r) => r.data.data ?? [])
}

export function getDevice(deviceId: string) {
  return request.get<ApiResponse<Device>>(`/devices/${deviceId}`).then((r) => r.data.data!)
}

export function getDeviceState(deviceId: string, silent = false) {
  return request
    .get<ApiResponse<DeviceState>>(`/devices/${deviceId}/state`, { silent })
    .then((r) => r.data.data!)
}
