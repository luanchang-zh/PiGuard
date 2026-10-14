<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useDeviceStore } from '@/stores/device'
import { listEvents } from '@/api/event'
import { snapshotContentUrl } from '@/api/frame'
import {
  eventTypeLabel,
  eventActionLabel,
  eventLevelTag,
  formatTime,
} from '@/utils/format'
import type { EventItem, EventType, EventLevel } from '@/types'

const device = useDeviceStore()
const loading = ref(false)
const items = ref<EventItem[]>([])

const filters = reactive({
  type: '' as EventType | '',
  level: '' as EventLevel | '',
  limit: 100,
})

const typeOptions: { value: EventType; label: string }[] = [
  { value: 'obstacle_warning', label: '障碍物预警' },
  { value: 'lane_departure', label: '车道偏离' },
  { value: 'sharp_turn', label: '急转弯' },
  { value: 'high_temperature', label: '车内高温' },
  { value: 'sensor_failure', label: '传感器故障' },
]

const levelOptions: { value: EventLevel; label: string }[] = [
  { value: 'normal', label: '正常' },
  { value: 'warning', label: '警告' },
  { value: 'danger', label: '危险' },
]

async function load() {
  loading.value = true
  try {
    const query: Record<string, unknown> = { limit: filters.limit }
    if (filters.type) query.type = filters.type
    if (filters.level) query.level = filters.level
    const res = await listEvents(device.currentDeviceId, query)
    items.value = res.items
  } finally {
    loading.value = false
  }
}

function imgUrl(row: EventItem) {
  if (row.snapshot_available && row.snapshot_id) return snapshotContentUrl(row.snapshot_id)
  return ''
}

const previewVisible = ref(false)
const previewUrl = ref('')
function preview(row: EventItem) {
  const url = imgUrl(row)
  if (!url) return
  previewUrl.value = url
  previewVisible.value = true
}

onMounted(load)
</script>

<template>
  <div class="events-page">
    <el-card shadow="never" class="filter-card">
      <el-form inline>
        <el-form-item label="告警类型">
          <el-select v-model="filters.type" placeholder="全部" clearable style="width: 160px">
            <el-option v-for="o in typeOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="告警级别">
          <el-select v-model="filters.level" placeholder="全部" clearable style="width: 120px">
            <el-option v-for="o in levelOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="数量">
          <el-input-number v-model="filters.limit" :min="1" :max="1000" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="load">查询</el-button>
          <el-button @click="filters.type = ''; filters.level = ''; load()">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <el-table :data="items" v-loading="loading" stripe>
        <el-table-column prop="timestamp" label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.timestamp) }}</template>
        </el-table-column>
        <el-table-column label="类型" width="140">
          <template #default="{ row }">{{ eventTypeLabel(row.type) }}</template>
        </el-table-column>
        <el-table-column label="动作" width="100">
          <template #default="{ row }">{{ eventActionLabel(row.action) }}</template>
        </el-table-column>
        <el-table-column label="级别" width="100">
          <template #default="{ row }">
            <el-tag :type="eventLevelTag(row.level)" size="small">{{ row.level }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="event_id" label="事件 ID" min-width="180" show-overflow-tooltip />
        <el-table-column label="详情" min-width="200">
          <template #default="{ row }">
            <span class="payload">{{ row.data ? JSON.stringify(row.data) : '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="图片" width="100" align="center">
          <template #default="{ row }">
            <el-button
              v-if="row.snapshot_available"
              link
              type="primary"
              @click="preview(row)"
            >查看</el-button>
            <span v-else style="color: var(--pg-text-faint)">-</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="previewVisible" title="告警图片" width="640px">
      <img :src="previewUrl" style="width: 100%; border-radius: 8px" />
    </el-dialog>
  </div>
</template>

<style scoped>
.filter-card :deep(.el-form-item) {
  margin-bottom: 0;
}

.payload {
  font-family: monospace;
  font-size: 12px;
  color: var(--pg-text-muted);
  word-break: break-all;
}
</style>
