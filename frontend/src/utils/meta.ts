import type {
  CommandStatus,
  CommandType,
  EventAction,
  EventLevel,
  EventType,
  Freshness,
  RiskLevel,
  SensorStatus,
} from '@/types'
import type { IconName } from '@/components/ui/icons'

/** 语义色调，对应 tokens.css 中的 --pg-ok / --pg-warn / --pg-danger / --pg-unknown / --pg-info。 */
export type Tone = 'ok' | 'warn' | 'danger' | 'unknown' | 'info' | 'primary'

export interface Meta {
  label: string
  tone: Tone
}

// ---------- 风险等级 ----------
export const RISK_META: Record<RiskLevel, Meta & { desc: string }> = {
  normal: { label: '正常', tone: 'ok', desc: '各项指标处于安全范围' },
  warning: { label: '警告', tone: 'warn', desc: '存在潜在风险，请注意驾驶' },
  danger: { label: '危险', tone: 'danger', desc: '高风险，请立即采取措施' },
  unknown: { label: '未知', tone: 'unknown', desc: '暂无可信的实时数据' },
}

export const LEVEL_META: Record<EventLevel, Meta> = {
  normal: { label: '正常', tone: 'ok' },
  warning: { label: '警告', tone: 'warn' },
  danger: { label: '危险', tone: 'danger' },
}

// ---------- 告警类型 ----------
export const EVENT_META: Record<EventType, { label: string; icon: IconName; color: string; desc: string }> = {
  obstacle_warning: {
    label: '障碍物预警',
    icon: 'radar',
    color: 'var(--pg-c-distance)',
    desc: '前方距离或车头时距低于阈值',
  },
  lane_departure: {
    label: '车道偏离',
    icon: 'road',
    color: 'var(--pg-c-lane)',
    desc: '车辆偏离车道中心超过阈值',
  },
  sharp_turn: {
    label: '急转弯',
    icon: 'turn',
    color: 'var(--pg-c-yaw)',
    desc: '偏航角速度过大且车速较高',
  },
  high_temperature: {
    label: '车内高温',
    icon: 'flame',
    color: 'var(--pg-c-temp)',
    desc: '车内温度持续高于触发值',
  },
  sensor_failure: {
    label: '传感器故障',
    icon: 'octagon',
    color: 'var(--pg-danger)',
    desc: '传感器超时、不可用或出错',
  },
}

export const EVENT_TYPES = Object.keys(EVENT_META) as EventType[]

export const ACTION_META: Record<EventAction, Meta> = {
  started: { label: '触发', tone: 'warn' },
  level_changed: { label: '升级', tone: 'danger' },
  recovered: { label: '恢复', tone: 'ok' },
}

// 告警 data 字段的人类可读名称与单位
export const EVENT_DATA_FIELDS: Record<string, { label: string; unit?: string; digits?: number }> = {
  distance_m: { label: '距离', unit: 'm', digits: 2 },
  speed_kmh: { label: '车速', unit: 'km/h', digits: 1 },
  headway_s: { label: '车头时距', unit: 's', digits: 2 },
  offset_ratio: { label: '偏移', digits: 2 },
  direction: { label: '方向' },
  yaw_rate_deg_s: { label: '角速度', unit: '°/s', digits: 1 },
  temperature_c: { label: '温度', unit: '°C', digits: 1 },
  sensor: { label: '传感器' },
  reason: { label: '原因' },
}

// ---------- 传感器 ----------
export const SENSORS: { key: string; label: string; model: string; icon: IconName }[] = [
  { key: 'distance', label: '超声波测距', model: 'HC-SR04', icon: 'radar' },
  { key: 'lane', label: '摄像头车道', model: 'OV5647', icon: 'camera' },
  { key: 'gyro', label: '陀螺仪', model: 'MPU6050', icon: 'compass' },
  { key: 'temperature', label: '温湿度', model: 'AHT20', icon: 'thermometer' },
  { key: 'speed', label: '车速', model: '场景/估算', icon: 'gauge' },
]

export const SENSOR_LABEL: Record<string, string> = Object.fromEntries(
  SENSORS.map((s) => [s.key, s.label]),
)

export const SENSOR_STATUS_META: Record<SensorStatus, Meta> = {
  ok: { label: '正常', tone: 'ok' },
  timeout: { label: '超时', tone: 'warn' },
  unavailable: { label: '不可用', tone: 'unknown' },
  error: { label: '错误', tone: 'danger' },
}

export const FRESHNESS_META: Record<Freshness, Meta> = {
  fresh: { label: '实时', tone: 'ok' },
  stale: { label: '过期', tone: 'warn' },
  fault: { label: '故障', tone: 'danger' },
  missing: { label: '缺失', tone: 'unknown' },
}

export const REASON_LABEL: Record<string, string> = {
  timeout: '超时',
  unavailable: '不可用',
  error: '错误',
  stale: '数据过期',
  connection_lost: '连接丢失',
}

export const DIRECTION_LABEL: Record<string, string> = {
  left: '左偏',
  center: '居中',
  right: '右偏',
  unknown: '未知',
}

// ---------- 命令 ----------
export const COMMAND_META: Record<CommandType, { label: string; icon: IconName }> = {
  'buzzer.test': { label: '蜂鸣器测试', icon: 'volume' },
  'indicator.test': { label: '指示灯测试', icon: 'bulb' },
  'camera.snapshot': { label: '远程拍照', icon: 'camera' },
  'scenario.start': { label: '启动场景', icon: 'play' },
  'scenario.stop': { label: '停止场景', icon: 'stop' },
  'config.update': { label: '配置下发', icon: 'sliders' },
}

export const COMMAND_STATUS_META: Record<CommandStatus, Meta & { final: boolean }> = {
  pending: { label: '已受理', tone: 'info', final: false },
  sent: { label: '已送达', tone: 'primary', final: false },
  success: { label: '执行成功', tone: 'ok', final: true },
  failed: { label: '执行失败', tone: 'danger', final: true },
  timeout: { label: '超时', tone: 'warn', final: true },
}

// ---------- 模拟场景 ----------
export const SCENARIOS: { value: string; label: string; desc: string; icon: IconName; tone: Tone }[] = [
  { value: 'normal_drive', label: '正常行驶', desc: '车道居中、距离安全', icon: 'car', tone: 'ok' },
  { value: 'obstacle_approach', label: '障碍物接近', desc: '前车逐步逼近直至危险', icon: 'radar', tone: 'danger' },
  { value: 'lane_departure', label: '车道偏离', desc: '车辆持续向一侧偏移', icon: 'road', tone: 'warn' },
  { value: 'sharp_turn', label: '急转弯', desc: '高速下偏航角速度骤增', icon: 'turn', tone: 'warn' },
  { value: 'high_temperature', label: '车内高温', desc: '温度攀升超过触发值', icon: 'flame', tone: 'warn' },
  { value: 'sensor_failure', label: '传感器故障', desc: '测距/摄像头/陀螺仪依次失效', icon: 'octagon', tone: 'danger' },
]

export const SCENARIO_LABEL: Record<string, string> = Object.fromEntries(
  SCENARIOS.map((s) => [s.value, s.label]),
)

export const INDICATOR_COLORS = [
  { value: 'green', label: '绿色', hex: '#34d399' },
  { value: 'yellow', label: '黄色', hex: '#fbbf24' },
  { value: 'red', label: '红色', hex: '#fb5174' },
] as const

export const INDICATOR_LABEL: Record<string, string> = Object.fromEntries(
  INDICATOR_COLORS.map((c) => [c.value, c.label]),
)
