import request from './request'
import type { ApiResponse, LatestFrame } from '@/types'

// 摄像头预览由后端直接返回图片内容 URL；前端只取最新预览元数据再拼内容地址。
export function getLatestFrame(deviceId: string) {
  return request
    .get<ApiResponse<LatestFrame>>(`/devices/${deviceId}/frame`)
    .then((r) => r.data.data!)
}

export function snapshotContentUrl(snapshotId: string) {
  return `${request.defaults.baseURL}/snapshots/${snapshotId}/content`
}
