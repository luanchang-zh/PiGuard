import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('@/components/AppLayout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'dashboard', name: 'dashboard', component: () => import('@/views/Dashboard.vue'), meta: { title: '实时监控' } },
      { path: 'events', name: 'events', component: () => import('@/views/Events.vue'), meta: { title: '告警记录' } },
      { path: 'control', name: 'control', component: () => import('@/views/Control.vue'), meta: { title: '设备控制' } },
      { path: 'config', name: 'config', component: () => import('@/views/Config.vue'), meta: { title: '参数配置' } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.meta.public) return true
  if (!auth.session) {
    const info = await auth.refresh()
    if (!info) return { name: 'login', query: { redirect: to.fullPath } }
  }
  return true
})

export default router
