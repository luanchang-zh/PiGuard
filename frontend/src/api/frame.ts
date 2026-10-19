import request, { API_BASE } from './request'
import type { ApiResponse, LatestFrame } from '@/types'

// 摄像头预览由后端直接返回图片内容 URL；前端只取最新预览元数据再拼内容地址。
// 尚未上传预览时后端返回 404/40403，属于正常空状态，因此静默处理。
export function getLatestFrame(deviceId: string) {
  return request
    .get<ApiResponse<LatestFrame>>(`/devices/${deviceId}/frame`, { silent: true })
    .then((r) => r.data.data!)
}

export function snapshotContentUrl(snapshotId: string) {
  return `${API_BASE}/snapshots/${encodeURIComponent(snapshotId)}/content`
}
