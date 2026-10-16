import { ref, onUnmounted } from 'vue'
import type {
  WsMessage,
  WsTelemetry,
  WsEvent,
  WsDeviceStatus,
  WsCommandUpdate,
  WsFrameUpdate,
  WsEventImageUpdate,
} from '@/types'

// WebSocket 订阅 /api/v1/ws。浏览器自动携带 Cookie，
// 后端在 originGuard/accessGuard 中完成鉴权。
export function useRealtime() {
  const connected = ref(false)
  const telemetry = ref<WsTelemetry | null>(null)
  const events = ref<WsEvent[]>([])
  const deviceStatus = ref<WsDeviceStatus | null>(null)
  const commandUpdates = ref<WsCommandUpdate[]>([])
  const frameUpdates = ref<WsFrameUpdate[]>([])
  const eventImageUpdates = ref<WsEventImageUpdate[]>([])

  let ws: WebSocket | null = null
  let reconnectTimer: number | null = null
  let closedByUser = false

  function invalidateConnection() {
    connected.value = false
    // 重连后必须等到新的推送，才能再次信任实时状态。
    telemetry.value = null
    deviceStatus.value = null
  }

  function connect() {
    closedByUser = false
    open()
  }

  function open() {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${proto}//${window.location.host}/api/v1/ws`
    ws = new WebSocket(url)

    ws.onopen = () => {
      connected.value = true
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
      if (!closedByUser) {
        reconnectTimer = window.setTimeout(open, 3000)
      }
    }

    ws.onerror = () => {
      invalidateConnection()
      ws?.close()
    }
  }

  function dispatch(msg: WsMessage) {
    switch (msg.type) {
      case 'telemetry':
        telemetry.value = msg.data as WsTelemetry
        break
      case 'event':
        events.value.unshift(msg.data as WsEvent)
        if (events.value.length > 200) events.value.pop()
        break
      case 'device_status':
        deviceStatus.value = msg.data as WsDeviceStatus
        break
      case 'command_update':
        commandUpdates.value.unshift(msg.data as WsCommandUpdate)
        if (commandUpdates.value.length > 100) commandUpdates.value.pop()
        break
      case 'frame_update':
        frameUpdates.value.unshift(msg.data as WsFrameUpdate)
        if (frameUpdates.value.length > 50) frameUpdates.value.pop()
        break
      case 'event_image_update':
        eventImageUpdates.value.unshift(msg.data as WsEventImageUpdate)
        if (eventImageUpdates.value.length > 100) eventImageUpdates.value.pop()
        break
    }
  }

  function disconnect() {
    closedByUser = true
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    ws?.close()
    ws = null
    invalidateConnection()
  }

  onUnmounted(disconnect)

  return {
    connected,
    telemetry,
    events,
    deviceStatus,
    commandUpdates,
    frameUpdates,
    eventImageUpdates,
    connect,
    disconnect,
  }
}
