<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDeviceStore } from '@/stores/device'
import { useRealtime } from '@/composables/useRealtime'
import PgIcon from '@/components/ui/PgIcon.vue'
import PgBadge from '@/components/ui/PgBadge.vue'
import type { IconName } from '@/components/ui/icons'
import { formatClock, formatTime } from '@/utils/format'
import { RISK_META } from '@/utils/meta'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const device = useDeviceStore()
const { connected, connecting, telemetry, deviceStatus, audioAlertEnabled, connect, toggleAudioAlert } = useRealtime()

const sidebarCollapsed = ref(localStorage.getItem('piguard_sidebar_collapsed') === 'true')

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
  localStorage.setItem('piguard_sidebar_collapsed', String(sidebarCollapsed.value))
}

const menus: { path: string; title: string; subtitle: string; icon: IconName }[] = [
  { path: '/dashboard', title: '实时监控', subtitle: 'HUD 遥测与传感器矩阵', icon: 'dashboard' },
  { path: '/events', title: '告警记录', subtitle: '事件追溯与快照抓拍', icon: 'bell' },
  { path: '/control', title: '设备控制', subtitle: '执行器测试与模拟演练', icon: 'zap' },
  { path: '/config', title: '参数配置', subtitle: '预警阈值与版本同步', icon: 'sliders' },
]

const activeMenu = computed(() => route.path)

// 当前设备在线状态：实时推送优先，其次退回到已注册设备列表
const isCurrentDeviceOnline = computed(() => {
  const id = device.currentDeviceId
  if (telemetry.value?.device_id === id) return telemetry.value.online
  if (deviceStatus.value?.device_id === id) return deviceStatus.value.online
  const dev = device.devices.find((d) => d.device_id === id)
  return dev ? dev.online : false
})

// 当前设备活跃风险等级
const currentRisk = computed(() => {
  if (telemetry.value?.device_id === device.currentDeviceId) {
    return telemetry.value.risk_level || 'normal'
  }
  return 'unknown'
})

const activeAlarmsCount = computed(() => {
  if (telemetry.value?.device_id === device.currentDeviceId) {
    return telemetry.value.active_events?.length ?? 0
  }
  return 0
})

// 顶部时钟
const currentTime = ref(new Date())
let timer: ReturnType<typeof setInterval> | null = null

async function onDeviceSelect(id: string) {
  const d = device.devices.find((x) => x.device_id === id)
  device.setDevice(id, d?.name)
}

async function handleLogout() {
  await auth.logout()
  router.replace('/login')
}

onMounted(() => {
  device.refreshDevices()
  connect()
  timer = setInterval(() => {
    currentTime.value = new Date()
  }, 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="pg-layout" :class="{ 'is-collapsed': sidebarCollapsed }">
    <!-- 侧边栏 -->
    <aside class="pg-sidebar">
      <!-- 品牌头部 -->
      <div class="pg-sidebar__brand">
        <div class="pg-logo-wrapper">
          <div class="pg-logo-glow" />
          <div class="pg-logo">
            <PgIcon name="shield" :size="20" :stroke="2.4" />
          </div>
        </div>
        <div v-show="!sidebarCollapsed" class="pg-brand-meta">
          <h1 class="pg-brand-title">PiGuard</h1>
          <span class="pg-brand-tag">ADAS 驾驶辅助</span>
        </div>
        <button
          class="pg-collapse-btn"
          :title="sidebarCollapsed ? '展开侧边栏' : '折叠侧边栏'"
          @click="toggleSidebar"
        >
          <PgIcon :name="sidebarCollapsed ? 'expand' : 'collapse'" :size="16" />
        </button>
      </div>

      <!-- 导航项 -->
      <nav class="pg-menu">
        <router-link
          v-for="m in menus"
          :key="m.path"
          :to="m.path"
          class="pg-menu-item"
          :class="{ active: activeMenu === m.path }"
          :title="sidebarCollapsed ? m.title : undefined"
        >
          <div class="pg-menu-icon">
            <PgIcon :name="m.icon" :size="18" />
          </div>
          <div v-show="!sidebarCollapsed" class="pg-menu-text">
            <span class="pg-menu-title">{{ m.title }}</span>
            <span class="pg-menu-desc">{{ m.subtitle }}</span>
          </div>
          <span
            v-if="m.path === '/events' && activeAlarmsCount > 0"
            class="pg-alarm-pill"
            :class="{ 'pill-mini': sidebarCollapsed }"
          >
            {{ activeAlarmsCount }}
          </span>
          <div class="pg-active-indicator" />
        </router-link>
      </nav>

      <!-- 侧边栏底部设备状态底卡 -->
      <div class="pg-sidebar__footer">
        <div v-show="!sidebarCollapsed" class="pg-device-panel">
          <div class="pg-device-panel__header">
            <div class="pg-device-info">
              <span class="pg-device-id mono">{{ device.currentDeviceId }}</span>
              <span class="pg-device-sub">{{ device.deviceName }}</span>
            </div>
            <PgBadge
              :tone="isCurrentDeviceOnline ? 'ok' : 'unknown'"
              :dot="true"
              :pulse="isCurrentDeviceOnline"
              size="sm"
            >
              {{ isCurrentDeviceOnline ? '在线' : '离线' }}
            </PgBadge>
          </div>

          <div class="pg-device-panel__stats">
            <div class="pg-stat-item">
              <span class="pg-stat-label">综合安全</span>
              <PgBadge :tone="RISK_META[currentRisk].tone" size="sm">
                {{ RISK_META[currentRisk].label }}
              </PgBadge>
            </div>
            <div class="pg-stat-item">
              <span class="pg-stat-label">实时通道</span>
              <PgBadge :tone="connected ? 'ok' : connecting ? 'warn' : 'unknown'" size="sm">
                {{ connected ? 'WebSocket' : connecting ? '重连中' : '未连接' }}
              </PgBadge>
            </div>
          </div>

          <!-- 音效报警控制 -->
          <button
            class="pg-sound-toggle-btn"
            :class="{ active: audioAlertEnabled }"
            @click="toggleAudioAlert"
          >
            <PgIcon :name="audioAlertEnabled ? 'volume' : 'volume'" :size="14" />
            <span>{{ audioAlertEnabled ? '座舱警报音已开启' : '座舱警报音静音中' }}</span>
          </button>
        </div>

        <!-- 折叠状态下的迷你指示器 -->
        <div v-show="sidebarCollapsed" class="pg-device-mini">
          <span
            class="pg-mini-dot"
            :class="isCurrentDeviceOnline ? 'on' : 'off'"
            :title="`${device.currentDeviceId} (${isCurrentDeviceOnline ? '在线' : '离线'})`"
          />
        </div>
      </div>
    </aside>

    <!-- 右侧主体内容 -->
    <div class="pg-main-wrapper">
      <!-- 顶栏 -->
      <header class="pg-topbar">
        <div class="pg-topbar__left">
          <div class="pg-page-meta">
            <h2 class="pg-page-title">{{ (route.meta.title as string) || '仪表盘' }}</h2>
            <span class="pg-page-sub">PiGuard ADAS 智能车载驾驶辅助系统</span>
          </div>
        </div>

        <div class="pg-topbar__right">
          <!-- 设备切换下拉框 -->
          <div class="pg-device-select-box">
            <span class="pg-box-label">设备:</span>
            <el-select
              :model-value="device.currentDeviceId"
              placeholder="选择设备"
              size="default"
              style="width: 220px"
              @change="onDeviceSelect"
            >
              <el-option
                v-for="d in device.devices"
                :key="d.device_id"
                :label="`${d.name} (${d.device_id})`"
                :value="d.device_id"
              >
                <div class="pg-device-opt">
                  <span>{{ d.name }}</span>
                  <PgBadge :tone="d.online ? 'ok' : 'unknown'" size="sm" :dot="true">
                    {{ d.device_id }}
                  </PgBadge>
                </div>
              </el-option>
            </el-select>
          </div>

          <!-- 实时时间 -->
          <div class="pg-clock mono" :title="formatTime(currentTime.toISOString())">
            <PgIcon name="clock" :size="14" class="pg-clock-icon" />
            <span>{{ formatClock(currentTime) }}</span>
          </div>

          <!-- 用户会话与退出 -->
          <div class="pg-user-panel">
            <div class="pg-user-avatar">
              <PgIcon name="user" :size="15" />
            </div>
            <span class="pg-user-name">{{ auth.session?.username || '管理员' }}</span>
            <el-tag size="small" type="primary" effect="plain" class="pg-role-tag">
              {{ auth.session?.role === 'platform' ? '平台端' : '系统' }}
            </el-tag>
            <button class="pg-logout-btn" title="退出登录" @click="handleLogout">
              <PgIcon name="logout" :size="16" />
            </button>
          </div>
        </div>
      </header>

      <!-- 路由内容区 -->
      <main class="pg-content-scroll">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
    </div>
  </div>
</template>

<style scoped>
.pg-layout {
  display: flex;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  background: var(--pg-bg-deep);
  position: relative;
}

/* 背景科技感微弱渐变光斑 */
.pg-layout::before {
  content: '';
  position: absolute;
  top: -20%;
  left: 10%;
  width: 50vw;
  height: 50vw;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.04) 0%, transparent 70%);
  pointer-events: none;
  z-index: 0;
}

.pg-layout::after {
  content: '';
  position: absolute;
  bottom: -20%;
  right: 5%;
  width: 45vw;
  height: 45vw;
  background: radial-gradient(circle, rgba(129, 140, 248, 0.035) 0%, transparent 70%);
  pointer-events: none;
  z-index: 0;
}

/* ---------- 侧边栏 ---------- */
.pg-sidebar {
  width: var(--pg-sidebar-w);
  flex-shrink: 0;
  background: var(--pg-surface-solid);
  border-right: 1px solid var(--pg-border);
  display: flex;
  flex-direction: column;
  z-index: 10;
  transition: width var(--pg-dur) var(--pg-ease);
  position: relative;
  backdrop-filter: blur(20px);
}

.is-collapsed .pg-sidebar {
  width: var(--pg-sidebar-w-collapsed);
}

.pg-sidebar__brand {
  height: var(--pg-topbar-h);
  padding: 0 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  border-bottom: 1px solid var(--pg-border);
  position: relative;
}

.pg-logo-wrapper {
  position: relative;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
}

.pg-logo-glow {
  position: absolute;
  inset: -2px;
  border-radius: 11px;
  background: var(--pg-gradient-brand);
  opacity: 0.6;
  filter: blur(6px);
}

.pg-logo {
  position: relative;
  width: 100%;
  height: 100%;
  border-radius: 10px;
  background: linear-gradient(135deg, #0fb5d6, #3b82f6);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  box-shadow: 0 4px 12px rgba(15, 181, 214, 0.4);
}

.pg-brand-meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.pg-brand-title {
  font-size: 16px;
  font-weight: 750;
  letter-spacing: -0.02em;
  background: linear-gradient(135deg, #fff 40%, #bae6fd 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  line-height: 1.2;
}

.pg-brand-tag {
  font-size: 10.5px;
  font-weight: 550;
  color: var(--pg-primary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.pg-collapse-btn {
  margin-left: auto;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  border: 1px solid var(--pg-border);
  background: var(--pg-surface-2);
  color: var(--pg-text-muted);
  cursor: pointer;
  display: grid;
  place-items: center;
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.is-collapsed .pg-collapse-btn {
  margin-left: 0;
}

.pg-collapse-btn:hover {
  color: var(--pg-text);
  border-color: var(--pg-border-strong);
  background: var(--pg-surface-hover);
}

/* 导航菜单 */
.pg-menu {
  flex: 1;
  padding: 16px 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  overflow-y: auto;
}

.pg-menu-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 10px 14px;
  border-radius: var(--pg-radius-sm);
  color: var(--pg-text-muted);
  text-decoration: none;
  transition: all var(--pg-dur) var(--pg-ease);
}

.is-collapsed .pg-menu-item {
  padding: 12px;
  justify-content: center;
}

.pg-menu-item:hover {
  background: var(--pg-surface-2);
  color: var(--pg-text);
}

.pg-menu-item.active {
  background: linear-gradient(90deg, rgba(34, 211, 238, 0.14) 0%, rgba(34, 211, 238, 0.03) 100%);
  color: var(--pg-text);
  border: 1px solid rgba(34, 211, 238, 0.22);
}

.pg-menu-item.active .pg-menu-icon {
  color: var(--pg-primary);
  transform: scale(1.08);
}

.pg-menu-icon {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  transition: transform var(--pg-dur) var(--pg-ease), color var(--pg-dur);
}

.pg-menu-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.pg-menu-title {
  font-size: 13.5px;
  font-weight: 600;
  color: inherit;
  line-height: 1.3;
}

.pg-menu-desc {
  font-size: 11px;
  color: var(--pg-text-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pg-alarm-pill {
  margin-left: auto;
  font-size: 11px;
  font-weight: 700;
  padding: 2px 7px;
  border-radius: 999px;
  background: var(--pg-danger);
  color: #fff;
  box-shadow: 0 0 10px rgba(251, 81, 116, 0.5);
  animation: pg-pulse 1.8s infinite;
}

.pg-alarm-pill.pill-mini {
  position: absolute;
  top: 6px;
  right: 6px;
  padding: 2px 5px;
  font-size: 10px;
}

.pg-active-indicator {
  position: absolute;
  left: 0;
  top: 25%;
  bottom: 25%;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: var(--pg-primary);
  opacity: 0;
  transition: opacity var(--pg-dur);
}

.pg-menu-item.active .pg-active-indicator {
  opacity: 1;
}

/* 侧边栏底部面板 */
.pg-sidebar__footer {
  padding: 14px;
  border-top: 1px solid var(--pg-border);
  background: rgba(10, 16, 28, 0.4);
}

.is-collapsed .pg-sidebar__footer {
  padding: 12px 6px;
}

.pg-device-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pg-device-panel__header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 6px;
}

.pg-device-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.pg-device-id {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--pg-text);
}

.pg-device-sub {
  font-size: 11px;
  color: var(--pg-text-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pg-device-panel__stats {
  display: flex;
  flex-direction: column;
  gap: 6px;
  background: rgba(30, 41, 64, 0.4);
  padding: 8px 10px;
  border-radius: var(--pg-radius-xs);
  border: 1px solid var(--pg-border);
}

.pg-stat-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11.5px;
}

.pg-stat-label {
  color: var(--pg-text-muted);
}

.pg-sound-toggle-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  padding: 6px 10px;
  border-radius: var(--pg-radius-xs);
  border: 1px solid var(--pg-border);
  background: transparent;
  color: var(--pg-text-muted);
  font-size: 11.5px;
  cursor: pointer;
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.pg-sound-toggle-btn.active {
  background: rgba(34, 211, 238, 0.12);
  color: var(--pg-primary);
  border-color: rgba(34, 211, 238, 0.3);
}

.pg-sound-toggle-btn:hover {
  background: var(--pg-surface-hover);
  color: var(--pg-text);
}

.pg-device-mini {
  display: flex;
  justify-content: center;
  padding: 6px 0;
}

.pg-mini-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}

.pg-mini-dot.on {
  background: var(--pg-ok);
  box-shadow: 0 0 8px var(--pg-ok);
}

.pg-mini-dot.off {
  background: var(--pg-unknown);
}

/* ---------- 右侧主体 ---------- */
.pg-main-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100vh;
  position: relative;
  z-index: 1;
}

/* 顶栏 */
.pg-topbar {
  height: var(--pg-topbar-h);
  padding: 0 var(--pg-page-pad);
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--pg-border);
  background: rgba(15, 23, 38, 0.65);
  backdrop-filter: blur(16px);
  z-index: 9;
}

.pg-topbar__left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.pg-page-meta {
  display: flex;
  flex-direction: column;
}

.pg-page-title {
  font-size: 17px;
  font-weight: 700;
  color: #fff;
  line-height: 1.2;
}

.pg-page-sub {
  font-size: 11.5px;
  color: var(--pg-text-muted);
}

.pg-topbar__right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.pg-device-select-box {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pg-box-label {
  font-size: 12px;
  color: var(--pg-text-muted);
}

.pg-device-opt {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.pg-clock {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: var(--pg-radius-pill);
  background: rgba(30, 41, 64, 0.45);
  border: 1px solid var(--pg-border);
  font-size: 12px;
  color: var(--pg-text-2);
}

.pg-clock-icon {
  color: var(--pg-primary);
}

.pg-user-panel {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 8px 4px 4px;
  border-radius: var(--pg-radius-pill);
  background: rgba(30, 41, 64, 0.4);
  border: 1px solid var(--pg-border);
}

.pg-user-avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: linear-gradient(135deg, #0fb5d6, #6366f1);
  display: grid;
  place-items: center;
  color: #fff;
}

.pg-user-name {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--pg-text);
}

.pg-role-tag {
  font-size: 11px;
}

.pg-logout-btn {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: none;
  background: transparent;
  color: var(--pg-text-muted);
  cursor: pointer;
  display: grid;
  place-items: center;
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.pg-logout-btn:hover {
  background: rgba(251, 81, 116, 0.15);
  color: var(--pg-danger);
}

/* 内容滚动区 */
.pg-content-scroll {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: var(--pg-page-pad);
}
</style>
