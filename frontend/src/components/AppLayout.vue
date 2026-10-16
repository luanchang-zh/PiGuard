<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDeviceStore } from '@/stores/device'
import { listDevices } from '@/api/device'
import { useRealtime } from '@/composables/useRealtime'
import {
  Monitor,
  Warning,
  Operation,
  Setting,
  SwitchButton,
} from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const device = useDeviceStore()
const { connected, deviceStatus, telemetry, connect } = useRealtime()

const menus = [
  { path: '/dashboard', title: '实时监控', icon: Monitor },
  { path: '/events', title: '告警记录', icon: Warning },
  { path: '/control', title: '设备控制', icon: Operation },
  { path: '/config', title: '参数配置', icon: Setting },
]

const activeMenu = computed(() => route.path)
const online = computed(() => {
  if (!connected.value) return null
  const id = device.currentDeviceId
  if (telemetry.value?.device_id === id) return telemetry.value.online
  if (deviceStatus.value?.device_id === id) return deviceStatus.value.online
  return null
})

async function loadDevices() {
  try {
    const list = await listDevices()
    device.setDevices(list)
    const current = list.find((d) => d.device_id === device.currentDeviceId) ?? list[0]
    if (current) device.setDevice(current.device_id, current.name)
  } catch {
    /* 后端未就绪时保留默认设备 */
  }
}

async function onDeviceChange(id: string) {
  const d = device.devices.find((x) => x.device_id === id)
  device.setDevice(id, d?.name)
}

async function handleLogout() {
  await auth.logout()
  router.replace('/login')
}

onMounted(() => {
  loadDevices()
  connect()
})
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="logo">
        <span class="logo-mark">PG</span>
        <span class="logo-text">PiGuard</span>
      </div>

      <nav class="menu">
        <router-link
          v-for="m in menus"
          :key="m.path"
          :to="m.path"
          class="menu-item"
          :class="{ active: activeMenu === m.path }"
        >
          <el-icon><component :is="m.icon" /></el-icon>
          <span>{{ m.title }}</span>
        </router-link>
      </nav>

      <div class="device-card">
        <div class="device-row">
          <span class="dot" :class="{ on: online, off: !online }" />
          <span class="device-name">{{ device.deviceName }}</span>
          <el-tag v-if="online === null" size="small" type="info">状态未知</el-tag>
        </div>
        <div class="device-meta">
          <span>{{ device.currentDeviceId }}</span>
          <el-tag size="small" :type="connected ? 'success' : 'info'" effect="plain">
            {{ connected ? 'WS 已连接' : 'WS 未连接' }}
          </el-tag>
        </div>
      </div>
    </aside>

    <div class="main">
      <header class="topbar">
        <h2 class="page-title">{{ (route.meta.title as string) || 'PiGuard' }}</h2>
        <div class="topbar-right">
          <el-select
            v-model="device.currentDeviceId"
            placeholder="选择设备"
            size="default"
            style="width: 200px"
            @change="onDeviceChange"
          >
            <el-option
              v-for="d in device.devices"
              :key="d.device_id"
              :label="`${d.name} (${d.device_id})`"
              :value="d.device_id"
            />
          </el-select>
          <span class="welcome">{{ auth.session?.username }}</span>
          <el-button :icon="SwitchButton" @click="handleLogout">退出</el-button>
        </div>
      </header>
      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  background: var(--pg-bg);
}

.sidebar {
  width: 220px;
  flex-shrink: 0;
  background: var(--pg-bg-elevated);
  border-right: 1px solid var(--pg-border);
  display: flex;
  flex-direction: column;
}

.logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 20px 24px;
  border-bottom: 1px solid var(--pg-border);
}

.logo-mark {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: linear-gradient(135deg, var(--pg-primary), #0ea5e9);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 800;
  color: #fff;
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
}

.menu {
  flex: 1;
  padding: 16px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 10px;
  color: var(--pg-text-muted);
  font-size: 14px;
  transition: all 0.2s;
}

.menu-item:hover {
  background: var(--pg-bg-soft);
  color: var(--pg-text);
}

.menu-item.active {
  background: var(--pg-primary);
  color: #fff;
}

.device-card {
  padding: 16px;
  border-top: 1px solid var(--pg-border);
}

.device-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.dot.on {
  background: var(--pg-ok);
  box-shadow: 0 0 6px var(--pg-ok);
}

.dot.off {
  background: var(--pg-text-faint);
}

.device-name {
  font-weight: 600;
  color: var(--pg-text);
}

.device-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  color: var(--pg-text-faint);
}

.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.topbar {
  height: 56px;
  padding: 0 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--pg-border);
  background: var(--pg-bg-elevated);
}

.page-title {
  margin: 0;
  font-size: 18px;
  color: #fff;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.welcome {
  color: var(--pg-text-muted);
  font-size: 14px;
}

.content {
  flex: 1;
  overflow: auto;
  padding: 24px;
}
</style>
