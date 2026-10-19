<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useDeviceStore } from '@/stores/device'
import { useRealtime } from '@/composables/useRealtime'
import { getDeviceState } from '@/api/device'
import { listTelemetry } from '@/api/telemetry'
import { getLatestFrame, snapshotContentUrl } from '@/api/frame'
import { createCommand } from '@/api/command'
import BaseChart from '@/components/BaseChart.vue'
import PgCard from '@/components/ui/PgCard.vue'
import PgBadge from '@/components/ui/PgBadge.vue'
import PgIcon from '@/components/ui/PgIcon.vue'
import PgEmpty from '@/components/ui/PgEmpty.vue'
import {
  formatNumber,
  formatTime,
  formatClock,
  formatRelative,
  describeEventData,
} from '@/utils/format'
import {
  RISK_META,
  EVENT_META,
  SENSORS,
  SENSOR_STATUS_META,
  FRESHNESS_META,
  DIRECTION_LABEL,
} from '@/utils/meta'
import type { DeviceState, RiskLevel } from '@/types'
import type { EChartsOption } from 'echarts'
import { ElMessage } from 'element-plus'

const device = useDeviceStore()
const { connected, telemetry, events, connect } = useRealtime()

const state = ref<DeviceState | null>(null)
const frameUrl = ref<string>('')
const frameCapturedAt = ref<string>('')
const frameSnapshotId = ref<string>('')
const frameLoading = ref(false)
const snapshotSubmitting = ref(false)

const distanceHistory = ref<{ t: string; v: number | null }[]>([])
const speedHistory = ref<{ t: string; v: number | null }[]>([])
const yawHistory = ref<{ t: string; v: number | null }[]>([])

let frameTimer: ReturnType<typeof setInterval> | undefined
let disposed = false

// 是否实时接收数据：WebSocket 连接且收到过对应当前设备的遥测
const live = computed(() => {
  return connected.value && telemetry.value !== null && telemetry.value.device_id === device.currentDeviceId
})

// 当前最新合并状态
const current = computed<DeviceState | null>(() => {
  if (telemetry.value && telemetry.value.device_id === device.currentDeviceId) {
    return {
      device_id: telemetry.value.device_id,
      timestamp: telemetry.value.timestamp,
      online: telemetry.value.online,
      speed_kmh: telemetry.value.speed_kmh,
      distance_m: telemetry.value.distance_m,
      temperature_c: telemetry.value.temperature_c,
      yaw_rate_deg_s: telemetry.value.yaw_rate_deg_s,
      lane: telemetry.value.lane,
      risk_level: telemetry.value.risk_level,
      samples: telemetry.value.samples,
      freshness: telemetry.value.freshness,
      active_events: telemetry.value.active_events,
    }
  }
  return state.value
})

const riskLevel = computed<RiskLevel>(() => {
  if (!live.value) return 'unknown'
  return current.value?.risk_level ?? 'unknown'
})

const riskMeta = computed(() => RISK_META[riskLevel.value])

// 计算 TTC (Time To Collision 车头碰撞时距)
const computedTtc = computed<number | null>(() => {
  const d = current.value?.distance_m
  const s = current.value?.speed_kmh
  if (d === undefined || d === null || s === undefined || s === null || s <= 0.5) return null
  const speedMs = s / 3.6
  const ttc = d / speedMs
  return Math.max(0, Math.min(99.9, ttc))
})

// 车道偏离车模水平位移（-1 到 +1 映射为 10% 到 90%，0 为 50%）
const laneOffsetPct = computed(() => {
  const v = current.value?.lane?.offset_ratio
  if (v === undefined || v === null) return 50
  const clamped = Math.max(-1, Math.min(1, v))
  return 50 + clamped * 38
})

const isLaneDeparting = computed(() => {
  const v = current.value?.lane?.offset_ratio
  return v !== undefined && v !== null && Math.abs(v) >= 0.35
})

// 历史数据队列
function pushHistory(arr: { t: string; v: number | null }[], t: string, v: number | null | undefined) {
  arr.push({ t, v: v ?? null })
  if (arr.length > 50) arr.shift()
}

// 距离趋势图
const distanceOption = computed<EChartsOption>(() => {
  const times = distanceHistory.value.map((d) => formatClock(d.t))
  const values = distanceHistory.value.map((d) => d.v)
  return {
    backgroundColor: 'transparent',
    grid: { left: 45, right: 18, top: 28, bottom: 26 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(15, 23, 38, 0.9)',
      borderColor: 'rgba(34, 211, 238, 0.4)',
      textStyle: { color: '#e2e8f0', fontSize: 12 },
      formatter: (params: any) => {
        const item = Array.isArray(params) ? params[0] : params
        return `<div style="font-size:11px;color:#94a3b8">${item.axisValue}</div>
                <div style="font-weight:700;color:#22d3ee;margin-top:2px">前方距离: ${item.data !== null ? item.data + ' m' : '--'}</div>`
      },
    },
    xAxis: {
      type: 'category',
      data: times,
      axisLabel: { color: '#64748b', fontSize: 10.5 },
      axisLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.15)' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      name: '米 (m)',
      nameTextStyle: { color: '#64748b', fontSize: 11, padding: [0, 0, 4, -10] },
      axisLabel: { color: '#64748b', fontSize: 10.5 },
      splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.08)', type: 'dashed' } },
    },
    series: [
      {
        name: '前方距离',
        type: 'line',
        data: values,
        smooth: 0.3,
        symbol: 'none',
        lineStyle: { color: '#22d3ee', width: 2.2 },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(34, 211, 238, 0.32)' },
              { offset: 1, color: 'rgba(34, 211, 238, 0.0)' },
            ],
          },
        },
        markLine: {
          symbol: 'none',
          silent: true,
          lineStyle: { type: 'dashed', width: 1 },
          data: [
            { yAxis: 8, lineStyle: { color: '#fb5174' }, label: { formatter: '危险 8m', color: '#fb5174', fontSize: 10 } },
            { yAxis: 15, lineStyle: { color: '#fbbf24' }, label: { formatter: '预警 15m', color: '#fbbf24', fontSize: 10 } },
          ],
        },
      },
    ],
  }
})

// 车速与偏航角速度趋势图
const speedOption = computed<EChartsOption>(() => {
  const times = speedHistory.value.map((d) => formatClock(d.t))
  const speeds = speedHistory.value.map((d) => d.v)
  const yaws = yawHistory.value.map((d) => d.v)

  return {
    backgroundColor: 'transparent',
    grid: { left: 45, right: 45, top: 28, bottom: 26 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(15, 23, 38, 0.9)',
      borderColor: 'rgba(167, 139, 250, 0.4)',
      textStyle: { color: '#e2e8f0', fontSize: 12 },
    },
    legend: {
      data: ['车速 (km/h)', '角速度 (°/s)'],
      textStyle: { color: '#94a3b8', fontSize: 11 },
      top: 0,
      right: 10,
    },
    xAxis: {
      type: 'category',
      data: times,
      axisLabel: { color: '#64748b', fontSize: 10.5 },
      axisLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.15)' } },
      axisTick: { show: false },
    },
    yAxis: [
      {
        type: 'value',
        name: 'km/h',
        nameTextStyle: { color: '#64748b', fontSize: 11, padding: [0, 0, 4, -10] },
        axisLabel: { color: '#a78bfa', fontSize: 10.5 },
        splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.08)', type: 'dashed' } },
      },
      {
        type: 'value',
        name: '°/s',
        nameTextStyle: { color: '#64748b', fontSize: 11, padding: [0, 0, 4, 10] },
        axisLabel: { color: '#34d399', fontSize: 10.5 },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: '车速 (km/h)',
        type: 'line',
        data: speeds,
        smooth: 0.3,
        symbol: 'none',
        lineStyle: { color: '#a78bfa', width: 2.2 },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(167, 139, 250, 0.28)' },
              { offset: 1, color: 'rgba(167, 139, 250, 0.0)' },
            ],
          },
        },
      },
      {
        name: '角速度 (°/s)',
        type: 'line',
        yAxisIndex: 1,
        data: yaws,
        smooth: 0.3,
        symbol: 'none',
        lineStyle: { color: '#34d399', width: 1.8, type: 'dashed' },
      },
    ],
  }
})

// 传感器矩阵行计算
const sensorRows = computed(() => {
  const s = current.value
  return SENSORS.map((def) => {
    const sample = s?.samples?.[def.key]
    const rawFreshness = s?.freshness?.[def.key] ?? 'missing'
    const freshness = (!live.value && rawFreshness === 'fresh') ? 'stale' : rawFreshness

    let status = sample?.status ?? 'unavailable'
    if (def.key === 'lane' && s?.lane) {
      status = s.lane.valid ? 'ok' : 'unavailable'
    }

    return {
      key: def.key,
      name: def.label,
      model: def.model,
      icon: def.icon,
      status,
      statusMeta: SENSOR_STATUS_META[status] || SENSOR_STATUS_META.unavailable,
      freshness,
      freshnessMeta: FRESHNESS_META[freshness] || FRESHNESS_META.missing,
      sampleAt: sample?.sample_at || s?.timestamp,
      source: sample?.source || 'mock',
    }
  })
})

async function loadInitialState() {
  try {
    const s = await getDeviceState(device.currentDeviceId, true)
    state.value = s
  } catch {
    /* ignore */
  }

  // 预充 40 个遥测点渲染趋势
  try {
    const res = await listTelemetry(device.currentDeviceId, { limit: 40 }, true)
    distanceHistory.value = []
    speedHistory.value = []
    yawHistory.value = []
    for (const it of res.items) {
      pushHistory(distanceHistory.value, it.timestamp, it.distance_m)
      pushHistory(speedHistory.value, it.timestamp, it.speed_kmh)
      pushHistory(yawHistory.value, it.timestamp, it.yaw_rate_deg_s)
    }
  } catch {
    /* ignore */
  }
}

async function loadLatestFrame() {
  if (frameLoading.value) return
  frameLoading.value = true
  try {
    const f = await getLatestFrame(device.currentDeviceId)
    if (f?.snapshot_id) {
      frameSnapshotId.value = f.snapshot_id
      frameCapturedAt.value = f.captured_at
      frameUrl.value = snapshotContentUrl(f.snapshot_id)
    }
  } catch {
    /* 尚未拍摄为正常情况 */
  } finally {
    frameLoading.value = false
  }
}

async function triggerSnapshot() {
  snapshotSubmitting.value = true
  try {
    await createCommand(device.currentDeviceId, {
      type: 'camera.snapshot',
      params: { quality: 85 },
    })
    ElMessage.success('已下发远程抓拍指令，设备正在上传画面')
    // 延迟 1.5s 后刷新帧
    setTimeout(loadLatestFrame, 1800)
  } catch {
    /* request 已提示 */
  } finally {
    snapshotSubmitting.value = false
  }
}

onMounted(async () => {
  connect()
  await loadInitialState()
  await loadLatestFrame()
  if (!disposed) {
    frameTimer = setInterval(loadLatestFrame, 3500)
  }
})

onUnmounted(() => {
  disposed = true
  if (frameTimer) clearInterval(frameTimer)
})

// 监听实时遥测，推进图表
watch(
  () => telemetry.value,
  (t) => {
    if (!t || t.device_id !== device.currentDeviceId) return
    state.value = t
    const ts = t.timestamp || new Date().toISOString()
    pushHistory(distanceHistory.value, ts, t.distance_m)
    pushHistory(speedHistory.value, ts, t.speed_kmh)
    pushHistory(yawHistory.value, ts, t.yaw_rate_deg_s)
  },
)

// 设备切换时重新载入初始数据
watch(
  () => device.currentDeviceId,
  async () => {
    state.value = null
    frameUrl.value = ''
    frameSnapshotId.value = ''
    await loadInitialState()
    await loadLatestFrame()
  },
)
</script>

<template>
  <div class="pg-page pg-dashboard pg-stagger">
    <!-- 断线提示条 -->
    <el-alert
      v-if="!live"
      type="warning"
      show-icon
      :closable="false"
      class="pg-alert-banner"
    >
      <template #title>
        <span class="pg-alert-title">
          {{ connected ? '实时 WebSocket 已连通，等待边缘端遥测心跳上报' : '实时 WebSocket 正在尝试重连中...' }}
        </span>
      </template>
      <template #default>
        当前展示的为最后已入库的历史缓存快照，请确保边缘端 Python 采集服务与 MQTT 通道处于运行状态。
      </template>
    </el-alert>

    <!-- 顶部驾驶舱综合风险总览条 -->
    <section class="pg-risk-cockpit" :class="`tone-${riskMeta.tone}`">
      <div class="pg-cockpit-glow" />
      <div class="pg-cockpit-left">
        <div class="pg-shield-radar">
          <div class="pg-radar-ring pg-radar-ring--1" />
          <div class="pg-radar-ring pg-radar-ring--2" />
          <div class="pg-radar-core">
            <PgIcon name="shieldCheck" :size="26" :stroke="2.4" />
          </div>
        </div>
        <div class="pg-risk-desc-box">
          <div class="pg-risk-badge-row">
            <span class="pg-risk-label">综合安全风险评估</span>
            <PgBadge :tone="riskMeta.tone" size="md" :pulse="riskLevel === 'danger'">
              {{ riskMeta.label }}
            </PgBadge>
          </div>
          <h2 class="pg-risk-headline">{{ riskMeta.desc }}</h2>
          <div class="pg-risk-submeta">
            <span class="mono">设备: {{ device.currentDeviceId }}</span>
            <span class="pg-sep">·</span>
            <span class="mono">上报时间: {{ current?.timestamp ? formatTime(current.timestamp) : '--' }}</span>
            <span class="pg-sep">·</span>
            <span>{{ formatRelative(current?.timestamp) }}</span>
          </div>
        </div>
      </div>

      <div class="pg-cockpit-right">
        <!-- 活跃警报标签池 -->
        <div class="pg-active-alarm-pool">
          <span class="pg-pool-title">
            <PgIcon name="bell" :size="14" />
            <span>实时活跃告警 ({{ current?.active_events?.length ?? 0 }})</span>
          </span>
          <div v-if="current?.active_events && current.active_events.length > 0" class="pg-pool-tags">
            <PgBadge
              v-for="evtKey in current.active_events"
              :key="evtKey"
              tone="danger"
              size="sm"
              :pulse="true"
            >
              {{ EVENT_META[evtKey as keyof typeof EVENT_META]?.label || evtKey }}
            </PgBadge>
          </div>
          <div v-else class="pg-pool-empty">
            <PgIcon name="checkCircle" :size="14" class="text-ok" />
            <span>当前无活跃未解除告警</span>
          </div>
        </div>

        <button class="pg-cockpit-refresh-btn" title="刷新最新数据" @click="loadInitialState">
          <PgIcon name="refresh" :size="15" />
        </button>
      </div>
    </section>

    <!-- 四大核心指标仪表卡片 -->
    <section class="pg-metric-grid">
      <!-- 1. 车速 -->
      <PgCard padding="sm" class="pg-metric-card" accent="var(--pg-c-speed)">
        <div class="pg-metric-inner">
          <div class="pg-metric-top">
            <div class="pg-metric-icon-box tone-primary">
              <PgIcon name="gauge" :size="20" />
            </div>
            <span class="pg-metric-name">实时车速</span>
            <PgBadge tone="primary" size="sm" class="pg-metric-tag">车载测速</PgBadge>
          </div>
          <div class="pg-metric-val-row">
            <span class="pg-metric-num mono">{{ formatNumber(current?.speed_kmh, 1) }}</span>
            <span class="pg-metric-unit">km/h</span>
          </div>
          <div class="pg-metric-bar">
            <div
              class="pg-metric-bar-fill"
              :style="{
                width: `${Math.min(100, ((current?.speed_kmh || 0) / 120) * 100)}%`,
                background: 'linear-gradient(90deg, #818cf8, #a78bfa)',
              }"
            />
          </div>
          <div class="pg-metric-subtext">
            <span>基准最高 120 km/h</span>
            <span class="mono">{{ formatClock(current?.samples?.speed?.sample_at) }}</span>
          </div>
        </div>
      </PgCard>

      <!-- 2. 前方超声波距离与 TTC -->
      <PgCard padding="sm" class="pg-metric-card" accent="var(--pg-c-distance)">
        <div class="pg-metric-inner">
          <div class="pg-metric-top">
            <div class="pg-metric-icon-box tone-info">
              <PgIcon name="radar" :size="20" />
            </div>
            <span class="pg-metric-name">前方障碍物距离</span>
            <PgBadge
              :tone="
                (current?.distance_m ?? 99) <= 8
                  ? 'danger'
                  : (current?.distance_m ?? 99) <= 15
                  ? 'warn'
                  : 'ok'
              "
              size="sm"
            >
              {{
                (current?.distance_m ?? 99) <= 8
                  ? '危险'
                  : (current?.distance_m ?? 99) <= 15
                  ? '预警'
                  : '安全'
              }}
            </PgBadge>
          </div>
          <div class="pg-metric-val-row">
            <span class="pg-metric-num mono">{{ formatNumber(current?.distance_m, 2) }}</span>
            <span class="pg-metric-unit">米 (m)</span>
          </div>
          <div class="pg-metric-bar">
            <div
              class="pg-metric-bar-fill"
              :style="{
                width: `${Math.min(100, ((current?.distance_m || 0) / 25) * 100)}%`,
                background:
                  (current?.distance_m ?? 99) <= 8
                    ? 'var(--pg-danger)'
                    : (current?.distance_m ?? 99) <= 15
                    ? 'var(--pg-warn)'
                    : 'var(--pg-ok)',
              }"
            />
          </div>
          <div class="pg-metric-subtext">
            <span v-if="computedTtc !== null" class="pg-ttc-hint">
              碰撞时距 TTC: <b>{{ formatNumber(computedTtc, 1) }}s</b>
            </span>
            <span v-else>测距范围 0.2 ~ 25 m</span>
            <span class="mono">{{ formatClock(current?.samples?.distance?.sample_at) }}</span>
          </div>
        </div>
      </PgCard>

      <!-- 3. 车内环境温度 -->
      <PgCard padding="sm" class="pg-metric-card" accent="var(--pg-c-temp)">
        <div class="pg-metric-inner">
          <div class="pg-metric-top">
            <div class="pg-metric-icon-box tone-warn">
              <PgIcon name="thermometer" :size="20" />
            </div>
            <span class="pg-metric-name">座舱内温度</span>
            <PgBadge
              :tone="(current?.temperature_c ?? 25) >= 35 ? 'danger' : 'ok'"
              size="sm"
            >
              {{ (current?.temperature_c ?? 25) >= 35 ? '高温' : '舒适' }}
            </PgBadge>
          </div>
          <div class="pg-metric-val-row">
            <span class="pg-metric-num mono">{{ formatNumber(current?.temperature_c, 1) }}</span>
            <span class="pg-metric-unit">°C</span>
          </div>
          <div class="pg-metric-bar">
            <div
              class="pg-metric-bar-fill"
              :style="{
                width: `${Math.min(100, (((current?.temperature_c || 20) - 10) / 40) * 100)}%`,
                background: 'linear-gradient(90deg, #38bdf8, #fb923c)',
              }"
            />
          </div>
          <div class="pg-metric-subtext">
            <span>预警上限 35.0 °C</span>
            <span class="mono">{{ formatClock(current?.samples?.temperature?.sample_at) }}</span>
          </div>
        </div>
      </PgCard>

      <!-- 4. 偏航角速度 -->
      <PgCard padding="sm" class="pg-metric-card" accent="var(--pg-c-yaw)">
        <div class="pg-metric-inner">
          <div class="pg-metric-top">
            <div class="pg-metric-icon-box tone-ok">
              <PgIcon name="compass" :size="20" />
            </div>
            <span class="pg-metric-name">偏航角速度 (Yaw)</span>
            <PgBadge
              :tone="Math.abs(current?.yaw_rate_deg_s || 0) >= 30 ? 'warn' : 'ok'"
              size="sm"
            >
              {{ Math.abs(current?.yaw_rate_deg_s || 0) >= 30 ? '急转' : '平稳' }}
            </PgBadge>
          </div>
          <div class="pg-metric-val-row">
            <span class="pg-metric-num mono">{{ formatNumber(current?.yaw_rate_deg_s, 1) }}</span>
            <span class="pg-metric-unit">°/s</span>
          </div>
          <div class="pg-metric-bar">
            <div
              class="pg-metric-bar-fill"
              :style="{
                width: `${Math.min(100, (Math.abs(current?.yaw_rate_deg_s || 0) / 60) * 100)}%`,
                background: 'linear-gradient(90deg, #34d399, #10b981)',
              }"
            />
          </div>
          <div class="pg-metric-subtext">
            <span>
              {{ (current?.yaw_rate_deg_s || 0) > 1 ? '右向偏转' : (current?.yaw_rate_deg_s || 0) < -1 ? '左向偏转' : '直线行驶' }}
            </span>
            <span class="mono">{{ formatClock(current?.samples?.gyro?.sample_at) }}</span>
          </div>
        </div>
      </PgCard>
    </section>

    <!-- 中部：ADAS 视觉与车道保持沉浸式监控 -->
    <section class="pg-vision-grid">
      <!-- 车道偏离 3D 轨迹透视面板 -->
      <PgCard
        title="车道保持与偏离预警 (LDW)"
        subtitle="基于 OV5647 摄像头视觉边缘车道线解算"
        icon="road"
        accent="var(--pg-c-lane)"
      >
        <template #extra>
          <PgBadge
            :tone="!live ? 'warn' : current?.lane?.valid ? (isLaneDeparting ? 'danger' : 'ok') : 'unknown'"
            size="sm"
            :dot="true"
          >
            {{ !live ? '数据过期' : current?.lane?.valid ? (isLaneDeparting ? '正在偏离' : '车道居中') : '未检测到标线' }}
          </PgBadge>
        </template>

        <div class="pg-lane-hud">
          <!-- 3D 道路透视仿真跑道 -->
          <div class="pg-lane-canvas">
            <div class="pg-lane-horizon" />
            <div class="pg-road-surface">
              <!-- 左边缘线 -->
              <div
                class="pg-road-line pg-road-line--left"
                :class="{ active: (current?.lane?.offset_ratio ?? 0) <= -0.35 }"
              />
              <!-- 中间虚线 -->
              <div class="pg-road-center-stripes" />
              <!-- 右边缘线 -->
              <div
                class="pg-road-line pg-road-line--right"
                :class="{ active: (current?.lane?.offset_ratio ?? 0) >= 0.35 }"
              />
              <!-- 车辆 HUD 定位模型 -->
              <div
                class="pg-vehicle-pointer"
                :style="{ left: `${laneOffsetPct}%` }"
              >
                <div class="pg-vehicle-halo" :class="{ alert: isLaneDeparting }" />
                <div class="pg-vehicle-body">
                  <PgIcon name="car" :size="34" />
                </div>
                <div class="pg-vehicle-axis" />
              </div>
            </div>
          </div>

          <!-- 车道数值状态指示条 -->
          <div class="pg-lane-telemetry-strip">
            <div class="pg-lane-stat">
              <span class="pg-lane-stat__label">偏离比率</span>
              <span class="pg-lane-stat__val mono" :class="{ 'text-danger': isLaneDeparting }">
                {{ formatNumber(current?.lane?.offset_ratio, 2) }}
              </span>
            </div>
            <div class="pg-lane-stat">
              <span class="pg-lane-stat__label">行驶方向</span>
              <span class="pg-lane-stat__val">
                {{ DIRECTION_LABEL[current?.lane?.direction ?? 'unknown'] || '未知' }}
              </span>
            </div>
            <div class="pg-lane-stat">
              <span class="pg-lane-stat__label">算法置信度</span>
              <span class="pg-lane-stat__val mono">
                {{ current?.lane?.confidence ? Math.round(current.lane.confidence * 100) + '%' : '--' }}
              </span>
            </div>
            <div class="pg-lane-stat">
              <span class="pg-lane-stat__label">采样时间</span>
              <span class="pg-lane-stat__val mono">
                {{ formatClock(current?.lane?.sample_at) }}
              </span>
            </div>
          </div>
        </div>
      </PgCard>

      <!-- 摄像头实时画面 / 抓拍预览 -->
      <PgCard
        title="车载摄像头视觉画面"
        subtitle="OV5647 传感器捕获流与快照证据"
        icon="camera"
        accent="var(--pg-primary)"
      >
        <template #extra>
          <div class="pg-cam-actions">
            <el-button
              size="small"
              type="primary"
              :loading="snapshotSubmitting"
              @click="triggerSnapshot"
            >
              <PgIcon name="camera" :size="13" />
              <span>抓拍一帧</span>
            </el-button>
          </div>
        </template>

        <div class="pg-camera-screen">
          <div v-if="frameUrl" class="pg-camera-img-wrap">
            <img :src="frameUrl" alt="Camera Stream" class="pg-camera-img" />
            <!-- HUD 视界十字瞄准线与信息遮罩 -->
            <div class="pg-hud-crosshair" />
            <div class="pg-cam-hud-badge top-left mono">
              <span>CAM 01 · 320x180</span>
            </div>
            <div class="pg-cam-hud-badge top-right mono">
              <span>{{ formatClock(frameCapturedAt) }}</span>
            </div>
            <div class="pg-cam-hud-badge bottom-left mono truncate" style="max-width: 220px">
              <span>ID: {{ frameSnapshotId }}</span>
            </div>
          </div>
          <PgEmpty
            v-else
            icon="camera"
            title="暂未捕获图像画面"
            description="边缘端尚未上传画面帧，可点击右上角「抓拍一帧」下发拍摄命令"
          />
        </div>
      </PgCard>
    </section>

    <!-- 下部：双联动态趋势图表 -->
    <section class="pg-charts-grid">
      <PgCard
        title="前方距离与预警阈值时序"
        subtitle="超声波 HC-SR04 采样趋势（米）"
        icon="activity"
      >
        <BaseChart :option="distanceOption" height="230px" />
      </PgCard>

      <PgCard
        title="动力学车速与偏航角速度曲线"
        subtitle="车辆平顺性与急转弯监测动态"
        icon="activity"
      >
        <BaseChart :option="speedOption" height="230px" />
      </PgCard>
    </section>

    <!-- 底部：传感器健康状态矩阵与实时告警队列 -->
    <section class="pg-bottom-grid">
      <!-- 传感器矩阵 -->
      <PgCard
        title="传感器硬件与采样链状态"
        subtitle="Raspberry Pi 4B GPIO / I2C / CSI 总线连接监测"
        icon="cpu"
        class="pg-sensor-card"
      >
        <el-table :data="sensorRows" size="default" class="pg-sensor-table">
          <el-table-column label="传感器" min-width="140">
            <template #default="{ row }">
              <div class="pg-sensor-cell">
                <div class="pg-sensor-icon">
                  <PgIcon :name="row.icon" :size="16" />
                </div>
                <div>
                  <div class="pg-sensor-name">{{ row.name }}</div>
                  <div class="pg-sensor-sub mono">{{ row.model }}</div>
                </div>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="工作状态" width="105">
            <template #default="{ row }">
              <PgBadge :tone="row.statusMeta.tone" size="sm" :dot="true">
                {{ row.statusMeta.label }}
              </PgBadge>
            </template>
          </el-table-column>

          <el-table-column label="数据新鲜度" width="105">
            <template #default="{ row }">
              <PgBadge :tone="row.freshnessMeta.tone" size="sm" variant="outline">
                {{ row.freshnessMeta.label }}
              </PgBadge>
            </template>
          </el-table-column>

          <el-table-column label="采样来源" width="90">
            <template #default="{ row }">
              <span class="mono faint">{{ row.source }}</span>
            </template>
          </el-table-column>

          <el-table-column label="最后采样时间" min-width="120" align="right">
            <template #default="{ row }">
              <span class="mono muted">{{ formatClock(row.sampleAt, true) }}</span>
            </template>
          </el-table-column>
        </el-table>
      </PgCard>

      <!-- 实时告警记录流水 -->
      <PgCard
        title="近期告警事件流"
        subtitle="实时 WebSocket 毫秒级推送历史"
        icon="bell"
        class="pg-recent-events-card"
      >
        <template #extra>
          <router-link to="/events" class="pg-card-link">
            <span>完整记录</span>
            <PgIcon name="chevronRight" :size="14" />
          </router-link>
        </template>

        <div v-if="events.length > 0" class="pg-event-feed">
          <div
            v-for="ev in events.slice(0, 7)"
            :key="ev.event_id"
            class="pg-event-feed-item"
            :class="`level-${ev.level}`"
          >
            <div class="pg-event-icon-col">
              <PgIcon
                :name="EVENT_META[ev.event_type as keyof typeof EVENT_META]?.icon || 'alert'"
                :size="16"
              />
            </div>
            <div class="pg-event-main-col">
              <div class="pg-event-title-row">
                <span class="pg-event-type-label">
                  {{ EVENT_META[ev.event_type as keyof typeof EVENT_META]?.label || ev.event_type }}
                </span>
                <PgBadge
                  :tone="ev.level === 'danger' ? 'danger' : ev.level === 'warning' ? 'warn' : 'ok'"
                  size="sm"
                >
                  {{ ev.level }}
                </PgBadge>
                <span class="pg-event-action-tag mono">{{ ev.action }}</span>
              </div>
              <div class="pg-event-detail-row">
                <span v-for="d in describeEventData(ev.data).slice(0, 3)" :key="d.key" class="pg-data-chip">
                  {{ d.label }}: <b>{{ d.value }}</b>
                </span>
              </div>
            </div>
            <div class="pg-event-time-col mono">
              {{ formatClock(ev.timestamp) }}
            </div>
          </div>
        </div>
        <PgEmpty
          v-else
          icon="shieldCheck"
          title="系统安全良好"
          description="当前未发生任何预警或故障事件"
          :compact="true"
        />
      </PgCard>
    </section>
  </div>
</template>

<style scoped>
.pg-dashboard {
  display: flex;
  flex-direction: column;
  gap: var(--pg-gap);
}

.pg-alert-banner {
  background: rgba(251, 191, 36, 0.12);
  border: 1px solid rgba(251, 191, 36, 0.35);
  border-radius: var(--pg-radius-sm);
  color: #fef08a;
}

.pg-alert-title {
  font-weight: 650;
  font-size: 13.5px;
}

/* 顶部座舱综合安全条 */
.pg-risk-cockpit {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 22px 28px;
  border-radius: var(--pg-radius-lg);
  background: var(--pg-surface-solid);
  border: 1px solid var(--pg-border-strong);
  box-shadow: var(--pg-shadow), var(--pg-inset-highlight);
  overflow: hidden;
}

.pg-cockpit-glow {
  position: absolute;
  top: 0;
  left: 0;
  bottom: 0;
  width: 320px;
  background: radial-gradient(circle at 10% 50%, var(--tone-soft) 0%, transparent 80%);
  pointer-events: none;
  opacity: 0.8;
}

.pg-cockpit-left {
  display: flex;
  align-items: center;
  gap: 20px;
  min-width: 0;
  position: relative;
  z-index: 1;
}

.pg-shield-radar {
  position: relative;
  width: 58px;
  height: 58px;
  flex-shrink: 0;
  display: grid;
  place-items: center;
}

.pg-radar-ring {
  position: absolute;
  border-radius: 50%;
  border: 1.5px solid var(--tone);
  opacity: 0.35;
}

.pg-radar-ring--1 {
  inset: 0;
  animation: pg-ping 2.6s cubic-bezier(0, 0, 0.2, 1) infinite;
}

.pg-radar-ring--2 {
  inset: -6px;
  opacity: 0.15;
}

.pg-radar-core {
  width: 46px;
  height: 46px;
  border-radius: 14px;
  background: var(--tone-soft);
  border: 1px solid color-mix(in srgb, var(--tone) 40%, transparent);
  color: var(--tone);
  display: grid;
  place-items: center;
  box-shadow: 0 0 16px var(--tone-soft);
}

.pg-risk-desc-box {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.pg-risk-badge-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 4px;
}

.pg-risk-label {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--pg-text-muted);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.pg-risk-headline {
  font-size: 20px;
  font-weight: 750;
  color: #fff;
  line-height: 1.25;
  margin-bottom: 6px;
}

.pg-risk-submeta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--pg-text-muted);
}

.pg-sep {
  color: var(--pg-border-strong);
}

.pg-cockpit-right {
  display: flex;
  align-items: center;
  gap: 16px;
  position: relative;
  z-index: 1;
}

.pg-active-alarm-pool {
  display: flex;
  flex-direction: column;
  gap: 6px;
  background: rgba(10, 16, 28, 0.45);
  border: 1px solid var(--pg-border);
  padding: 8px 14px;
  border-radius: var(--pg-radius-sm);
  min-width: 220px;
}

.pg-pool-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  color: var(--pg-text-muted);
}

.pg-pool-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.pg-pool-empty {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--pg-text-muted);
}

.text-ok {
  color: var(--pg-ok);
}

.pg-cockpit-refresh-btn {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  border: 1px solid var(--pg-border);
  background: var(--pg-surface-2);
  color: var(--pg-text-muted);
  cursor: pointer;
  display: grid;
  place-items: center;
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.pg-cockpit-refresh-btn:hover {
  background: var(--pg-surface-hover);
  color: var(--pg-text);
  border-color: var(--pg-border-strong);
}

/* 四大指标卡片 */
.pg-metric-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--pg-gap);
}

@media (max-width: 1200px) {
  .pg-metric-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 640px) {
  .pg-metric-grid {
    grid-template-columns: 1fr;
  }
}

.pg-metric-card {
  min-height: 140px;
}

.pg-metric-inner {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.pg-metric-top {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pg-metric-icon-box {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--tone-soft);
  color: var(--tone);
  border: 1px solid color-mix(in srgb, var(--tone) 30%, transparent);
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.pg-metric-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--pg-text-2);
}

.pg-metric-tag {
  margin-left: auto;
}

.pg-metric-val-row {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-top: 2px;
}

.pg-metric-num {
  font-size: 32px;
  font-weight: 750;
  color: #fff;
  line-height: 1;
  letter-spacing: -0.02em;
}

.pg-metric-unit {
  font-size: 13px;
  color: var(--pg-text-muted);
  font-weight: 500;
}

.pg-metric-bar {
  height: 5px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.12);
  overflow: hidden;
}

.pg-metric-bar-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.4s var(--pg-ease);
}

.pg-metric-subtext {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11.5px;
  color: var(--pg-text-faint);
}

.pg-ttc-hint b {
  color: var(--pg-warn);
}

/* 视觉与车道保持区 */
.pg-vision-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--pg-gap);
}

@media (max-width: 992px) {
  .pg-vision-grid {
    grid-template-columns: 1fr;
  }
}

.pg-lane-hud {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.pg-lane-canvas {
  position: relative;
  height: 170px;
  border-radius: var(--pg-radius-sm);
  background: radial-gradient(circle at 50% 0%, #152238 0%, #080d17 100%);
  border: 1px solid var(--pg-border);
  overflow: hidden;
  perspective: 350px;
}

.pg-lane-horizon {
  position: absolute;
  top: 15%;
  left: 0;
  right: 0;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(34, 211, 238, 0.3), transparent);
}

.pg-road-surface {
  position: absolute;
  inset: 15% 10% 0 10%;
  transform: rotateX(42deg);
  transform-origin: 50% 0%;
  background: linear-gradient(180deg, rgba(30, 41, 64, 0.4) 0%, rgba(10, 16, 28, 0.9) 100%);
  border-left: 2px solid rgba(148, 163, 184, 0.25);
  border-right: 2px solid rgba(148, 163, 184, 0.25);
}

.pg-road-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 4px;
  transition: all 0.3s;
}

.pg-road-line--left {
  left: 14%;
  background: #f59e0b;
}

.pg-road-line--left.active {
  background: #fb5174;
  box-shadow: 0 0 12px #fb5174;
}

.pg-road-line--right {
  right: 14%;
  background: #f59e0b;
}

.pg-road-line--right.active {
  background: #fb5174;
  box-shadow: 0 0 12px #fb5174;
}

.pg-road-center-stripes {
  position: absolute;
  top: 0;
  bottom: 0;
  left: calc(50% - 1.5px);
  width: 3px;
  background: repeating-linear-gradient(
    180deg,
    #ffffff 0,
    #ffffff 18px,
    transparent 18px,
    transparent 36px
  );
  opacity: 0.6;
}

.pg-vehicle-pointer {
  position: absolute;
  bottom: 16px;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  transition: left 0.25s cubic-bezier(0.2, 0.8, 0.3, 1);
  z-index: 2;
}

.pg-vehicle-halo {
  position: absolute;
  width: 50px;
  height: 50px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.4) 0%, transparent 70%);
  top: -8px;
}

.pg-vehicle-halo.alert {
  background: radial-gradient(circle, rgba(251, 81, 116, 0.6) 0%, transparent 70%);
}

.pg-vehicle-body {
  position: relative;
  color: #fff;
  filter: drop-shadow(0 4px 10px rgba(0, 0, 0, 0.8));
}

.pg-vehicle-axis {
  width: 2px;
  height: 18px;
  background: #22d3ee;
  box-shadow: 0 0 8px #22d3ee;
}

.pg-lane-telemetry-strip {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  background: rgba(15, 23, 38, 0.5);
  border: 1px solid var(--pg-border);
  padding: 10px 14px;
  border-radius: var(--pg-radius-sm);
}

.pg-lane-stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pg-lane-stat__label {
  font-size: 11px;
  color: var(--pg-text-muted);
}

.pg-lane-stat__val {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--pg-text);
}

.text-danger {
  color: var(--pg-danger) !important;
}

/* 摄像头监视器 */
.pg-camera-screen {
  position: relative;
  height: 220px;
  background: #030712;
  border-radius: var(--pg-radius-sm);
  border: 1px solid var(--pg-border);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.pg-camera-img-wrap {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.pg-camera-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.pg-hud-crosshair {
  position: absolute;
  width: 40px;
  height: 40px;
  border: 1px solid rgba(34, 211, 238, 0.45);
  border-radius: 50%;
  pointer-events: none;
}

.pg-cam-hud-badge {
  position: absolute;
  padding: 3px 8px;
  border-radius: 4px;
  background: rgba(3, 7, 18, 0.75);
  backdrop-filter: blur(8px);
  color: #22d3ee;
  font-size: 11px;
  border: 1px solid rgba(34, 211, 238, 0.25);
  pointer-events: none;
}

.pg-cam-hud-badge.top-left { top: 8px; left: 8px; }
.pg-cam-hud-badge.top-right { top: 8px; right: 8px; color: #94a3b8; }
.pg-cam-hud-badge.bottom-left { bottom: 8px; left: 8px; color: #94a3b8; }

/* 趋势图表网格 */
.pg-charts-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--pg-gap);
}

@media (max-width: 992px) {
  .pg-charts-grid {
    grid-template-columns: 1fr;
  }
}

/* 底部状态矩阵与最近告警 */
.pg-bottom-grid {
  display: grid;
  grid-template-columns: 1.15fr 0.85fr;
  gap: var(--pg-gap);
}

@media (max-width: 1080px) {
  .pg-bottom-grid {
    grid-template-columns: 1fr;
  }
}

.pg-sensor-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pg-sensor-icon {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: var(--pg-surface-2);
  display: grid;
  place-items: center;
  color: var(--pg-primary);
  border: 1px solid var(--pg-border);
}

.pg-sensor-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--pg-text);
}

.pg-sensor-sub {
  font-size: 11px;
  color: var(--pg-text-faint);
}

.pg-card-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--pg-primary);
}

.pg-card-link:hover {
  text-decoration: underline;
}

/* 告警信息流水 */
.pg-event-feed {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 250px;
  overflow-y: auto;
}

.pg-event-feed-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border-radius: var(--pg-radius-sm);
  background: rgba(15, 23, 38, 0.45);
  border: 1px solid var(--pg-border);
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.pg-event-feed-item:hover {
  background: var(--pg-surface-hover);
  border-color: var(--pg-border-strong);
}

.pg-event-icon-col {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--pg-surface-2);
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.level-danger .pg-event-icon-col {
  background: rgba(251, 81, 116, 0.15);
  color: var(--pg-danger);
}

.level-warning .pg-event-icon-col {
  background: rgba(251, 191, 36, 0.15);
  color: var(--pg-warn);
}

.pg-event-main-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.pg-event-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pg-event-type-label {
  font-size: 12.5px;
  font-weight: 650;
  color: var(--pg-text);
}

.pg-event-action-tag {
  font-size: 11px;
  color: var(--pg-text-faint);
}

.pg-event-detail-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.pg-data-chip {
  font-size: 11px;
  color: var(--pg-text-muted);
}

.pg-data-chip b {
  color: var(--pg-text-2);
}

.pg-event-time-col {
  font-size: 11.5px;
  color: var(--pg-text-faint);
  flex-shrink: 0;
}
</style>
