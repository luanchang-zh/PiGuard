// 与 Go 后端 /api/v1 协议一致的类型定义。
// 字段命名沿用后端 JSON tag，避免额外的转换层。

// ---------- 通用响应信封 ----------
export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data?: T
  error?: { field?: string } | Record<string, unknown>
}

// ---------- 认证 ----------
export interface SessionInfo {
  username: string
  role: 'platform' | 'device'
  auth_mode: 'basic' | 'cookie' | 'none'
  expires_at?: string
  csrf_token?: string
}

// ---------- 设备 ----------
export interface Device {
  device_id: string
  name: string
  online: boolean
  last_seen_at?: string
  config_version: number
  mode?: string
  software_version?: string
  sensors?: Record<string, SensorStatus>
}

export type SensorStatus = 'ok' | 'timeout' | 'unavailable' | 'error'

// ---------- 最新状态（GET /state）----------
export interface DeviceState {
  device_id: string
  timestamp?: string
  online: boolean
  speed_kmh?: number
  distance_m?: number
  temperature_c?: number
  yaw_rate_deg_s?: number
  lane: LaneState
  risk_level: RiskLevel
  samples: Record<string, SampleInfo>
  freshness: Record<string, Freshness>
  active_events: string[]
}

export interface LaneState {
  valid: boolean
  offset_ratio?: number
  direction: 'left' | 'center' | 'right' | 'unknown'
  confidence?: number
  sample_at?: string
  source?: string
  status?: SensorStatus
}

export type RiskLevel = 'normal' | 'warning' | 'danger' | 'unknown'

export interface SampleInfo {
  sample_at?: string
  source?: string
  status?: SensorStatus
}

export type Freshness = 'fresh' | 'stale' | 'fault' | 'missing'

// ---------- 遥测历史 ----------
export interface TelemetryItem {
  timestamp: string
  speed_kmh: number | null
  distance_m: number | null
  temperature_c: number | null
  yaw_rate_deg_s: number | null
  lane_offset_ratio: number | null
  risk_level: RiskLevel
  samples: Record<string, SampleInfo>
  freshness: Record<string, Freshness>
}

export interface TelemetryList {
  items: TelemetryItem[]
}

// ---------- 告警事件 ----------
export type EventType =
  | 'obstacle_warning'
  | 'lane_departure'
  | 'sharp_turn'
  | 'high_temperature'
  | 'sensor_failure'

export type EventAction = 'started' | 'level_changed' | 'recovered'
export type EventLevel = 'normal' | 'warning' | 'danger'

export interface EventImage {
  frame_id?: string
  snapshot_id?: string
  snapshot_available?: boolean
  snapshot_url?: string
  snapshot_state: 'none' | 'pending' | 'available' | 'invalid' | 'missing' | 'unavailable'
}

export interface EventItem {
  event_id: string
  type: EventType
  action: EventAction
  level: EventLevel
  timestamp: string
  data?: Record<string, unknown>
  frame_id?: string
  snapshot_id?: string
  snapshot_available?: boolean
  snapshot_url?: string
  snapshot_state?: string
}

export interface EventList {
  items: EventItem[]
}

// ---------- 命令 ----------
export type CommandType =
  | 'buzzer.test'
  | 'camera.snapshot'
  | 'scenario.start'
  | 'scenario.stop'
  | 'config.update'

export type CommandStatus = 'pending' | 'sent' | 'success' | 'failed' | 'timeout'

export interface CommandCreateRequest {
  type: CommandType
  params: Record<string, unknown>
}

export interface CommandView {
  command_id: string
  device_id: string
  type: CommandType
  params: Record<string, unknown>
  status: CommandStatus
  issued_at: string
  expires_at: string
  ack_at?: string
  executed_at?: string
  result?: Record<string, unknown>
  error?: { code: string; message: string }
}

export interface CommandCreateResult {
  command_id: string
  status: CommandStatus
}

// ---------- 配置 ----------
export interface RuleSet {
  obstacle: ObstacleRule
  lane_departure: LaneDepartureRule
  sharp_turn: SharpTurnRule
  temperature: TemperatureRule
}

export interface ObstacleRule {
  warning_distance_m: number
  danger_distance_m: number
}

export interface LaneDepartureRule {
  offset_threshold: number
  duration_ms: number
}

export interface SharpTurnRule {
  yaw_threshold_deg_s: number
  min_speed_kmh: number
  duration_ms: number
}

export interface TemperatureRule {
  trigger_c: number
  recover_c: number
  duration_ms: number
}

export interface ConfigView {
  desired_version: number
  reported_version: number
  rules: RuleSet
  status: 'pending' | 'success' | 'failed'
  ack_at?: string
  applied_at?: string
  error?: { code: string; message: string }
}

export interface ConfigUpdateRequest {
  expected_version: number
  rules: Partial<RuleSet>
}

export interface ConfigUpdateResult {
  config_version: number
  status: 'pending'
}

// ---------- 图片 ----------
export interface SnapshotUploadResult {
  snapshot_id: string
  device_id: string
  type: 'preview' | 'snapshot' | 'event'
  captured_at: string
}

export interface LatestFrame {
  snapshot_id: string
  device_id: string
  type: string
  captured_at: string
  url: string
}

// ---------- WebSocket 推送 ----------
export interface WsMessage<T = unknown> {
  type: WsType
  timestamp: string
  data: T
}

export type WsType =
  | 'telemetry'
  | 'event'
  | 'event_image_update'
  | 'device_status'
  | 'command_update'
  | 'config_update'
  | 'frame_update'

export interface WsTelemetry {
  device_id: string
  timestamp?: string
  online: boolean
  speed_kmh?: number
  distance_m?: number
  temperature_c?: number
  yaw_rate_deg_s?: number
  lane: LaneState
  risk_level: RiskLevel
  samples: Record<string, SampleInfo>
  freshness: Record<string, Freshness>
  active_events: string[]
}

export interface WsEvent {
  event_id: string
  device_id: string
  event_type: EventType
  action: EventAction
  level: EventLevel
  timestamp: string
  data?: Record<string, unknown>
  frame_id?: string
  snapshot_id?: string
  snapshot_available?: boolean
  snapshot_url?: string
  snapshot_state?: string
}

export interface WsEventImageUpdate {
  event_id: string
  device_id: string
  event_image: EventImage
}

export interface WsDeviceStatus {
  device_id: string
  online: boolean
}

export interface WsCommandUpdate {
  command_id: string
  device_id: string
  status: CommandStatus
}

export interface WsFrameUpdate {
  device_id: string
  snapshot_id: string
  type: string
  captured_at: string
  url?: string
}
