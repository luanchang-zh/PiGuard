import axios, { AxiosError, type AxiosInstance } from 'axios'
import { ElMessage } from 'element-plus'
import type { ApiResponse } from '@/types'
import { useAuthStore } from '@/stores/auth'

declare module 'axios' {
  interface AxiosRequestConfig {
    /** 为 true 时不弹全局错误提示，由调用方自行处理。 */
    silent?: boolean
  }
}

// 开发环境走 vite 代理（/api -> 127.0.0.1:8080），
// 生产环境若前后端不同源，可通过 VITE_API_BASE 注入后端地址。
export const API_BASE = import.meta.env.VITE_API_BASE ?? '/api/v1'

const request: AxiosInstance = axios.create({
  baseURL: API_BASE,
  timeout: 10000,
  withCredentials: true,
})

// 同一提示 3 秒内只弹一次，避免后端掉线时刷屏。
const recent = new Map<string, number>()
function toast(msg: string) {
  const now = Date.now()
  const last = recent.get(msg)
  if (last && now - last < 3000) return
  recent.set(msg, now)
  ElMessage({ type: 'error', message: msg, grouping: true })
}

// 后端业务码 → 友好文案（未列出的使用后端 message）
const CODE_TEXT: Record<number, string> = {
  40001: '请求参数不合法',
  40101: '认证失败，请检查用户名和密码',
  40301: '没有权限执行该操作',
  40401: '设备不存在',
  40901: '命令重复提交',
  40902: '配置版本冲突，已有更新的配置，请刷新后重试',
  50301: '设备离线，命令无法下发',
  50302: '设备发布通道不可用，请稍后重试',
  50303: '会话数量已满，请稍后再试',
}

export function errorMessage(err: unknown, fallback = '请求失败'): string {
  const e = err as AxiosError<ApiResponse> & ApiResponse
  const body = e?.response?.data ?? (typeof e?.code === 'number' ? (e as ApiResponse) : undefined)
  if (body && typeof body.code === 'number' && CODE_TEXT[body.code]) return CODE_TEXT[body.code]
  if (body?.message) return body.message
  if (e?.code === 'ECONNABORTED') return '请求超时，请检查网络或后端服务'
  if (e?.message === 'Network Error') return '无法连接到平台服务'
  return e?.message || fallback
}

// 写请求带上 CSRF token（Cookie 会话模式下后端要求 X-CSRF-Token）。
request.interceptors.request.use((config) => {
  const method = (config.method ?? 'get').toLowerCase()
  if (method !== 'get' && method !== 'head') {
    const auth = useAuthStore()
    if (auth.csrfToken) {
      config.headers['X-CSRF-Token'] = auth.csrfToken
    }
  }
  return config
})

request.interceptors.response.use(
  (response) => {
    const body = response.data as ApiResponse
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      if (!response.config.silent) toast(errorMessage(body))
      return Promise.reject(body)
    }
    return response
  },
  async (error: AxiosError<ApiResponse>) => {
    const url = error.config?.url ?? ''
    const status = error.response?.status
    const isSessionCall = url.endsWith('/auth/session')

    if (status === 401 && !isSessionCall) {
      // 会话失效：清理本地状态并回到登录页（保留当前地址以便登录后返回）。
      const auth = useAuthStore()
      auth.clear()
      const { default: router } = await import('@/router')
      const current = router.currentRoute.value
      if (current.name !== 'login') {
        toast('登录已过期，请重新登录')
        router.replace({ name: 'login', query: { redirect: current.fullPath } })
      }
      return Promise.reject(error)
    }

    if (!error.config?.silent && !(isSessionCall && error.config?.method === 'get')) {
      toast(errorMessage(error))
    }
    return Promise.reject(error)
  },
)

export default request
