<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useDeviceStore } from '@/stores/device'
import { useRealtime } from '@/composables/useRealtime'
import { getDeviceState } from '@/api/device'
import { listTelemetry } from '@/api/telemetry'
import { getLatestFrame, snapshotContentUrl } from '@/api/frame'
import BaseChart from '@/components/BaseChart.vue'
import {
  formatNumber,
  riskLabel,
  sensorStatusText,
  sensorStatusTag,
  freshnessText,
  freshnessTag,
  directionLabel,
} from '@/utils/format'
import type { DeviceState, RiskLevel } from '@/types'
import type { EChartsOption } from 'echarts'

const device = useDeviceStore()
const { telemetry, events, connect } = useRealtime()

const state = ref<DeviceState | null>(null)
const frameUrl = ref<string>('')
const distanceHistory = ref<{ t: string; v: number | null }[]>([])
const speedHistory = ref<{ t: string; v: number | null }[]>([])

const current = computed<DeviceState | null>(() => {
  if (telemetry.value) {
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

const riskColor = computed(() => {
  const map: Record<RiskLevel, string> = {
    normal: 'var(--pg-risk-normal)',
    warning: 'var(--pg-risk-warning)',
    danger: 'var(--pg-risk-danger)',
    unknown: 'var(--pg-risk-unknown)',
  }
  return map[current.value?.risk_level ?? 'unknown']
})

// 距离趋势图
const distanceOption = computed(() => {
  const data = distanceHistory.value.map((d) => [d.t, d.v])
  return {
    grid: { left: 40, right: 16, top: 20, bottom: 30 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'time',
      axisLabel: { color: '#94a3b8', fontSize: 10 },
      axisLine: { lineStyle: { color: '#334155' } },
    },
    yAxis: {
      type: 'value',
      name: 'm',
      nameTextStyle: { color: '#94a3b8' },
      axisLabel: { color: '#94a3b8' },
      splitLine: { lineStyle: { color: '#1e293b' } },
    },
    series: [
      {
        type: 'line',
        data,
        smooth: true,
        symbol: 'none',
        lineStyle: { color: '#38bdf8', width: 2 },
        areaStyle: { color: 'rgba(56,189,248,0.15)' },
      },
    ],
  } as EChartsOption
})

// 速度趋势图
const speedOption = computed(() => {
  const data = speedHistory.value.map((d) => [d.t, d.v])
  return {
    grid: { left: 40, right: 16, top: 20, bottom: 30 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'time',
      axisLabel: { color: '#94a3b8', fontSize: 10 },
      axisLine: { lineStyle: { color: '#334155' } },
    },
    yAxis: {
      type: 'value',
      name: 'km/h',
      nameTextStyle: { color: '#94a3b8' },
      axisLabel: { color: '#94a3b8' },
      splitLine: { lineStyle: { color: '#1e293b' } },
    },
    series: [
      {
        type: 'line',
        data,
        smooth: true,
        symbol: 'none',
        lineStyle: { color: '#a78bfa', width: 2 },
        areaStyle: { color: 'rgba(167,139,250,0.15)' },
      },
    ],
  } as EChartsOption
})

// 车道偏离可视化
const laneOffsetPct = computed(() => {
  const v = current.value?.lane.offset_ratio
  if (v === undefined || v === null) return 50
  const pct = Math.max(0, Math.min(100, 50 + v * 100))
  return pct
})

function pushHistory(arr: { t: string; v: number | null }[], t: string, v: number | null | undefined) {
  arr.push({ t, v: v ?? null })
  if (arr.length > 60) arr.shift()
}

async function loadState() {
  try {
    const s = await getDeviceState(device.currentDeviceId)
    state.value = s
  } catch {
    /* ignore */
  }
  // 拉取最近遥测历史，预填充趋势图
  try {
    const res = await listTelemetry(device.currentDeviceId, { limit: 60 })
    const items = [...res.items].reverse()
    for (const it of items) {
      pushHistory(distanceHistory.value, it.timestamp, it.distance)
      pushHistory(speedHistory.value, it.timestamp, it.speed)
    }
  } catch {
    /* ignore */
  }
}

async function loadFrame() {
  try {
    const f = await getLatestFrame(device.currentDeviceId)
    if (f?.snapshot_id) frameUrl.value = snapshotContentUrl(f.snapshot_id)
  } catch {
    frameUrl.value = ''
  }
}

onMounted(async () => {
  connect()
  await loadState()
  await loadFrame()
  setInterval(loadFrame, 3000)
})

// 监听实时数据更新历史
watch(
  () => telemetry.value,
  (t) => {
    if (!t) return
    const ts = t.timestamp || new Date().toISOString()
    pushHistory(distanceHistory.value, ts, t.distance_m)
    pushHistory(speedHistory.value, ts, t.speed_kmh)
  },
)

const sensorRows = computed(() => {
  const s = current.value
  if (!s) return []
  const names: { key: string; label: string }[] = [
    { key: 'distance', label: '超声波距离' },
    { key: 'gyro', label: '陀螺仪' },
    { key: 'temperature', label: '温度' },
    { key: 'lane', label: '摄像头车道' },
    { key: 'speed', label: '车速' },
  ]
  return names.map((n) => ({
    name: n.label,
    status: (s.samples?.[n.key]?.status ?? 'unavailable') as never,
    freshness: (s.freshness?.[n.key] ?? 'missing') as never,
    source: s.samples?.[n.key]?.source ?? '--',
  }))
})
</script>

<template>
  <div class="dashboard">
    <!-- 风险等级 + 在线状态 -->
    <div class="risk-banner" :style="{ borderColor: riskColor }">
      <div class="risk-left">
        <div class="risk-label">综合风险</div>
        <div class="risk-value" :style="{ color: riskColor }">
          {{ riskLabel(current?.risk_level ?? 'unknown') }}
        </div>
      </div>
      <div class="risk-meta">
        <el-tag :type="current?.online ? 'success' : 'info'" effect="dark">
          {{ current?.online ? '设备在线' : '设备离线' }}
        </el-tag>
        <span class="time">{{ current?.timestamp ? new Date(current.timestamp).toLocaleTimeString() : '--' }}</span>
      </div>
      <div class="active-events" v-if="current?.active_events?.length">
        <span class="ae-label">活跃告警：</span>
        <el-tag v-for="e in current.active_events" :key="e" size="small" type="warning" effect="plain">
          {{ e }}
        </el-tag>
      </div>
    </div>

    <el-row :gutter="16" class="metrics">
      <!-- 车速 -->
      <el-col :span="6">
        <el-card shadow="never" class="metric-card">
          <div class="metric-icon speed">S</div>
          <div class="metric-info">
            <div class="metric-value">{{ formatNumber(current?.speed_kmh) }}</div>
            <div class="metric-unit">km/h</div>
            <div class="metric-name">车速</div>
          </div>
        </el-card>
      </el-col>
      <!-- 距离 -->
      <el-col :span="6">
        <el-card shadow="never" class="metric-card">
          <div class="metric-icon distance">D</div>
          <div class="metric-info">
            <div class="metric-value">{{ formatNumber(current?.distance_m) }}</div>
            <div class="metric-unit">m</div>
            <div class="metric-name">前方距离</div>
          </div>
        </el-card>
      </el-col>
      <!-- 温度 -->
      <el-col :span="6">
        <el-card shadow="never" class="metric-card">
          <div class="metric-icon temp">T</div>
          <div class="metric-info">
            <div class="metric-value">{{ formatNumber(current?.temperature_c) }}</div>
            <div class="metric-unit">°C</div>
            <div class="metric-name">车内温度</div>
          </div>
        </el-card>
      </el-col>
      <!-- 角速度 -->
      <el-col :span="6">
        <el-card shadow="never" class="metric-card">
          <div class="metric-icon gyro">Y</div>
          <div class="metric-info">
            <div class="metric-value">{{ formatNumber(current?.yaw_rate_deg_s) }}</div>
            <div class="metric-unit">°/s</div>
            <div class="metric-name">偏航角速度</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="charts-row">
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>前方距离趋势</template>
          <BaseChart :option="distanceOption" height="220px" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>车速趋势</template>
          <BaseChart :option="speedOption" height="220px" />
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <!-- 车道偏离 -->
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>车道偏离</template>
          <div class="lane-panel">
            <div class="lane-track">
              <div class="lane-left" />
              <div class="lane-center" :style="{ left: laneOffsetPct + '%' }">
                <div class="car-icon">🚗</div>
              </div>
              <div class="lane-right" />
            </div>
            <div class="lane-info">
              <div>
                偏移：<b>{{ formatNumber(current?.lane.offset_ratio, 2) }}</b>
                （{{ directionLabel(current?.lane.direction ?? 'unknown') }}）
              </div>
              <el-tag :type="current?.lane.valid ? 'success' : 'info'" size="small">
                {{ current?.lane.valid ? '检测有效' : '未检测到车道' }}
              </el-tag>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 摄像头预览 -->
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>摄像头预览</template>
          <div class="cam-panel">
            <img v-if="frameUrl" :src="frameUrl" class="cam-img" alt="preview" />
            <div v-else class="cam-empty">暂无预览画面</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" style="margin-top: 16px">
      <!-- 传感器状态 -->
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>传感器状态</template>
          <el-table :data="sensorRows" size="small">
            <el-table-column prop="name" label="传感器" />
            <el-table-column label="状态">
              <template #default="{ row }">
                <el-tag :type="sensorStatusTag(row.status)" size="small">
                  {{ sensorStatusText(row.status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="新鲜度">
              <template #default="{ row }">
                <el-tag :type="freshnessTag(row.freshness)" size="small" effect="plain">
                  {{ freshnessText(row.freshness) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="source" label="来源" />
          </el-table>
        </el-card>
      </el-col>

      <!-- 最近告警 -->
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>最近告警</template>
          <div v-if="events.length === 0" class="empty">暂无告警</div>
          <div v-else class="event-list">
            <div v-for="e in events.slice(0, 8)" :key="e.event_id" class="event-item">
              <el-tag :type="e.level === 'danger' ? 'danger' : e.level === 'warning' ? 'warning' : 'success'" size="small">
                {{ e.level }}
              </el-tag>
              <span class="event-type">{{ e.event_type }}</span>
              <span class="event-action">{{ e.action }}</span>
              <span class="event-time">{{ new Date(e.timestamp).toLocaleTimeString() }}</span>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.risk-banner {
  display: flex;
  align-items: center;
  gap: 24px;
  padding: 16px 24px;
  background: var(--pg-bg-elevated);
  border: 1px solid var(--pg-border);
  border-left: 4px solid;
  border-radius: 12px;
}

.risk-label {
  font-size: 13px;
  color: var(--pg-text-muted);
}

.risk-value {
  font-size: 28px;
  font-weight: 800;
  margin-top: 2px;
}

.risk-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--pg-text-faint);
}

.active-events {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
}

.ae-label {
  color: var(--pg-text-muted);
  font-size: 13px;
}

.metrics {
  margin: 0;
}

.metric-card {
  border-radius: 12px;
}

.metric-card :deep(.el-card__body) {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
}

.metric-icon {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 800;
  font-size: 22px;
  color: #fff;
}

.metric-icon.speed { background: linear-gradient(135deg, #a78bfa, #7c3aed); }
.metric-icon.distance { background: linear-gradient(135deg, #38bdf8, #0284c7); }
.metric-icon.temp { background: linear-gradient(135deg, #fb923c, #ea580c); }
.metric-icon.gyro { background: linear-gradient(135deg, #34d399, #059669); }

.metric-value {
  font-size: 28px;
  font-weight: 700;
  color: #fff;
  line-height: 1;
}

.metric-unit {
  font-size: 12px;
  color: var(--pg-text-muted);
  margin-top: 2px;
}

.metric-name {
  font-size: 13px;
  color: var(--pg-text-faint);
  margin-top: 4px;
}

.charts-row {
  margin: 0;
}

.lane-panel {
  padding: 8px 0;
}

.lane-track {
  position: relative;
  height: 100px;
  background: linear-gradient(180deg, #1e293b 0%, #0f172a 100%);
  border-radius: 10px;
  overflow: hidden;
  margin-bottom: 12px;
}

.lane-left,
.lane-right {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 3px;
  background: repeating-linear-gradient(
    180deg,
    #fbbf24 0 12px,
    transparent 12px 24px
  );
}

.lane-left { left: 12%; }
.lane-right { right: 12%; }

.lane-center {
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%);
  transition: left 0.3s;
}

.car-icon {
  font-size: 28px;
}

.lane-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 14px;
  color: var(--pg-text-muted);
}

.cam-panel {
  height: 220px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #0f172a;
  border-radius: 8px;
  overflow: hidden;
}

.cam-img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.cam-empty {
  color: var(--pg-text-faint);
}

.empty {
  text-align: center;
  color: var(--pg-text-faint);
  padding: 24px 0;
}

.event-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 220px;
  overflow-y: auto;
}

.event-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--pg-bg);
  border-radius: 8px;
  font-size: 13px;
}

.event-type {
  color: var(--pg-text);
  flex: 1;
}

.event-action {
  color: var(--pg-text-muted);
}

.event-time {
  color: var(--pg-text-faint);
  font-size: 12px;
}
</style>
