<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useDeviceStore } from '@/stores/device'
import { createCommand, getCommand } from '@/api/command'
import PgCard from '@/components/ui/PgCard.vue'
import PgBadge from '@/components/ui/PgBadge.vue'
import PgIcon from '@/components/ui/PgIcon.vue'
import PgEmpty from '@/components/ui/PgEmpty.vue'
import {
  formatTime,
  describeParams,
} from '@/utils/format'
import {
  SCENARIOS,
  COMMAND_META,
  COMMAND_STATUS_META,
} from '@/utils/meta'
import type { CommandType, CommandView } from '@/types'

const device = useDeviceStore()
const submitting = ref(false)
const activeCommand = ref<CommandView | null>(null)
const commandHistory = ref<CommandView[]>([])
const activeScenario = ref<string | null>(null)

// 1. 蜂鸣器参数
const buzzer = reactive({
  duration_ms: 600,
})

// 2. 摄像头拍照参数
const snapshot = reactive({
  quality: 85,
})

// 3. 场景参数
const selectedScenario = ref('obstacle_approach')
const scenarioSpeed = ref<number>(1)

async function send(type: CommandType, params: Record<string, unknown>) {
  submitting.value = true
  try {
    const res = await createCommand(device.currentDeviceId, { type, params })
    const initialView: CommandView = {
      command_id: res.command_id,
      device_id: device.currentDeviceId,
      type,
      params,
      status: res.status,
      issued_at: new Date().toISOString(),
      expires_at: new Date(Date.now() + 10000).toISOString(),
    }
    activeCommand.value = initialView
    commandHistory.value.unshift(initialView)
    if (commandHistory.value.length > 30) commandHistory.value.pop()

    if (type === 'scenario.start') {
      activeScenario.value = (params.scenario as string) || null
    } else if (type === 'scenario.stop') {
      activeScenario.value = null
    }

    ElMessage.success(`指令已签发：${COMMAND_META[type]?.label || type} (ID: ${res.command_id})`)
    await pollStatus(res.command_id)
  } finally {
    submitting.value = false
  }
}

async function pollStatus(id: string) {
  for (let i = 0; i < 24; i++) {
    try {
      const cmd = await getCommand(id)
      activeCommand.value = cmd
      const idx = commandHistory.value.findIndex((c) => c.command_id === id)
      if (idx >= 0) commandHistory.value[idx] = cmd

      if (cmd.status === 'success' || cmd.status === 'failed' || cmd.status === 'timeout') {
        if (cmd.status === 'success') {
          ElMessage.success(`指令已完成：${COMMAND_META[cmd.type]?.label || cmd.type}`)
        } else if (cmd.status === 'failed') {
          ElMessage.error(`指令执行失败：${cmd.error?.message || '未知错误'}`)
        }
        break
      }
    } catch {
      break
    }
    await new Promise((r) => setTimeout(r, 500))
  }
}

function setBuzzerPreset(ms: number) {
  buzzer.duration_ms = ms
}
</script>

<template>
  <div class="pg-page pg-control-page pg-stagger">
    <!-- 顶部：蜂鸣器与摄像头控制台 -->
    <div class="pg-actuators-grid">
      <!-- 1. 蜂鸣器执行器 -->
      <PgCard
        title="蜂鸣器声学预警执行器"
        subtitle="GPIO 压电蜂鸣器报警脉冲测试"
        icon="volume"
        accent="var(--pg-warn)"
        class="pg-actuator-card"
      >
        <div class="pg-actuator-content">
          <!-- 动态声波动画 -->
          <div class="pg-sound-wave-box">
            <div class="pg-wave-bar" :style="{ height: '30%' }" />
            <div class="pg-wave-bar" :style="{ height: '60%' }" />
            <div class="pg-wave-bar" :style="{ height: '95%' }" />
            <div class="pg-wave-bar" :style="{ height: '60%' }" />
            <div class="pg-wave-bar" :style="{ height: '30%' }" />
            <span class="pg-wave-label mono">{{ buzzer.duration_ms }} ms</span>
          </div>

          <div class="pg-control-row">
            <span class="pg-control-label">发声时长:</span>
            <el-slider
              v-model="buzzer.duration_ms"
              :min="100"
              :max="3000"
              :step="50"
              class="pg-flex-1"
            />
          </div>

          <div class="pg-preset-pills">
            <button
              v-for="ms in [200, 500, 1000, 2000]"
              :key="ms"
              class="pg-preset-btn"
              :class="{ active: buzzer.duration_ms === ms }"
              @click="setBuzzerPreset(ms)"
            >
              {{ ms }}ms
            </button>
          </div>

          <el-button
            type="primary"
            class="pg-act-btn"
            :loading="submitting"
            @click="send('buzzer.test', { duration_ms: buzzer.duration_ms })"
          >
            <PgIcon name="volume" :size="16" />
            <span>触发蜂鸣报警</span>
          </el-button>
        </div>
      </PgCard>

      <!-- 2. 摄像头抓拍执行器 -->
      <PgCard
        title="摄像头抓拍"
        subtitle="即时触发单帧证据捕获与 HTTP 上传"
        icon="camera"
        accent="var(--pg-c-lane)"
        class="pg-actuator-card"
      >
        <div class="pg-actuator-content">
          <div class="pg-cam-act-box">
            <div class="pg-cam-icon-halo">
              <PgIcon name="camera" :size="32" />
            </div>
            <span class="pg-cam-quality-tag mono">JPEG 画质: {{ snapshot.quality }}%</span>
          </div>

          <div class="pg-control-row">
            <span class="pg-control-label">压缩画质:</span>
            <el-slider
              v-model="snapshot.quality"
              :min="20"
              :max="100"
              :step="5"
              class="pg-flex-1"
            />
          </div>

          <div class="pg-preset-pills">
            <button
              v-for="q in [50, 75, 85, 95]"
              :key="q"
              class="pg-preset-btn"
              :class="{ active: snapshot.quality === q }"
              @click="snapshot.quality = q"
            >
              {{ q }}%
            </button>
          </div>

          <el-button
            type="primary"
            class="pg-act-btn"
            :loading="submitting"
            @click="send('camera.snapshot', { quality: snapshot.quality })"
          >
            <PgIcon name="camera" :size="16" />
            <span>立即抓拍证据</span>
          </el-button>
        </div>
      </PgCard>
    </div>

    <!-- 中部：ADAS 驾驶模拟演练中心 -->
    <PgCard
      title="ADAS 典型驾驶工况演练中心"
      subtitle="边缘端仿真器注入不同驾驶道路工况并校验多传感器规则链预警反应"
      icon="play"
      accent="var(--pg-accent)"
    >
      <template #extra>
        <div class="pg-scenario-top-actions">
          <div class="pg-speed-selector">
            <span class="label">演练倍速:</span>
            <el-segmented
              v-model="scenarioSpeed"
              :options="[
                { label: '0.5x 慢速', value: 0.5 },
                { label: '1.0x 常规', value: 1.0 },
                { label: '2.0x 快速', value: 2.0 },
              ]"
              size="small"
            />
          </div>

          <el-button
            type="danger"
            plain
            size="small"
            :loading="submitting"
            @click="send('scenario.stop', {})"
          >
            <PgIcon name="stop" :size="14" />
            <span>强制终止当前场景</span>
          </el-button>
        </div>
      </template>

      <div class="pg-scenarios-grid">
        <div
          v-for="s in SCENARIOS"
          :key="s.value"
          class="pg-scenario-card"
          :class="{
            selected: selectedScenario === s.value,
            running: activeScenario === s.value,
          }"
          @click="selectedScenario = s.value"
        >
          <div class="pg-sc-top">
            <div class="pg-sc-icon" :class="`tone-${s.tone}`">
              <PgIcon :name="s.icon" :size="20" />
            </div>
            <div class="pg-sc-badge">
              <PgBadge
                v-if="activeScenario === s.value"
                tone="ok"
                size="sm"
                :pulse="true"
              >
                正在运行中
              </PgBadge>
              <PgBadge v-else :tone="s.tone" size="sm">
                {{ s.label }}
              </PgBadge>
            </div>
          </div>

          <h3 class="pg-sc-title">{{ s.label }}</h3>
          <p class="pg-sc-desc">{{ s.desc }}</p>

          <div class="pg-sc-actions">
            <el-button
              type="primary"
              size="small"
              class="w-full"
              :loading="submitting && selectedScenario === s.value"
              @click.stop="
                selectedScenario = s.value;
                send('scenario.start', { scenario: s.value, speed: scenarioSpeed });
              "
            >
              <PgIcon name="play" :size="13" />
              <span>启动演练</span>
            </el-button>
          </div>
        </div>
      </div>
    </PgCard>

    <!-- 底部：命令下发状态与执行日志流水 -->
    <PgCard
      title="指令执行跟踪与回执流水"
      subtitle="MQTT 下发与 HTTP/QoS 1 Ack 双向握手遥测跟踪 (超时时间 10 秒)"
      icon="activity"
    >
      <div v-if="commandHistory.length > 0" class="pg-cmd-timeline">
        <div
          v-for="cmd in commandHistory"
          :key="cmd.command_id"
          class="pg-cmd-row"
          :class="`status-${cmd.status}`"
        >
          <div class="pg-cmd-status-icon">
            <PgIcon
              :name="
                cmd.status === 'success'
                  ? 'checkCircle'
                  : cmd.status === 'failed'
                  ? 'xCircle'
                  : cmd.status === 'timeout'
                  ? 'clock'
                  : 'loader'
              "
              :size="18"
              :spin="cmd.status === 'pending' || cmd.status === 'sent'"
            />
          </div>

          <div class="pg-cmd-main">
            <div class="pg-cmd-head">
              <span class="pg-cmd-type">{{ COMMAND_META[cmd.type]?.label || cmd.type }}</span>
              <PgBadge :tone="COMMAND_STATUS_META[cmd.status]?.tone || 'unknown'" size="sm">
                {{ COMMAND_STATUS_META[cmd.status]?.label || cmd.status }}
              </PgBadge>
              <span class="pg-cmd-id mono">ID: {{ cmd.command_id }}</span>
            </div>

            <div class="pg-cmd-meta-line">
              <span class="label">参数:</span>
              <span class="val mono">{{ describeParams(cmd.params) || '无' }}</span>
              <span class="pg-sep">·</span>
              <span class="label">签发时间:</span>
              <span class="val mono">{{ formatTime(cmd.issued_at) }}</span>
              <template v-if="cmd.executed_at">
                <span class="pg-sep">·</span>
                <span class="label">设备执行完成:</span>
                <span class="val mono">{{ formatTime(cmd.executed_at) }}</span>
              </template>
            </div>

            <!-- 执行结果或错误提示 -->
            <div v-if="cmd.result" class="pg-cmd-payload mono">
              <span class="payload-label">回执数据:</span>
              <span>{{ JSON.stringify(cmd.result) }}</span>
            </div>
            <div v-if="cmd.error" class="pg-cmd-payload error mono">
              <span class="payload-label">错误原因:</span>
              <span>[{{ cmd.error.code }}] {{ cmd.error.message }}</span>
            </div>
          </div>
        </div>
      </div>
      <PgEmpty
        v-else
        icon="sliders"
        title="暂无指令下发记录"
        description="在上方控制台中点击任意执行器或场景启动测试指令"
        :compact="true"
      />
    </PgCard>
  </div>
</template>

<style scoped>
.pg-control-page {
  display: flex;
  flex-direction: column;
  gap: var(--pg-gap);
}

/* 蜂鸣器与摄像头双列网格 */
.pg-actuators-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--pg-gap);
}

@media (max-width: 992px) {
  .pg-actuators-grid {
    grid-template-columns: 1fr;
  }
}

.pg-actuator-card {
  min-height: 280px;
}

.pg-actuator-content {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.pg-control-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.pg-control-label {
  font-size: 12.5px;
  color: var(--pg-text-muted);
  width: 65px;
  flex-shrink: 0;
}

.pg-flex-1 {
  flex: 1;
}

.pg-preset-pills {
  display: flex;
  gap: 6px;
}

.pg-preset-btn {
  flex: 1;
  padding: 5px 8px;
  border-radius: var(--pg-radius-xs);
  background: var(--pg-surface-2);
  border: 1px solid var(--pg-border);
  color: var(--pg-text-muted);
  font-size: 11px;
  font-family: var(--pg-font-mono);
  cursor: pointer;
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.pg-preset-btn:hover {
  background: var(--pg-surface-hover);
  color: var(--pg-text);
  border-color: var(--pg-border-strong);
}

.pg-preset-btn.active {
  background: rgba(34, 211, 238, 0.14);
  color: var(--pg-primary);
  border-color: rgba(34, 211, 238, 0.35);
  font-weight: 650;
}

.pg-act-btn {
  width: 100%;
  margin-top: 4px;
  height: 38px;
}

/* 蜂鸣器动效模拟 */
.pg-sound-wave-box {
  height: 60px;
  background: rgba(10, 16, 28, 0.5);
  border-radius: var(--pg-radius-sm);
  border: 1px solid var(--pg-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  position: relative;
}

.pg-wave-bar {
  width: 4px;
  background: var(--pg-warn);
  border-radius: 999px;
  transition: height 0.2s;
  box-shadow: 0 0 8px rgba(251, 191, 36, 0.4);
}

.pg-wave-label {
  position: absolute;
  right: 10px;
  bottom: 6px;
  font-size: 11px;
  color: var(--pg-text-faint);
}

/* 摄像头快照展示 */
.pg-cam-act-box {
  height: 60px;
  background: rgba(10, 16, 28, 0.5);
  border-radius: var(--pg-radius-sm);
  border: 1px solid var(--pg-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
}

.pg-cam-icon-halo {
  color: var(--pg-primary);
}

.pg-cam-quality-tag {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--pg-text-2);
}

/* 场景中心 */
.pg-scenario-top-actions {
  display: flex;
  align-items: center;
  gap: 14px;
}

.pg-speed-selector {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pg-speed-selector .label {
  font-size: 12px;
  color: var(--pg-text-muted);
}

.pg-scenarios-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 12px;
}

@media (max-width: 1400px) {
  .pg-scenarios-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 768px) {
  .pg-scenarios-grid {
    grid-template-columns: 1fr;
  }
}

.pg-scenario-card {
  padding: 16px;
  border-radius: var(--pg-radius-sm);
  background: var(--pg-surface);
  border: 1px solid var(--pg-border);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  transition: all var(--pg-dur) var(--pg-ease);
  position: relative;
}

.pg-scenario-card:hover {
  background: var(--pg-surface-hover);
  border-color: var(--pg-border-strong);
  transform: translateY(-2px);
}

.pg-scenario-card.selected {
  border-color: var(--pg-primary);
  background: rgba(34, 211, 238, 0.06);
}

.pg-scenario-card.running {
  border-color: var(--pg-ok);
  background: rgba(52, 211, 153, 0.08);
}

.pg-sc-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.pg-sc-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--tone-soft);
  color: var(--tone);
  border: 1px solid color-mix(in srgb, var(--tone) 30%, transparent);
  display: grid;
  place-items: center;
}

.pg-sc-title {
  font-size: 14px;
  font-weight: 650;
  color: #fff;
  margin-bottom: 4px;
}

.pg-sc-desc {
  font-size: 11.5px;
  color: var(--pg-text-muted);
  line-height: 1.4;
  margin-bottom: 14px;
  min-height: 32px;
}

.pg-sc-actions {
  margin-top: auto;
}

.w-full {
  width: 100%;
}

/* 指令流水线 */
.pg-cmd-timeline {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 360px;
  overflow-y: auto;
}

.pg-cmd-row {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 12px 16px;
  border-radius: var(--pg-radius-sm);
  background: rgba(15, 23, 38, 0.5);
  border: 1px solid var(--pg-border);
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.pg-cmd-row:hover {
  background: var(--pg-surface-hover);
  border-color: var(--pg-border-strong);
}

.pg-cmd-status-icon {
  width: 32px;
  height: 32px;
  border-radius: 9px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
  margin-top: 2px;
}

.status-success .pg-cmd-status-icon {
  background: rgba(52, 211, 153, 0.15);
  color: var(--pg-ok);
}

.status-failed .pg-cmd-status-icon {
  background: rgba(251, 81, 116, 0.15);
  color: var(--pg-danger);
}

.status-timeout .pg-cmd-status-icon {
  background: rgba(251, 191, 36, 0.15);
  color: var(--pg-warn);
}

.status-pending .pg-cmd-status-icon,
.status-sent .pg-cmd-status-icon {
  background: rgba(34, 211, 238, 0.15);
  color: var(--pg-primary);
}

.pg-cmd-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.pg-cmd-head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pg-cmd-type {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--pg-text);
}

.pg-cmd-id {
  font-size: 11px;
  color: var(--pg-text-faint);
  margin-left: auto;
}

.pg-cmd-meta-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12px;
  color: var(--pg-text-muted);
}

.pg-cmd-payload {
  margin-top: 4px;
  padding: 6px 10px;
  border-radius: var(--pg-radius-xs);
  background: rgba(3, 7, 18, 0.6);
  border: 1px solid var(--pg-border);
  font-size: 11.5px;
  color: var(--pg-primary);
}

.pg-cmd-payload.error {
  color: var(--pg-danger);
  border-color: rgba(251, 81, 116, 0.3);
}

.payload-label {
  color: var(--pg-text-faint);
  margin-right: 6px;
}
</style>
