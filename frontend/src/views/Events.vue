<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useDeviceStore } from '@/stores/device'
import { listEvents } from '@/api/event'
import { snapshotContentUrl } from '@/api/frame'
import PgCard from '@/components/ui/PgCard.vue'
import PgBadge from '@/components/ui/PgBadge.vue'
import PgIcon from '@/components/ui/PgIcon.vue'
import PgEmpty from '@/components/ui/PgEmpty.vue'
import {
  formatTime,
  formatRelative,
  describeEventData,
  toCsv,
  downloadText,
} from '@/utils/format'
import {
  EVENT_META,
  EVENT_TYPES,
  ACTION_META,
  LEVEL_META,
} from '@/utils/meta'
import type { EventItem, EventType, EventLevel } from '@/types'
import { ElMessage } from 'element-plus'

const device = useDeviceStore()
const loading = ref(false)
const items = ref<EventItem[]>([])
const searchKeyword = ref('')

const filters = reactive({
  type: '' as EventType | '',
  level: '' as EventLevel | '',
  limit: 100,
})

const typeOptions = EVENT_TYPES.map((t) => ({
  value: t,
  label: EVENT_META[t].label,
  icon: EVENT_META[t].icon,
}))

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
    items.value = res.items || []
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  filters.type = ''
  filters.level = ''
  searchKeyword.value = ''
  load()
}

// 统计数据
const stats = computed(() => {
  let danger = 0
  let warning = 0
  let withSnapshot = 0
  for (const it of items.value) {
    if (it.level === 'danger') danger++
    else if (it.level === 'warning') warning++
    if (it.snapshot_available && it.snapshot_id) withSnapshot++
  }
  return {
    total: items.value.length,
    danger,
    warning,
    withSnapshot,
  }
})

// 过滤后的事件列表
const filteredItems = computed(() => {
  if (!searchKeyword.value.trim()) return items.value
  const kw = searchKeyword.value.trim().toLowerCase()
  return items.value.filter((it) => {
    return (
      it.event_id.toLowerCase().includes(kw) ||
      it.type.toLowerCase().includes(kw) ||
      (EVENT_META[it.type]?.label && EVENT_META[it.type].label.includes(kw)) ||
      (it.data && JSON.stringify(it.data).toLowerCase().includes(kw))
    )
  })
})

// 图片弹窗
const previewVisible = ref(false)
const activeEvent = ref<EventItem | null>(null)

function openPreview(row: any) {
  activeEvent.value = row as EventItem
  previewVisible.value = true
}

const previewUrl = computed(() => {
  if (!activeEvent.value?.snapshot_id) return ''
  return snapshotContentUrl(activeEvent.value.snapshot_id)
})

function exportCsv() {
  if (items.value.length === 0) {
    ElMessage.warning('暂无记录可导出')
    return
  }
  const headers = ['事件ID', '时间', '类型', '动作', '级别', '快照ID', '详细数据']
  const rows = items.value.map((e) => [
    e.event_id,
    e.timestamp,
    EVENT_META[e.type]?.label || e.type,
    ACTION_META[e.action]?.label || e.action,
    LEVEL_META[e.level]?.label || e.level,
    e.snapshot_id || '',
    e.data ? JSON.stringify(e.data) : '',
  ])
  const csvContent = toCsv([headers, ...rows])
  downloadText(`PiGuard-Events-${device.currentDeviceId}-${Date.now()}.csv`, csvContent)
  ElMessage.success('已导出告警事件 CSV 文件')
}

onMounted(load)
</script>

<template>
  <div class="pg-page pg-events-page pg-stagger">
    <!-- 顶部概览统计条 -->
    <div class="pg-stats-row">
      <div class="pg-stat-chip">
        <div class="pg-stat-icon-box tone-info">
          <PgIcon name="bell" :size="18" />
        </div>
        <div class="pg-stat-content">
          <span class="pg-stat-num mono">{{ stats.total }}</span>
          <span class="pg-stat-title">当前加载告警</span>
        </div>
      </div>

      <div class="pg-stat-chip">
        <div class="pg-stat-icon-box tone-danger">
          <PgIcon name="octagon" :size="18" />
        </div>
        <div class="pg-stat-content">
          <span class="pg-stat-num mono text-danger">{{ stats.danger }}</span>
          <span class="pg-stat-title">危险级别 (Danger)</span>
        </div>
      </div>

      <div class="pg-stat-chip">
        <div class="pg-stat-icon-box tone-warn">
          <PgIcon name="alert" :size="18" />
        </div>
        <div class="pg-stat-content">
          <span class="pg-stat-num mono text-warn">{{ stats.warning }}</span>
          <span class="pg-stat-title">预警级别 (Warning)</span>
        </div>
      </div>

      <div class="pg-stat-chip">
        <div class="pg-stat-icon-box tone-primary">
          <PgIcon name="camera" :size="18" />
        </div>
        <div class="pg-stat-content">
          <span class="pg-stat-num mono text-primary">{{ stats.withSnapshot }}</span>
          <span class="pg-stat-title">关联快照截图</span>
        </div>
      </div>
    </div>

    <!-- 筛选控制栏 -->
    <PgCard padding="sm" class="pg-filter-panel">
      <div class="pg-filter-toolbar">
        <div class="pg-filter-group">
          <!-- 告警类型 -->
          <el-select
            v-model="filters.type"
            placeholder="全部告警类型"
            clearable
            style="width: 170px"
            @change="load"
          >
            <el-option
              v-for="o in typeOptions"
              :key="o.value"
              :label="o.label"
              :value="o.value"
            >
              <div class="pg-opt-row">
                <PgIcon :name="o.icon" :size="14" />
                <span>{{ o.label }}</span>
              </div>
            </el-option>
          </el-select>

          <!-- 告警级别 -->
          <el-select
            v-model="filters.level"
            placeholder="全部级别"
            clearable
            style="width: 130px"
            @change="load"
          >
            <el-option
              v-for="o in levelOptions"
              :key="o.value"
              :label="o.label"
              :value="o.value"
            />
          </el-select>

          <!-- 数量限制 -->
          <el-select v-model="filters.limit" style="width: 110px" @change="load">
            <el-option :value="50" label="50 条" />
            <el-option :value="100" label="100 条" />
            <el-option :value="200" label="200 条" />
            <el-option :value="500" label="500 条" />
          </el-select>

          <!-- 关键词搜索 -->
          <el-input
            v-model="searchKeyword"
            placeholder="搜索事件ID / 参数内容..."
            clearable
            style="width: 220px"
          >
            <template #prefix>
              <PgIcon name="search" :size="14" class="faint" />
            </template>
          </el-input>
        </div>

        <div class="pg-filter-actions">
          <el-button @click="resetFilters">重置</el-button>
          <el-button type="primary" :loading="loading" @click="load">
            <PgIcon name="refresh" :size="14" />
            <span>查询</span>
          </el-button>
          <el-button @click="exportCsv">
            <PgIcon name="download" :size="14" />
            <span>导出 CSV</span>
          </el-button>
        </div>
      </div>
    </PgCard>

    <!-- 事件列表卡片 -->
    <PgCard padding="none" class="pg-table-card">
      <el-table
        :data="filteredItems"
        v-loading="loading"
        stripe
        row-key="event_id"
        class="pg-events-table"
      >
        <template #empty>
          <PgEmpty
            icon="shieldCheck"
            title="未检索到符合条件的告警事件"
            description="可尝试重置查询条件或调整数量限制"
          />
        </template>
        <!-- 时间 -->
        <el-table-column label="发生时间" width="190">
          <template #default="{ row }">
            <div class="pg-time-cell">
              <span class="mono pg-time-main">{{ formatTime(row.timestamp) }}</span>
              <span class="pg-time-sub">{{ formatRelative(row.timestamp) }}</span>
            </div>
          </template>
        </el-table-column>

        <!-- 类型 -->
        <el-table-column label="告警类型" width="160">
          <template #default="{ row }">
            <div class="pg-type-cell">
              <div
                class="pg-type-icon-box"
                :style="{
                  color: EVENT_META[row.type as keyof typeof EVENT_META]?.color || 'var(--pg-primary)',
                  background: 'rgba(255, 255, 255, 0.05)',
                }"
              >
                <PgIcon
                  :name="EVENT_META[row.type as keyof typeof EVENT_META]?.icon || 'alert'"
                  :size="15"
                />
              </div>
              <span class="pg-type-name">
                {{ EVENT_META[row.type as keyof typeof EVENT_META]?.label || row.type }}
              </span>
            </div>
          </template>
        </el-table-column>

        <!-- 级别 -->
        <el-table-column label="级别" width="100">
          <template #default="{ row }">
            <PgBadge
              :tone="row.level === 'danger' ? 'danger' : row.level === 'warning' ? 'warn' : 'ok'"
              size="sm"
            >
              {{ LEVEL_META[row.level as keyof typeof LEVEL_META]?.label || row.level }}
            </PgBadge>
          </template>
        </el-table-column>

        <!-- 动作 -->
        <el-table-column label="动作" width="95">
          <template #default="{ row }">
            <PgBadge
              :tone="ACTION_META[row.action as keyof typeof ACTION_META]?.tone || 'unknown'"
              variant="outline"
              size="sm"
            >
              {{ ACTION_META[row.action as keyof typeof ACTION_META]?.label || row.action }}
            </PgBadge>
          </template>
        </el-table-column>

        <!-- 结构化数据解析 -->
        <el-table-column label="遥测参数上下文" min-width="260">
          <template #default="{ row }">
            <div v-if="row.data && Object.keys(row.data).length > 0" class="pg-chips-wrap">
              <span
                v-for="d in describeEventData(row.data)"
                :key="d.key"
                class="pg-data-chip-pill"
              >
                <span class="chip-label">{{ d.label }}:</span>
                <span class="chip-value mono">{{ d.value }}</span>
              </span>
            </div>
            <span v-else class="faint">无附加参数</span>
          </template>
        </el-table-column>

        <!-- 快照证据 -->
        <el-table-column label="快照画面" width="120" align="center">
          <template #default="{ row }">
            <div v-if="row.snapshot_available && row.snapshot_id">
              <el-button
                link
                type="primary"
                size="small"
                @click="openPreview(row)"
              >
                <PgIcon name="image" :size="14" />
                <span>查看快照</span>
              </el-button>
            </div>
            <div v-else-if="row.snapshot_state === 'pending'">
              <PgBadge tone="info" size="sm" variant="outline">上传中...</PgBadge>
            </div>
            <span v-else class="faint">-</span>
          </template>
        </el-table-column>

        <!-- 事件ID -->
        <el-table-column prop="event_id" label="事件 ID" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono faint" style="font-size: 11.5px">{{ row.event_id }}</span>
          </template>
        </el-table-column>
      </el-table>
    </PgCard>

    <!-- 快照弹窗 -->
    <el-dialog
      v-model="previewVisible"
      title="车载快照证据详情"
      width="680px"
      destroy-on-close
    >
      <div v-if="activeEvent" class="pg-preview-dialog-body">
        <div class="pg-preview-img-box">
          <img :src="previewUrl" alt="Snapshot" class="pg-preview-img" />
          <div class="pg-preview-hud-info mono">
            <span>{{ activeEvent.snapshot_id }}</span>
          </div>
        </div>

        <div class="pg-preview-meta-panel">
          <div class="pg-preview-row">
            <span class="label">对应告警:</span>
            <span class="val font-semibold">
              {{ EVENT_META[activeEvent.type]?.label || activeEvent.type }}
              ({{ activeEvent.level }})
            </span>
          </div>
          <div class="pg-preview-row">
            <span class="label">捕获时间:</span>
            <span class="val mono">{{ formatTime(activeEvent.timestamp) }}</span>
          </div>
          <div class="pg-preview-row">
            <span class="label">快照状态:</span>
            <PgBadge tone="ok" size="sm">已入库已验证</PgBadge>
          </div>
          <div class="pg-preview-row" v-if="activeEvent.data">
            <span class="label">伴随遥测:</span>
            <div class="pg-chips-wrap">
              <span
                v-for="d in describeEventData(activeEvent.data)"
                :key="d.key"
                class="pg-data-chip-pill"
              >
                {{ d.label }}: <b>{{ d.value }}</b>
              </span>
            </div>
          </div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.pg-events-page {
  display: flex;
  flex-direction: column;
  gap: var(--pg-gap);
}

/* 顶部统计条 */
.pg-stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--pg-gap);
}

@media (max-width: 992px) {
  .pg-stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}

.pg-stat-chip {
  display: flex;
  align-items: center;
  gap: 14px;
  background: var(--pg-surface);
  border: 1px solid var(--pg-border);
  padding: 16px 20px;
  border-radius: var(--pg-radius-lg);
  box-shadow: var(--pg-shadow);
  backdrop-filter: blur(16px);
}

.pg-stat-icon-box {
  width: 42px;
  height: 42px;
  border-radius: 12px;
  background: var(--tone-soft);
  color: var(--tone);
  border: 1px solid color-mix(in srgb, var(--tone) 30%, transparent);
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.pg-stat-content {
  display: flex;
  flex-direction: column;
}

.pg-stat-num {
  font-size: 24px;
  font-weight: 750;
  color: #fff;
  line-height: 1.1;
}

.pg-stat-title {
  font-size: 12px;
  color: var(--pg-text-muted);
  margin-top: 2px;
}

.text-danger { color: var(--pg-danger) !important; }
.text-warn { color: var(--pg-warn) !important; }
.text-primary { color: var(--pg-primary) !important; }

/* 筛选条 */
.pg-filter-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.pg-filter-group {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.pg-filter-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pg-opt-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 表格定制 */
.pg-table-card {
  overflow: hidden;
}

.pg-time-cell {
  display: flex;
  flex-direction: column;
}

.pg-time-main {
  font-size: 12.5px;
  color: var(--pg-text);
}

.pg-time-sub {
  font-size: 11px;
  color: var(--pg-text-faint);
}

.pg-type-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pg-type-icon-box {
  width: 28px;
  height: 28px;
  border-radius: 7px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.pg-type-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--pg-text);
}

.pg-chips-wrap {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.pg-data-chip-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: var(--pg-radius-xs);
  background: rgba(30, 41, 64, 0.45);
  border: 1px solid var(--pg-border);
  font-size: 11.5px;
}

.chip-label {
  color: var(--pg-text-muted);
}

.chip-value {
  color: var(--pg-text);
  font-weight: 600;
}

/* 预览模态框 */
.pg-preview-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.pg-preview-img-box {
  position: relative;
  width: 100%;
  height: 360px;
  background: #030712;
  border-radius: var(--pg-radius-sm);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--pg-border);
}

.pg-preview-img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.pg-preview-hud-info {
  position: absolute;
  bottom: 8px;
  right: 8px;
  background: rgba(3, 7, 18, 0.75);
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 11px;
  color: var(--pg-primary);
  border: 1px solid rgba(34, 211, 238, 0.3);
}

.pg-preview-meta-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: rgba(30, 41, 64, 0.4);
  padding: 14px 18px;
  border-radius: var(--pg-radius-sm);
  border: 1px solid var(--pg-border);
}

.pg-preview-row {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}

.pg-preview-row .label {
  width: 80px;
  color: var(--pg-text-muted);
  flex-shrink: 0;
}

.pg-preview-row .val {
  color: var(--pg-text);
}
</style>
