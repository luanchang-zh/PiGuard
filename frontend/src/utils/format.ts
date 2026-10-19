import { EVENT_DATA_FIELDS, EVENT_META, DIRECTION_LABEL, REASON_LABEL, SENSOR_LABEL } from './meta'

export function isNum(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v)
}

export function formatNumber(value: number | null | undefined, digits = 1): string {
  if (!isNum(value)) return '--'
  return value.toFixed(digits)
}

function toDate(iso?: string | number | Date | null): Date | null {
  if (iso === undefined || iso === null || iso === '') return null
  const d = iso instanceof Date ? iso : new Date(iso)
  return Number.isNaN(d.getTime()) ? null : d
}

const pad = (n: number, w = 2) => String(n).padStart(w, '0')

/** 2026-10-19 15:13:36 */
export function formatTime(iso?: string | null): string {
  const d = toDate(iso)
  if (!d) return '--'
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 15:13:36 */
export function formatClock(iso?: string | number | Date | null, withMs = false): string {
  const d = toDate(iso)
  if (!d) return '--:--:--'
  const base = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  return withMs ? `${base}.${pad(d.getMilliseconds(), 3)}` : base
}

/** 今天显示时间，其它日期显示 月-日 时间 */
export function formatSmartTime(iso?: string | null): string {
  const d = toDate(iso)
  if (!d) return '--'
  const now = new Date()
  if (d.toDateString() === now.toDateString()) return formatClock(d)
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${formatClock(d)}`
}

/** 相对时间：刚刚 / 12 秒前 / 3 分钟前 */
export function formatRelative(iso: string | number | Date | null | undefined, now = Date.now()): string {
  const d = toDate(iso)
  if (!d) return '--'
  const diff = Math.max(0, now - d.getTime())
  const s = Math.floor(diff / 1000)
  if (s < 3) return '刚刚'
  if (s < 60) return `${s} 秒前`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} 分钟前`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} 小时前`
  return `${Math.floor(h / 24)} 天前`
}

export function formatDuration(ms: number | null | undefined): string {
  if (!isNum(ms)) return '--'
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)} s`
}

export function eventTypeLabel(type: string): string {
  return (EVENT_META as Record<string, { label: string }>)[type]?.label ?? type
}

/** 把告警 data 转为可读的键值对 */
export function describeEventData(data?: Record<string, unknown> | null): { key: string; label: string; value: string }[] {
  if (!data) return []
  return Object.entries(data).map(([key, raw]) => {
    const def = EVENT_DATA_FIELDS[key]
    let value: string
    if (isNum(raw)) {
      value = raw.toFixed(def?.digits ?? 2) + (def?.unit ? ` ${def.unit}` : '')
    } else if (key === 'direction' && typeof raw === 'string') {
      value = DIRECTION_LABEL[raw] ?? raw
    } else if (key === 'reason' && typeof raw === 'string') {
      value = REASON_LABEL[raw] ?? raw
    } else if (key === 'sensor' && typeof raw === 'string') {
      value = SENSOR_LABEL[raw] ?? raw
    } else if (raw === null || raw === undefined) {
      value = '--'
    } else if (typeof raw === 'object') {
      value = JSON.stringify(raw)
    } else {
      value = String(raw)
    }
    return { key, label: def?.label ?? key, value }
  })
}

/** 把命令参数转成简短描述 */
export function describeParams(params?: Record<string, unknown> | null): string {
  if (!params) return ''
  const parts: string[] = []
  for (const [k, v] of Object.entries(params)) {
    if (k === 'duration_ms' && isNum(v)) parts.push(`${v} ms`)
    else if (k === 'quality' && isNum(v)) parts.push(`画质 ${v}%`)
    else if (k === 'speed' && isNum(v)) parts.push(`${v}x`)
    else parts.push(String(v))
  }
  return parts.join(' · ')
}

/** 生成 CSV 文本，带 BOM 方便 Excel 识别中文 */
export function toCsv(rows: (string | number | null | undefined)[][]): string {
  const esc = (v: string | number | null | undefined) => {
    const s = v === null || v === undefined ? '' : String(v)
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
  }
  return '\ufeff' + rows.map((r) => r.map(esc).join(',')).join('\n')
}

export function downloadText(filename: string, text: string, mime = 'text/csv;charset=utf-8') {
  const blob = new Blob([text], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
