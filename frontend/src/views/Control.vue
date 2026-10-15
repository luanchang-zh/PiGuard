<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useDeviceStore } from '@/stores/device'
import { createCommand, getCommand } from '@/api/command'
import { commandStatusTag } from '@/utils/format'
import type { CommandType, CommandView } from '@/types'

const device = useDeviceStore()
const submitting = ref(false)
const commandId = ref('')
const commandStatus = ref<CommandView | null>(null)
const statusLoading = ref(false)

// 蜂鸣器
const buzzer = reactive({ duration_ms: 500 })
// LED
const indicator = reactive({ duration_ms: 500, color: 'green' as 'green' | 'yellow' | 'red' })
// 拍照
const snapshot = reactive({ quality: 80 })
// 场景
const scenario = reactive({
  scenario: 'obstacle_approach' as string,
  speed: 1 as number,
})

const scenarioOptions = [
  { value: 'normal_drive', label: '正常行驶' },
  { value: 'obstacle_approach', label: '障碍物接近' },
  { value: 'lane_departure', label: '车道偏离' },
  { value: 'sharp_turn', label: '急转弯' },
  { value: 'high_temperature', label: '车内高温' },
  { value: 'sensor_failure', label: '传感器故障' },
]

async function send(type: CommandType, params: Record<string, unknown>) {
  submitting.value = true
  try {
    const res = await createCommand(device.currentDeviceId, { type, params })
    commandId.value = res.command_id
    ElMessage.success(`命令已下发：${res.command_id}`)
    await pollStatus(res.command_id)
  } catch {
    /* 拦截器已提示 */
  } finally {
    submitting.value = false
  }
}

async function pollStatus(id: string) {
  statusLoading.value = true
  // 最多轮询 12 秒，命令后端超时为 10 秒
  for (let i = 0; i < 24; i++) {
    try {
      const cmd = await getCommand(id)
      commandStatus.value = cmd
      if (cmd.status === 'success' || cmd.status === 'failed' || cmd.status === 'timeout') break
    } catch {
      break
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  statusLoading.value = false
}
</script>

<template>
  <div class="control-page">
    <el-row :gutter="16">
      <!-- 蜂鸣器测试 -->
      <el-col :span="8">
        <el-card shadow="never">
          <template #header>蜂鸣器测试</template>
          <el-form label-width="90px">
            <el-form-item label="持续时长">
              <el-input-number v-model="buzzer.duration_ms" :min="100" :max="5000" :step="100" />
              <span class="unit">ms</span>
            </el-form-item>
            <el-button type="primary" :loading="submitting" @click="send('buzzer.test', { duration_ms: buzzer.duration_ms })">
              触发蜂鸣
            </el-button>
          </el-form>
        </el-card>
      </el-col>

      <!-- LED 指示灯测试 -->
      <el-col :span="8">
        <el-card shadow="never">
          <template #header>LED 指示灯测试</template>
          <el-form label-width="90px">
            <el-form-item label="颜色">
              <el-radio-group v-model="indicator.color">
                <el-radio value="green">绿</el-radio>
                <el-radio value="yellow">黄</el-radio>
                <el-radio value="red">红</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="持续时长">
              <el-input-number v-model="indicator.duration_ms" :min="100" :max="5000" :step="100" />
              <span class="unit">ms</span>
            </el-form-item>
            <el-button type="primary" :loading="submitting" @click="send('indicator.test', { ...indicator })">
              点亮 LED
            </el-button>
          </el-form>
        </el-card>
      </el-col>

      <!-- 摄像头拍照 -->
      <el-col :span="8">
        <el-card shadow="never">
          <template #header>摄像头拍照</template>
          <el-form label-width="90px">
            <el-form-item label="画质">
              <el-input-number v-model="snapshot.quality" :min="1" :max="100" />
              <span class="unit">%</span>
            </el-form-item>
            <el-button type="primary" :loading="submitting" @click="send('camera.snapshot', { quality: snapshot.quality })">
              拍照
            </el-button>
          </el-form>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>模拟场景</template>
      <el-form inline label-width="90px">
        <el-form-item label="场景">
          <el-select v-model="scenario.scenario" style="width: 200px">
            <el-option v-for="o in scenarioOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="速度倍率">
          <el-select v-model="scenario.speed" style="width: 120px">
            <el-option :value="0.5" label="0.5x" />
            <el-option :value="1" label="1x" />
            <el-option :value="2" label="2x" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="send('scenario.start', { ...scenario })">
            启动场景
          </el-button>
          <el-button :loading="submitting" @click="send('scenario.stop', {})">停止场景</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 命令状态 -->
    <el-card shadow="never" style="margin-top: 16px" v-if="commandStatus">
      <template #header>命令执行状态</template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="命令 ID">{{ commandStatus.command_id }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ commandStatus.type }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="commandStatusTag(commandStatus.status)" effect="dark">
            {{ commandStatus.status }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="下发时间">
          {{ new Date(commandStatus.issued_at).toLocaleString() }}
        </el-descriptions-item>
        <el-descriptions-item label="执行时间" v-if="commandStatus.executed_at">
          {{ new Date(commandStatus.executed_at).toLocaleString() }}
        </el-descriptions-item>
        <el-descriptions-item label="结果" v-if="commandStatus.result">
          <span class="payload">{{ JSON.stringify(commandStatus.result) }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="错误" v-if="commandStatus.error" :span="2">
          <el-tag type="danger">{{ commandStatus.error.code }}: {{ commandStatus.error.message }}</el-tag>
        </el-descriptions-item>
      </el-descriptions>
    </el-card>
  </div>
</template>

<style scoped>
.unit {
  margin-left: 8px;
  color: var(--pg-text-muted);
  font-size: 13px;
}

.payload {
  font-family: monospace;
  font-size: 12px;
  color: var(--pg-text-muted);
}
</style>
