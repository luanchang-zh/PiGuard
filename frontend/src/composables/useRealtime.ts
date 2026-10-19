import { ref } from 'vue'
import type {
  WsMessage,
  WsTelemetry,
  WsEvent,
  WsDeviceStatus,
  WsCommandUpdate,
  WsFrameUpdate,
  WsEventImageUpdate,
} from '@/types'

// 单例状态：整个应用共享同一个 WebSocket 连接与消息队列，避免多组件重复建立连接
const connected = ref(false)
const connecting = ref(false)
const lastMessageAt = ref<Date | null>(null)
const reconnectCount = ref(0)

const telemetry = ref<WsTelemetry | null>(null)
const events = ref<WsEvent[]>([])
const deviceStatus = ref<WsDeviceStatus | null>(null)
const commandUpdates = ref<WsCommandUpdate[]>([])
const frameUpdates = ref<WsFrameUpdate[]>([])
const eventImageUpdates = ref<WsEventImageUpdate[]>([])

// 报警音效开关（默认静音，用户可在界面开启）
const audioAlertEnabled = ref(false)

let ws: WebSocket | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let closedByUser = false
let audioCtx: AudioContext | null = null

function playAlarmBeep(level: 'warning' | 'danger') {
  if (!audioAlertEnabled.value) return
  try {
    if (!audioCtx) {
      const AudioContextClass = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
      if (!AudioContextClass) return
      audioCtx = new AudioContextClass()
    }
    if (audioCtx.state === 'suspended') {
      audioCtx.resume()
    }
    const osc = audioCtx.createOscillator()
    const gain = audioCtx.createGain()
    osc.connect(gain)
    gain.connect(audioCtx.destination)

    const now = audioCtx.currentTime
    if (level === 'danger') {
      // 危险：双高频急促报警 (880Hz -> 1046Hz)
      osc.type = 'sawtooth'
      osc.frequency.setValueAtTime(880, now)
      osc.frequency.setValueAtTime(1046, now + 0.1)
      gain.gain.setValueAtTime(0.15, now)
      gain.gain.exponentialRampToValueAtTime(0.01, now + 0.25)
      osc.start(now)
      osc.stop(now + 0.25)
    } else {
      // 警告：柔和双音提示 (587Hz -> 659Hz)
      osc.type = 'sine'
      osc.frequency.setValueAtTime(587.33, now)
      osc.frequency.setValueAtTime(659.25, now + 0.12)
      gain.gain.setValueAtTime(0.12, now)
      gain.gain.exponentialRampToValueAtTime(0.01, now + 0.3)
      osc.start(now)
      osc.stop(now + 0.3)
    }
  } catch {
    /* 忽略音频策略限制 */
  }
}

function invalidateConnection() {
  connected.value = false
  connecting.value = false
  // 断开后实时遥测失效，等待重新推送
  telemetry.value = null
  deviceStatus.value = null
}

function dispatch(msg: WsMessage) {
  lastMessageAt.value = new Date()
  switch (msg.type) {
    case 'telemetry':
      telemetry.value = msg.data as WsTelemetry
      break

    case 'event': {
      const ev = msg.data as WsEvent
      // 去重加入头部
      const existsIndex = events.value.findIndex((e) => e.event_id === ev.event_id)
      if (existsIndex >= 0) {
        events.value[existsIndex] = ev
      } else {
        events.value.unshift(ev)
        if (events.value.length > 300) events.value.pop()
        if (ev.action === 'started' || ev.action === 'level_changed') {
          playAlarmBeep(ev.level === 'danger' ? 'danger' : 'warning')
        }
      }
      break
    }

    case 'device_status':
      deviceStatus.value = msg.data as WsDeviceStatus
      break

    case 'command_update': {
      const cmd = msg.data as WsCommandUpdate
      const idx = commandUpdates.value.findIndex((c) => c.command_id === cmd.command_id)
      if (idx >= 0) {
        commandUpdates.value[idx] = cmd
      } else {
        commandUpdates.value.unshift(cmd)
        if (commandUpdates.value.length > 100) commandUpdates.value.pop()
      }
      break
    }

    case 'frame_update':
      frameUpdates.value.unshift(msg.data as WsFrameUpdate)
      if (frameUpdates.value.length > 50) frameUpdates.value.pop()
      break

    case 'event_image_update': {
      const imgUpdate = msg.data as WsEventImageUpdate
      eventImageUpdates.value.unshift(imgUpdate)
      if (eventImageUpdates.value.length > 100) eventImageUpdates.value.pop()

      // 同步更新缓存中对应告警的图片引用信息
      const target = events.value.find((e) => e.event_id === imgUpdate.event_id)
      if (target) {
        const img = imgUpdate.event_image
        target.snapshot_id = img.snapshot_id
        target.snapshot_available = img.snapshot_available
        target.snapshot_url = img.snapshot_url
        target.snapshot_state = img.snapshot_state
        target.frame_id = img.frame_id
      }
      break
    }
  }
}

function open() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
    return
  }

  connecting.value = true
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = `${proto}//${window.location.host}/api/v1/ws`

  try {
    ws = new WebSocket(url)
  } catch {
    connecting.value = false
    scheduleReconnect()
    return
  }

  ws.onopen = () => {
    connected.value = true
    connecting.value = false
    reconnectCount.value = 0
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  ws.onmessage = (ev) => {
    try {
      const msg = JSON.parse(ev.data) as WsMessage
      dispatch(msg)
    } catch {
      /* 忽略非 JSON 帧 */
    }
  }

  ws.onclose = () => {
    invalidateConnection()
    ws = null
    if (!closedByUser) {
      scheduleReconnect()
    }
  }

  ws.onerror = () => {
    invalidateConnection()
    ws?.close()
    ws = null
  }
}

function scheduleReconnect() {
  if (reconnectTimer || closedByUser) return
  reconnectCount.value++
  // 指数退避 (1s, 2s, 4s, 最大 8s) + 随机抖动
  const delay = Math.min(8000, 1000 * Math.pow(1.5, Math.min(reconnectCount.value, 6))) + Math.random() * 500
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    open()
  }, delay)
}

export function useRealtime() {
  function connect() {
    closedByUser = false
    open()
  }

  function disconnect() {
    closedByUser = true
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (ws) {
      ws.close()
      ws = null
    }
    invalidateConnection()
  }

  function toggleAudioAlert() {
    audioAlertEnabled.value = !audioAlertEnabled.value
  }

  return {
    connected,
    connecting,
    reconnectCount,
    lastMessageAt,
    telemetry,
    events,
    deviceStatus,
    commandUpdates,
    frameUpdates,
    eventImageUpdates,
    audioAlertEnabled,
    connect,
    disconnect,
    toggleAudioAlert,
  }
}
