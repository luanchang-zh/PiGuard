import { defineStore } from 'pinia'
import { ref } from 'vue'
import { login as apiLogin, getSession, logout as apiLogout } from '@/api/auth'
import type { SessionInfo } from '@/types'

// 登录成功后后端返回 csrf_token，之后所有写请求必须带 X-CSRF-Token。
// 保存在 localStorage，刷新页面后仍可恢复会话（Cookie 由浏览器持久化）。
const CSRF_KEY = 'piguard_csrf_token'

export const useAuthStore = defineStore('auth', () => {
  const session = ref<SessionInfo | null>(null)
  const csrfToken = ref<string>(localStorage.getItem(CSRF_KEY) || '')

  async function login(username: string, password: string) {
    const info = await apiLogin(username, password)
    session.value = info
    if (info.csrf_token) {
      csrfToken.value = info.csrf_token
      localStorage.setItem(CSRF_KEY, info.csrf_token)
    }
    return info
  }

  async function refresh() {
    try {
      const info = await getSession()
      session.value = info ?? null
      if (info?.csrf_token) {
        csrfToken.value = info.csrf_token
        localStorage.setItem(CSRF_KEY, info.csrf_token)
      }
      return info
    } catch {
      session.value = null
      return null
    }
  }

  async function logout() {
    try {
      await apiLogout()
    } catch {
      /* 忽略网络错误 */
    }
    session.value = null
    csrfToken.value = ''
    localStorage.removeItem(CSRF_KEY)
  }

  return { session, csrfToken, login, refresh, logout }
})
