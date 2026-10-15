import axios, { AxiosError, type AxiosInstance } from 'axios'
import { ElMessage } from 'element-plus'
import type { ApiResponse } from '@/types'
import { useAuthStore } from '@/stores/auth'

// 开发环境走 vite 代理（/api -> 127.0.0.1:8080），
// 生产环境若前后端不同源，可通过 VITE_API_BASE 注入后端地址。
const baseURL = import.meta.env.VITE_API_BASE ?? '/api/v1'

const request: AxiosInstance = axios.create({
  baseURL,
  timeout: 10000,
  withCredentials: true,
})

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

// 业务码映射。规范里 code=0 为成功。
request.interceptors.response.use(
  (response) => {
    const body = response.data as ApiResponse
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      ElMessage.error(body.message || '请求失败')
      return Promise.reject(body)
    }
    return response
  },
  (error: AxiosError<ApiResponse>) => {
    // 尚未上传预览是正常空状态，由调用方显示占位内容。
    if (error.response?.status === 404 && error.response.data?.code === 40403 && error.config?.url?.endsWith('/frame')) {
      return Promise.reject(error)
    }
    if (error.response?.status === 401) {
      ElMessage.error('认证已失效，请重新登录')
      window.location.href = '/login'
    } else {
      const msg = error.response?.data?.message || error.message || '网络异常'
      ElMessage.error(msg)
    }
    return Promise.reject(error)
  },
)

export default request
