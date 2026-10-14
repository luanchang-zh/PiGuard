import request from './request'
import type { ApiResponse, Device, DeviceState } from '@/types'

export function listDevices() {
  return request.get<ApiResponse<Device[]>>('/devices').then((r) => r.data.data!)
}

export function getDevice(deviceId: string) {
  return request.get<ApiResponse<Device>>(`/devices/${deviceId}`).then((r) => r.data.data!)
}

export function getDeviceState(deviceId: string) {
  return request.get<ApiResponse<DeviceState>>(`/devices/${deviceId}/state`).then((r) => r.data.data!)
}
