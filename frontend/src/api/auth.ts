import request from './request'
import type { ApiResponse, SessionInfo } from '@/types'

// 后端要求：POST /auth/session 必须用 Basic 认证，成功后下发 Cookie。
// 之后的请求携带 Cookie，写操作需要 X-CSRF-Token。
export function basicAuthHeader(username: string, password: string) {
  return `Basic ${btoa(`${username}:${password}`)}`
}

export async function login(username: string, password: string) {
  const resp = await request.post<ApiResponse<SessionInfo>>(
    '/auth/session',
    {},
    { headers: { Authorization: basicAuthHeader(username, password) } },
  )
  return resp.data.data!
}

export async function getSession() {
  const resp = await request.get<ApiResponse<SessionInfo>>('/auth/session')
  return resp.data.data
}

export async function logout() {
  await request.delete('/auth/session')
}
