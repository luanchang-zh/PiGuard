<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useDeviceStore } from '@/stores/device'
import { getConfig, updateConfig } from '@/api/config'
import type { ConfigView, RuleSet } from '@/types'

const device = useDeviceStore()
const loading = ref(false)
const saving = ref(false)
const config = ref<ConfigView | null>(null)
let pollTimer: ReturnType<typeof setInterval> | undefined

// 表单使用深拷贝，避免直接修改返回对象
const form = reactive<RuleSet>({
  obstacle: { warning_distance_m: 15, danger_distance_m: 8 },
  lane_departure: { offset_threshold: 0.35, duration_ms: 1000 },
  sharp_turn: { yaw_threshold_deg_s: 30, min_speed_kmh: 20, duration_ms: 300 },
  temperature: { trigger_c: 35, recover_c: 33, duration_ms: 5000 },
})

async function load() {
  loading.value = true
  try {
    const c = await getConfig(device.currentDeviceId)
    config.value = c
    Object.assign(form, c.rules)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!config.value) return
  saving.value = true
  try {
    // 后端支持 rules 补丁：只传要修改的分组即可，但这里传全量更直观。
    // expected_version 必须等于当前 desired_version，避免并发覆盖。
    const res = await updateConfig(device.currentDeviceId, {
      expected_version: config.value.desired_version,
      rules: form,
    })
    ElMessage.success(`配置已受理，版本 ${res.config_version}，等待设备确认`)
    await load()
  } catch {
    /* 拦截器已提示；版本冲突时刷新 */
    await load()
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  load()
  pollTimer = setInterval(async () => {
    if (config.value?.status !== 'pending' || saving.value) return
    try {
      config.value = await getConfig(device.currentDeviceId)
    } catch {
      /* 请求拦截器显示错误；不覆盖用户正在编辑的表单。 */
    }
  }, 2000)
})
onUnmounted(() => clearInterval(pollTimer))
</script>

<template>
  <div class="config-page" v-loading="loading">
    <el-card shadow="never" v-if="config">
      <template #header>
        <div class="header">
          <span>参数配置</span>
          <div class="versions">
            <el-tag type="info" effect="plain">期望版本 v{{ config.desired_version }}</el-tag>
            <el-tag :type="config.reported_version === config.desired_version ? 'success' : 'warning'" effect="plain">
              设备版本 v{{ config.reported_version }}
            </el-tag>
            <el-tag :type="config.status === 'success' ? 'success' : config.status === 'failed' ? 'danger' : 'warning'" effect="plain">
              {{ { pending: '同步中', success: '已同步', failed: '同步失败' }[config.status] }}
            </el-tag>
          </div>
        </div>
      </template>

      <el-form label-width="160px" class="config-form">
        <!-- 障碍物预警 -->
        <fieldset class="group">
          <legend>障碍物预警</legend>
          <el-form-item label="警告距离 (m)">
            <el-input-number v-model="form.obstacle.warning_distance_m" :min="0.1" :step="0.5" :precision="1" />
          </el-form-item>
          <el-form-item label="危险距离 (m)">
            <el-input-number v-model="form.obstacle.danger_distance_m" :min="0.1" :step="0.5" :precision="1" />
            <span class="hint">须小于警告距离</span>
          </el-form-item>
        </fieldset>

        <!-- 车道偏离 -->
        <fieldset class="group">
          <legend>车道偏离预警</legend>
          <el-form-item label="偏移阈值">
            <el-input-number v-model="form.lane_departure.offset_threshold" :min="0.01" :max="1" :step="0.05" :precision="2" />
          </el-form-item>
          <el-form-item label="持续时间 (ms)">
            <el-input-number v-model="form.lane_departure.duration_ms" :min="1" :step="100" />
          </el-form-item>
        </fieldset>

        <!-- 急转弯 -->
        <fieldset class="group">
          <legend>急转弯预警</legend>
          <el-form-item label="角速度阈值 (°/s)">
            <el-input-number v-model="form.sharp_turn.yaw_threshold_deg_s" :min="0.1" :step="1" :precision="1" />
          </el-form-item>
          <el-form-item label="最低车速 (km/h)">
            <el-input-number v-model="form.sharp_turn.min_speed_kmh" :min="0" :step="1" :precision="1" />
          </el-form-item>
          <el-form-item label="持续时间 (ms)">
            <el-input-number v-model="form.sharp_turn.duration_ms" :min="1" :step="100" />
          </el-form-item>
        </fieldset>

        <!-- 车内高温 -->
        <fieldset class="group">
          <legend>车内高温预警</legend>
          <el-form-item label="触发温度 (°C)">
            <el-input-number v-model="form.temperature.trigger_c" :step="0.5" :precision="1" />
          </el-form-item>
          <el-form-item label="恢复温度 (°C)">
            <el-input-number v-model="form.temperature.recover_c" :step="0.5" :precision="1" />
            <span class="hint">须小于触发温度</span>
          </el-form-item>
          <el-form-item label="持续时间 (ms)">
            <el-input-number v-model="form.temperature.duration_ms" :min="1" :step="100" />
          </el-form-item>
        </fieldset>

        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存并下发</el-button>
          <el-button @click="load">重新加载</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<style scoped>
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.versions {
  display: flex;
  gap: 8px;
}

.group {
  border: 1px solid var(--pg-border);
  border-radius: 10px;
  padding: 16px 20px 4px;
  margin-bottom: 20px;
}

.group legend {
  padding: 0 10px;
  color: var(--pg-primary-light);
  font-weight: 600;
}

.hint {
  margin-left: 12px;
  color: var(--pg-text-faint);
  font-size: 12px;
}
</style>
