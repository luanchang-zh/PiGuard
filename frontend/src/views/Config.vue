<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useDeviceStore } from '@/stores/device'
import { getConfig, updateConfig } from '@/api/config'
import PgCard from '@/components/ui/PgCard.vue'
import PgBadge from '@/components/ui/PgBadge.vue'
import PgIcon from '@/components/ui/PgIcon.vue'
import { formatTime } from '@/utils/format'
import type { ConfigView, RuleSet } from '@/types'

const device = useDeviceStore()
const loading = ref(false)
const saving = ref(false)
const config = ref<ConfigView | null>(null)
let pollTimer: ReturnType<typeof setInterval> | undefined

// 表单对象深拷贝
const form = reactive<RuleSet>({
  obstacle: { warning_distance_m: 15.0, danger_distance_m: 8.0 },
  lane_departure: { offset_threshold: 0.35, duration_ms: 1000 },
  sharp_turn: { yaw_threshold_deg_s: 30.0, min_speed_kmh: 20.0, duration_ms: 300 },
  temperature: { trigger_c: 35.0, recover_c: 33.0, duration_ms: 5000 },
})

const isSynced = computed(() => {
  if (!config.value) return false
  return (
    config.value.status === 'success' &&
    config.value.desired_version === config.value.reported_version
  )
})

const syncStatusMeta = computed(() => {
  if (!config.value) return { label: '未同步', tone: 'unknown' as const }
  switch (config.value.status) {
    case 'success':
      return { label: '配置已同步生效', tone: 'ok' as const }
    case 'failed':
      return { label: '边缘端应用失败', tone: 'danger' as const }
    case 'pending':
    default:
      return { label: '正在向边缘端下发同步...', tone: 'warn' as const }
  }
})

// 校验规则是否有效
const validationErrors = computed(() => {
  const errs: string[] = []
  if (form.obstacle.danger_distance_m >= form.obstacle.warning_distance_m) {
    errs.push('障碍物危险距离必须严格小于警告距离')
  }
  if (form.temperature.recover_c >= form.temperature.trigger_c) {
    errs.push('高温恢复温度必须严格低于触发温度（迟滞回差机制）')
  }
  if (form.lane_departure.offset_threshold <= 0 || form.lane_departure.offset_threshold > 1) {
    errs.push('车道偏离阈值须在 0.01 到 1.00 之间')
  }
  return errs
})

async function load() {
  loading.value = true
  try {
    const c = await getConfig(device.currentDeviceId, true)
    config.value = c
    if (c.rules) {
      form.obstacle = { ...c.rules.obstacle }
      form.lane_departure = { ...c.rules.lane_departure }
      form.sharp_turn = { ...c.rules.sharp_turn }
      form.temperature = { ...c.rules.temperature }
    }
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!config.value) return
  if (validationErrors.value.length > 0) {
    ElMessage.error(validationErrors.value[0])
    return
  }

  saving.value = true
  try {
    const res = await updateConfig(device.currentDeviceId, {
      expected_version: config.value.desired_version,
      rules: form,
    })
    ElMessage.success(`新配置已提交受理，版本号 v${res.config_version}，正在等待设备确认`)
    await load()
  } catch {
    await load()
  } finally {
    saving.value = false
  }
}

function restoreDefaults() {
  form.obstacle = { warning_distance_m: 15.0, danger_distance_m: 8.0 }
  form.lane_departure = { offset_threshold: 0.35, duration_ms: 1000 }
  form.sharp_turn = { yaw_threshold_deg_s: 30.0, min_speed_kmh: 20.0, duration_ms: 300 }
  form.temperature = { trigger_c: 35.0, recover_c: 33.0, duration_ms: 5000 }
  ElMessage.info('已恢复为课设官方基准推荐阈值')
}

onMounted(() => {
  load()
  pollTimer = setInterval(async () => {
    if (config.value?.status !== 'pending' || saving.value) return
    try {
      config.value = await getConfig(device.currentDeviceId, true)
    } catch {
      /* ignore */
    }
  }, 2000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="pg-page pg-config-page pg-stagger">
    <!-- 顶部配置同步状态英雄榜 -->
    <PgCard padding="md" class="pg-config-status-card" :accent="syncStatusMeta.tone === 'ok' ? 'var(--pg-ok)' : 'var(--pg-warn)'">
      <div class="pg-config-hero">
        <div class="pg-hero-left">
          <div class="pg-version-shield" :class="`tone-${syncStatusMeta.tone}`">
            <PgIcon
              :name="syncStatusMeta.tone === 'ok' ? 'shieldCheck' : syncStatusMeta.tone === 'danger' ? 'xCircle' : 'rotate'"
              :size="26"
              :spin="config?.status === 'pending'"
            />
          </div>
          <div class="pg-hero-text">
            <div class="pg-hero-tags">
              <span class="pg-hero-title">ADAS 预警规则引擎参数配置</span>
              <PgBadge :tone="syncStatusMeta.tone" size="md" :dot="true" :pulse="config?.status === 'pending'">
                {{ syncStatusMeta.label }}
              </PgBadge>
            </div>
            <p class="pg-hero-desc">
              向树莓派边缘端规则链实时分发预警阈值。支持多模块原子更新与乐观锁版本冲突校验。
            </p>
          </div>
        </div>

        <div class="pg-hero-right">
          <div class="pg-version-chip">
            <span class="v-label">期望版本:</span>
            <span class="v-num mono">v{{ config?.desired_version ?? '--' }}</span>
          </div>
          <div class="pg-version-chip" :class="{ 'v-match': isSynced }">
            <span class="v-label">设备版本:</span>
            <span class="v-num mono">v{{ config?.reported_version ?? '--' }}</span>
          </div>
        </div>
      </div>

      <div v-if="config?.applied_at || config?.ack_at" class="pg-sync-meta-bar">
        <span v-if="config.ack_at" class="mono muted">
          设备回执确认时间: {{ formatTime(config.ack_at) }}
        </span>
        <span v-if="config.applied_at" class="mono muted">
          设备生效时间: {{ formatTime(config.applied_at) }}
        </span>
        <span v-if="config.error" class="mono text-danger">
          异常报错: {{ config.error.code }} - {{ config.error.message }}
        </span>
      </div>
    </PgCard>

    <!-- 校验告警提示 -->
    <el-alert
      v-if="validationErrors.length > 0"
      type="error"
      show-icon
      :closable="false"
      class="pg-val-alert"
    >
      <template #title>参数配置存在冲突：{{ validationErrors[0] }}</template>
    </el-alert>

    <!-- 四大规则参数分组卡片 -->
    <div class="pg-rules-grid">
      <!-- 1. 前方超声波障碍物预警 -->
      <PgCard
        title="1. 前方障碍物预警规则 (Obstacle)"
        subtitle="基于 HC-SR04 超声波测距与行车安全距离分级"
        icon="radar"
        accent="var(--pg-c-distance)"
      >
        <div class="pg-rule-content">
          <div class="pg-rule-row">
            <span class="label">警告距离 (Warning):</span>
            <el-input-number
              v-model="form.obstacle.warning_distance_m"
              :min="0.2"
              :max="30"
              :step="0.5"
              :precision="1"
            />
            <span class="unit">米 (m)</span>
          </div>

          <div class="pg-rule-row">
            <span class="label">危险距离 (Danger):</span>
            <el-input-number
              v-model="form.obstacle.danger_distance_m"
              :min="0.1"
              :max="form.obstacle.warning_distance_m - 0.1"
              :step="0.5"
              :precision="1"
            />
            <span class="unit">米 (m)</span>
            <span class="hint">（须小于警告距离）</span>
          </div>

          <!-- 距离区间条形图可视化 -->
          <div class="pg-dist-range-bar">
            <div
              class="range-seg seg-danger"
              :style="{ width: `${(form.obstacle.danger_distance_m / 25) * 100}%` }"
            >
              <span>危险 &lt;{{ form.obstacle.danger_distance_m }}m</span>
            </div>
            <div
              class="range-seg seg-warn"
              :style="{
                width: `${((form.obstacle.warning_distance_m - form.obstacle.danger_distance_m) / 25) * 100}%`,
              }"
            >
              <span>预警 {{ form.obstacle.danger_distance_m }}~{{ form.obstacle.warning_distance_m }}m</span>
            </div>
            <div class="range-seg seg-safe">
              <span>安全 &gt;{{ form.obstacle.warning_distance_m }}m</span>
            </div>
          </div>
        </div>
      </PgCard>

      <!-- 2. 车道偏离预警规则 -->
      <PgCard
        title="2. 车道偏离预警规则 (Lane Departure)"
        subtitle="基于 OV5647 视觉标线偏离持续时间触发"
        icon="road"
        accent="var(--pg-c-lane)"
      >
        <div class="pg-rule-content">
          <div class="pg-rule-row">
            <span class="label">偏离比率阈值:</span>
            <el-input-number
              v-model="form.lane_departure.offset_threshold"
              :min="0.05"
              :max="0.95"
              :step="0.05"
              :precision="2"
            />
            <span class="hint">（0 ~ 1.00，推荐 0.35）</span>
          </div>

          <div class="pg-rule-row">
            <span class="label">持续防抖时间:</span>
            <el-input-number
              v-model="form.lane_departure.duration_ms"
              :min="100"
              :max="10000"
              :step="100"
            />
            <span class="unit">ms</span>
            <span class="hint mono">({{ (form.lane_departure.duration_ms / 1000).toFixed(1) }} 秒)</span>
          </div>

          <div class="pg-rule-desc-box">
            <span>当车辆横向偏移绝对值连续超过阈值并保持设定时间后，触发车道偏离报警。</span>
          </div>
        </div>
      </PgCard>

      <!-- 3. 急转弯预警规则 -->
      <PgCard
        title="3. 急转弯预警规则 (Sharp Turn)"
        subtitle="基于 MPU6050 陀螺仪偏航角速度与最低车速联立判定"
        icon="turn"
        accent="var(--pg-c-yaw)"
      >
        <div class="pg-rule-content">
          <div class="pg-rule-row">
            <span class="label">角速度阈值:</span>
            <el-input-number
              v-model="form.sharp_turn.yaw_threshold_deg_s"
              :min="5"
              :max="120"
              :step="5"
              :precision="1"
            />
            <span class="unit">°/s</span>
          </div>

          <div class="pg-rule-row">
            <span class="label">最低激活车速:</span>
            <el-input-number
              v-model="form.sharp_turn.min_speed_kmh"
              :min="0"
              :max="120"
              :step="5"
              :precision="1"
            />
            <span class="unit">km/h</span>
            <span class="hint">（避免低速调头误报）</span>
          </div>

          <div class="pg-rule-row">
            <span class="label">判定维持时间:</span>
            <el-input-number
              v-model="form.sharp_turn.duration_ms"
              :min="50"
              :max="3000"
              :step="50"
            />
            <span class="unit">ms</span>
          </div>
        </div>
      </PgCard>

      <!-- 4. 车内高温预警规则 -->
      <PgCard
        title="4. 车内高温预警规则 (Temperature)"
        subtitle="基于 AHT20 温湿度传感器与迟滞回差解除"
        icon="flame"
        accent="var(--pg-c-temp)"
      >
        <div class="pg-rule-content">
          <div class="pg-rule-row">
            <span class="label">高温触发温度:</span>
            <el-input-number
              v-model="form.temperature.trigger_c"
              :min="20"
              :max="60"
              :step="0.5"
              :precision="1"
            />
            <span class="unit">°C</span>
          </div>

          <div class="pg-rule-row">
            <span class="label">解除恢复温度:</span>
            <el-input-number
              v-model="form.temperature.recover_c"
              :min="15"
              :max="form.temperature.trigger_c - 0.5"
              :step="0.5"
              :precision="1"
            />
            <span class="unit">°C</span>
            <span class="hint">（须低于触发温度，回差迟滞）</span>
          </div>

          <div class="pg-rule-row">
            <span class="label">持续保持时间:</span>
            <el-input-number
              v-model="form.temperature.duration_ms"
              :min="500"
              :max="30000"
              :step="500"
            />
            <span class="unit">ms</span>
            <span class="hint mono">({{ (form.temperature.duration_ms / 1000).toFixed(1) }} 秒)</span>
          </div>
        </div>
      </PgCard>
    </div>

    <!-- 底部操作按钮栏 -->
    <PgCard padding="sm" class="pg-action-footer">
      <div class="pg-footer-actions">
        <el-button @click="restoreDefaults">
          <PgIcon name="undo" :size="14" />
          <span>恢复官方基准推荐值</span>
        </el-button>

        <el-button @click="load">
          <PgIcon name="refresh" :size="14" />
          <span>重新载入</span>
        </el-button>

        <el-button
          type="primary"
          class="pg-save-btn"
          :loading="saving"
          :disabled="validationErrors.length > 0"
          @click="save"
        >
          <PgIcon name="save" :size="16" />
          <span>保存并下发到边缘端设备</span>
        </el-button>
      </div>
    </PgCard>
  </div>
</template>

<style scoped>
.pg-config-page {
  display: flex;
  flex-direction: column;
  gap: var(--pg-gap);
}

.pg-config-status-card {
  position: relative;
  overflow: hidden;
}

.pg-config-hero {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
}

.pg-hero-left {
  display: flex;
  align-items: center;
  gap: 18px;
  min-width: 0;
}

.pg-version-shield {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  background: var(--tone-soft);
  color: var(--tone);
  border: 1px solid color-mix(in srgb, var(--tone) 35%, transparent);
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.pg-hero-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.pg-hero-tags {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pg-hero-title {
  font-size: 17px;
  font-weight: 700;
  color: #fff;
}

.pg-hero-desc {
  font-size: 12.5px;
  color: var(--pg-text-muted);
  max-width: 580px;
}

.pg-hero-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.pg-version-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(10, 16, 28, 0.55);
  border: 1px solid var(--pg-border);
  padding: 8px 14px;
  border-radius: var(--pg-radius-sm);
}

.v-label {
  font-size: 11.5px;
  color: var(--pg-text-muted);
}

.v-num {
  font-size: 15px;
  font-weight: 750;
  color: #fff;
}

.v-match .v-num {
  color: var(--pg-ok);
}

.pg-sync-meta-bar {
  margin-top: 14px;
  padding-top: 10px;
  border-top: 1px solid var(--pg-border);
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  font-size: 12px;
}

.pg-val-alert {
  border-radius: var(--pg-radius-sm);
}

/* 四大规则卡片网格 */
.pg-rules-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--pg-gap);
}

@media (max-width: 992px) {
  .pg-rules-grid {
    grid-template-columns: 1fr;
  }
}

.pg-rule-content {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.pg-rule-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pg-rule-row .label {
  width: 130px;
  font-size: 13px;
  color: var(--pg-text-2);
  flex-shrink: 0;
}

.unit {
  font-size: 12px;
  color: var(--pg-text-muted);
}

.hint {
  font-size: 11.5px;
  color: var(--pg-text-faint);
}

.pg-rule-desc-box {
  background: rgba(10, 16, 28, 0.4);
  padding: 10px 14px;
  border-radius: var(--pg-radius-xs);
  font-size: 12px;
  color: var(--pg-text-muted);
  line-height: 1.4;
  border: 1px solid var(--pg-border);
}

/* 距离分级条 */
.pg-dist-range-bar {
  display: flex;
  height: 28px;
  border-radius: 6px;
  overflow: hidden;
  margin-top: 6px;
  font-size: 10.5px;
  font-weight: 600;
  text-align: center;
  line-height: 28px;
}

.range-seg {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  padding: 0 4px;
}

.seg-danger {
  background: rgba(251, 81, 116, 0.35);
  color: #ff859d;
  border-right: 2px solid #fb5174;
}

.seg-warn {
  background: rgba(251, 191, 36, 0.28);
  color: #fef08a;
  border-right: 2px solid #fbbf24;
}

.seg-safe {
  flex: 1;
  background: rgba(52, 211, 153, 0.2);
  color: #6ee7b7;
}

/* 底部操作 */
.pg-action-footer {
  position: sticky;
  bottom: 0;
  z-index: 5;
}

.pg-footer-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}

.pg-save-btn {
  padding: 0 24px;
}
</style>
