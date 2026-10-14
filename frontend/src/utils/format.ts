import type { RiskLevel, SensorStatus, Freshness, EventLevel, CommandStatus } from '@/types'

export function formatNumber(value: number | null | undefined, digits = 1): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '--'
  return value.toFixed(digits)
}

export function formatTime(iso?: string): string {
  if (!iso) return '--'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('zh-CN', { hour12: false })
}

export function riskLabel(level: RiskLevel): string {
  return ({ normal: '正常', warning: '警告', danger: '危险', unknown: '未知' } as const)[level]
}

export function riskTagType(level: RiskLevel): '' | 'success' | 'warning' | 'danger' | 'info' {
  return ({ normal: 'success', warning: 'warning', danger: 'danger', unknown: 'info' } as const)[level]
}

export function eventLevelTag(level: EventLevel): '' | 'success' | 'warning' | 'danger' | 'info' {
  return ({ normal: 'success', warning: 'warning', danger: 'danger' } as const)[level]
}

export function eventTypeLabel(type: string): string {
  const map: Record<string, string> = {
    obstacle_warning: '障碍物预警',
    lane_departure: '车道偏离',
    sharp_turn: '急转弯',
    high_temperature: '车内高温',
    sensor_failure: '传感器故障',
  }
  return map[type] ?? type
}

export function eventActionLabel(action: string): string {
  return ({ started: '开始', level_changed: '升级', recovered: '恢复' } as const)[action] ?? action
}

export function commandStatusTag(status: CommandStatus): '' | 'success' | 'warning' | 'danger' | 'info' {
  return ({
    pending: 'warning',
    sent: 'info',
    success: 'success',
    failed: 'danger',
    timeout: 'danger',
  } as const)[status]
}

export function sensorStatusText(s: SensorStatus): string {
  return ({ ok: '正常', timeout: '超时', unavailable: '不可用', error: '错误' } as const)[s]
}

export function sensorStatusTag(s: SensorStatus): '' | 'success' | 'warning' | 'danger' | 'info' {
  return ({ ok: 'success', timeout: 'warning', unavailable: 'info', error: 'danger' } as const)[s]
}

export function freshnessText(f: Freshness): string {
  return ({ fresh: '新鲜', stale: '过期', fault: '故障', missing: '缺失' } as const)[f]
}

export function freshnessTag(f: Freshness): '' | 'success' | 'warning' | 'danger' | 'info' {
  return ({ fresh: 'success', stale: 'warning', fault: 'danger', missing: 'info' } as const)[f]
}

export function directionLabel(d: string): string {
  return ({ left: '左偏', center: '居中', right: '右偏', unknown: '未知' } as const)[d] ?? d
}
