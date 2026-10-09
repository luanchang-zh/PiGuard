# 车载驾驶辅助预警系统：网络交互与 API 规范

> 适用项目：基于 Raspberry Pi 4B 的简易车载驾驶辅助预警系统  
> 文档目的：统一 Raspberry Pi 边缘端、Go 平台端、Vue 前端之间的网络交互方式与数据格式。  
> 协议版本：v1  
> 默认设备 ID：`car-001`

---

# 1. 网络交互总览

系统采用三类网络协议：

| 协议 | 用途 | 通信双方 |
|---|---|---|
| MQTT | 遥测、告警、设备状态、下行控制、命令回执 | Raspberry Pi ↔ Go 平台 |
| HTTP REST | 前端查询、下发命令、读取历史数据、上传摄像头图片 | Vue ↔ Go、Raspberry Pi → Go |
| WebSocket | 平台向前端实时推送状态、告警和命令结果 | Go → Vue |

整体数据流：

```text
┌──────────────┐
│ Raspberry Pi │
└──────┬───────┘
       │
       │ MQTT
       │
       ▼
┌──────────────┐
│  Mosquitto   │
└──────┬───────┘
       │
       │ MQTT
       ▼
┌──────────────┐
│  Go Backend  │
└──────┬───────┘
       │
       ├── REST API
       │
       └── WebSocket
       │
       ▼
┌──────────────┐
│   Vue Web    │
└──────────────┘
```

摄像头图片单独采用 HTTP：

```text
Raspberry Pi
      │
      │ HTTP POST JPEG
      ▼
Go Backend
      │
      ▼
Vue Web
```

---

# 2. 通用数据约定

## 2.1 时间格式

所有跨设备时间统一使用：

```text
ISO 8601 UTC
```

例如：

```text
2026-09-29T10:30:00.123Z
```

Go：

```go
time.RFC3339Nano
```

Python：

```python
datetime.now(timezone.utc).isoformat()
```

---

## 2.2 ID 格式

### device_id

设备唯一编号：

```text
car-001
```

### command_id

控制命令唯一 ID：

```text
cmd-20260929-000001
```

推荐实际代码使用 UUID：

```text
550e8400-e29b-41d4-a716-446655440000
```

### event_id

事件唯一 ID：

```text
evt-20260929-000001
```

同样推荐 UUID。

### message_id

用于消息追踪：

```text
msg-xxxx
```

---

## 2.3 schema_version

所有 MQTT 消息建议带：

```json
{
  "schema_version": 1
}
```

后续协议升级：

```text
schema_version = 2
```

可以兼容旧设备。

---

# 3. MQTT 连接规范

## 3.1 Broker

默认：

```text
mqtt://192.168.1.100:1883
```

开发阶段可以：

```text
1883
```

无 TLS。

如果后续需要增强安全性：

```text
8883
```

使用 TLS。

---

# 4. MQTT Topic 规范

统一使用：

```text
car/{device_id}/{type}
```

完整 Topic：

```text
car/{device_id}/telemetry

car/{device_id}/events

car/{device_id}/status

car/{device_id}/commands

car/{device_id}/command-acks

car/{device_id}/config

car/{device_id}/config-acks
```

示例：

```text
car/car-001/telemetry
```

---

# 5. MQTT QoS 规定

| Topic | QoS | Retain |
|---|---:|---|
| telemetry | 0 | false |
| events | 1 | false |
| status | 1 | true |
| commands | 1 | false |
| command-acks | 1 | false |
| config | 1 | true |
| config-acks | 1 | false |

说明：

### telemetry

高频数据。

偶尔丢一条不影响整体功能，因此：

```text
QoS 0
```

### events

告警事件不能随便丢：

```text
QoS 1
```

### commands

控制命令必须尽量可靠：

```text
QoS 1
```

但由于 QoS 1 可能重复投递，所以必须通过：

```text
command_id
```

做幂等。

---

# 6. MQTT Telemetry

Topic：

```text
car/{device_id}/telemetry
```

方向：

```text
Raspberry Pi -> Platform
```

默认频率：

```text
2 Hz
```

消息：

```json
{
  "schema_version": 1,
  "message_id": "msg-001",
  "device_id": "car-001",
  "timestamp": "2026-09-29T10:30:00.123Z",
  "seq": 1024,

  "speed": {
    "value": 32.4,
    "unit": "km/h",
    "source": "mock",
    "status": "ok",
    "sample_at": "2026-09-29T10:30:00.100Z"
  },

  "distance": {
    "raw_value": 0.18,
    "mapped_value": 18.0,
    "unit": "m",
    "scale": 100,
    "source": "mock",
    "status": "ok",
    "sample_at": "2026-09-29T10:30:00.110Z"
  },

  "temperature": {
    "value": 27.5,
    "unit": "C",
    "source": "mock",
    "status": "ok",
    "sample_at": "2026-09-29T10:29:59.900Z"
  },

  "gyro": {
    "x": 0.6,
    "y": 1.2,
    "z": 12.8,
    "yaw_rate": 12.8,
    "unit": "deg/s",
    "source": "mock",
    "status": "ok",
    "sample_at": "2026-09-29T10:30:00.120Z"
  },

  "lane": {
    "valid": true,
    "offset_ratio": 0.18,
    "direction": "right",
    "confidence": 0.91,
    "sample_at": "2026-09-29T10:30:00.000Z"
  },

  "risk": {
    "level": "normal",
    "active_events": []
  }
}
```

---

# 7. Telemetry 字段定义

## speed.source

可选：

```text
mock
hardware
estimated
replay
```

## status

可选：

```text
ok
timeout
unavailable
error
```

## lane.direction

可选：

```text
left
center
right
unknown
```

## risk.level

可选：

```text
normal
warning
danger
unknown
```

---

# 8. MQTT Event

Topic：

```text
car/{device_id}/events
```

方向：

```text
Raspberry Pi -> Platform
```

用于：

```text
告警开始
告警升级
告警恢复
传感器故障
设备异常
```

---

# 9. Event 消息

```json
{
  "schema_version": 1,
  "event_id": "evt-001",
  "device_id": "car-001",

  "type": "obstacle_warning",

  "action": "started",

  "level": "danger",

  "timestamp": "2026-09-29T10:30:12.000Z",

  "data": {
    "distance_m": 6.8,
    "speed_kmh": 32.0,
    "headway_s": 0.76
  },

  "snapshot_available": true
}
```

---

# 10. Event type

第一版规定：

```text
obstacle_warning

lane_departure

sharp_turn

high_temperature

sensor_failure
```

---

# 11. Event action

```text
started

level_changed

recovered
```

例如：

```text
NORMAL -> WARNING
```

：

```text
started
```

```text
WARNING -> DANGER
```

：

```text
level_changed
```

```text
DANGER -> NORMAL
```

：

```text
recovered
```

---

# 12. Sensor Failure Event

```json
{
  "schema_version": 1,
  "event_id": "evt-002",
  "device_id": "car-001",
  "type": "sensor_failure",
  "action": "started",
  "level": "warning",
  "timestamp": "2026-09-29T10:31:00Z",

  "data": {
    "sensor": "distance",
    "reason": "timeout"
  }
}
```

---

# 13. MQTT Device Status

Topic：

```text
car/{device_id}/status
```

方向：

```text
Raspberry Pi -> Platform
```

Retain：

```text
true
```

消息：

```json
{
  "schema_version": 1,
  "device_id": "car-001",

  "online": true,

  "timestamp": "2026-09-29T10:30:00Z",

  "software_version": "0.1.0",

  "config_version": 5,

  "mode": "mock",

  "sensors": {
    "distance": "ok",
    "camera": "ok",
    "gyro": "ok",
    "temperature": "ok"
  },

  "mqtt": {
    "connected": true
  }
}
```

---

# 14. MQTT Last Will

树莓派建立 MQTT 连接时设置：

Topic：

```text
car/{device_id}/status
```

Will payload：

```json
{
  "schema_version": 1,
  "device_id": "car-001",
  "online": false,
  "reason": "connection_lost"
}
```

Retain：

```text
true
```

这样树莓派异常掉线后 Broker 可以自动发布离线状态。

---

# 15. MQTT Commands

Topic：

```text
car/{device_id}/commands
```

方向：

```text
Platform -> Raspberry Pi
```

消息通用结构：

```json
{
  "schema_version": 1,

  "command_id": "cmd-001",

  "device_id": "car-001",

  "type": "buzzer.test",

  "issued_at": "2026-09-29T10:30:00Z",

  "expires_at": "2026-09-29T10:30:10Z",

  "params": {}
}
```

---

# 16. 命令类型

当前 Go 控制闭环实现 `buzzer.test`、`indicator.test`、`scenario.start`、`scenario.stop`，统一由平台签发 10 秒有效期。下面的摄像头和配置约定属于后续能力；当前普通 commands API 对未支持类型返回 400/40001，配置使用独立 config Topic。

第一版：

```text
buzzer.test

indicator.test

camera.snapshot

scenario.start

scenario.stop

config.update
```

---

# 17. buzzer.test

请求：

```json
{
  "schema_version": 1,
  "command_id": "cmd-001",
  "device_id": "car-001",
  "type": "buzzer.test",
  "issued_at": "2026-09-29T10:30:00Z",
  "expires_at": "2026-09-29T10:30:10Z",

  "params": {
    "duration_ms": 1000
  }
}
```

限制：

```text
100 <= duration_ms <= 5000
```

---

# 18. indicator.test

```json
{
  "schema_version": 1,
  "command_id": "cmd-002",
  "device_id": "car-001",
  "type": "indicator.test",

  "issued_at": "2026-09-29T10:30:00Z",
  "expires_at": "2026-09-29T10:30:10Z",

  "params": {
    "color": "red",
    "duration_ms": 2000
  }
}
```

color：

```text
green
yellow
red
```

---

`indicator.test` 的 `duration_ms` 与蜂鸣器相同：整数，100—5000ms；color 只能是 green/yellow/red。

# 19. camera.snapshot

```json
{
  "schema_version": 1,
  "command_id": "cmd-003",
  "device_id": "car-001",
  "type": "camera.snapshot",

  "issued_at": "2026-09-29T10:30:00Z",
  "expires_at": "2026-09-29T10:30:10Z",

  "params": {
    "quality": 80
  }
}
```

执行后树莓派：

```text
读取最新摄像头 Frame
↓
编码 JPEG
↓
HTTP 上传
↓
Command Ack 返回 snapshot_id
```

---

# 20. scenario.start

```json
{
  "schema_version": 1,
  "command_id": "cmd-004",
  "device_id": "car-001",
  "type": "scenario.start",

  "issued_at": "2026-09-29T10:30:00Z",
  "expires_at": "2026-09-29T10:30:10Z",

  "params": {
    "scenario": "obstacle_approach",
    "speed": 1.0
  }
}
```

scenario：

```text
normal_drive

obstacle_approach

lane_departure

sharp_turn

high_temperature

sensor_failure
```

speed：

```text
0.5
1.0
2.0
```

代表场景回放速度。

---

# 21. scenario.stop

```json
{
  "schema_version": 1,
  "command_id": "cmd-005",
  "device_id": "car-001",
  "type": "scenario.stop",

  "issued_at": "2026-09-29T10:30:00Z",
  "expires_at": "2026-09-29T10:30:10Z",

  "params": {}
}
```

---

# 22. config.update

推荐配置更新走 MQTT 独立 Topic：

```text
car/{device_id}/config
```

而不是普通 command。

消息：

```json
{
  "schema_version": 1,

  "device_id": "car-001",

  "config_version": 6,

  "issued_at": "2026-09-29T10:30:00Z",

  "rules": {
    "obstacle": {
      "warning_distance_m": 20,
      "danger_distance_m": 8
    },

    "lane_departure": {
      "offset_threshold": 0.35,
      "duration_ms": 1000
    },

    "sharp_turn": {
      "yaw_threshold_deg_s": 30,
      "min_speed_kmh": 20,
      "duration_ms": 300
    },

    "temperature": {
      "trigger_c": 35,
      "recover_c": 33,
      "duration_ms": 5000
    }
  }
}
```

Retain：

```text
true
```

这样新上线的设备可以收到最新配置。

---

# 23. Command Ack

Topic：

```text
car/{device_id}/command-acks
```

方向：

```text
Raspberry Pi -> Platform
```

成功：

```json
{
  "schema_version": 1,

  "command_id": "cmd-001",

  "device_id": "car-001",

  "status": "success",

  "executed_at": "2026-09-29T10:30:01Z",

  "result": {
    "message": "buzzer test completed"
  }
}
```

---

# 24. Command Ack 失败

```json
{
  "schema_version": 1,

  "command_id": "cmd-001",

  "device_id": "car-001",

  "status": "failed",

  "executed_at": "2026-09-29T10:30:01Z",

  "error": {
    "code": "COMMAND_EXPIRED",
    "message": "command has expired"
  }
}
```

---

# 25. Command 状态

平台内部统一使用：

```text
pending

sent

success

failed

timeout
```

流程：

```text
HTTP 请求
↓
pending
↓
MQTT publish 成功
↓
sent
↓
等待 ack
├── success
├── failed
└── timeout
```

注意：

```text
MQTT publish 成功
```

不代表：

```text
设备已经执行成功
```

---

当前 Go 平台的状态补充规则：

- HTTP 202/pending 表示命令已保存并异步受理，发布成功才进入 sent，合法设备回执才决定 success/failed。
- 明确无法提交的发送错误写 failed/MQTT_UNAVAILABLE；发布结果不确定则等待回执或截止，不据此声称设备未执行。
- 平台以收到有效回执的时间和 expires_at 比较；相等或晚于截止为 timeout。timeout 表示未及时确认。
- 快速 Ack 可从 pending 直接进入终态，后续发布回调不能覆盖回 sent。重复/冲突/迟到 Ack 不改终态、不重复推送。
- 平台重启恢复未结束命令的截止时间，不自动重发动作命令。每次有效 POST 是新命令，本版没有 HTTP 幂等键。

# 26. 命令错误码

建议：

```text
COMMAND_EXPIRED

COMMAND_DUPLICATED

UNSUPPORTED_COMMAND

INVALID_PARAMS

DEVICE_BUSY

DEVICE_NOT_SUPPORTED

SENSOR_UNAVAILABLE

ACTUATOR_UNAVAILABLE

INTERNAL_ERROR
```

---

# 27. 配置回执

Topic：

```text
car/{device_id}/config-acks
```

消息：

```json
{
  "schema_version": 1,

  "device_id": "car-001",

  "config_version": 6,

  "status": "success",

  "applied_at": "2026-09-29T10:30:01Z"
}
```

失败：

```json
{
  "schema_version": 1,

  "device_id": "car-001",

  "config_version": 6,

  "status": "failed",

  "error": {
    "code": "INVALID_CONFIG",
    "message": "danger_distance_m must be less than warning_distance_m"
  }
}
```

---

# 28. HTTP API 总览

Base URL：

```text
/api/v1
```

---

# 29. 设备 API

## GET /api/v1/devices

获取所有设备。

Response：

```json
{
  "code": 0,

  "data": [
    {
      "device_id": "car-001",
      "name": "Demo Car",
      "online": true,
      "last_seen_at": "2026-09-29T10:30:00Z",
      "config_version": 5
    }
  ]
}
```

---

# 30. GET /api/v1/devices/{device_id}

获取设备详情。

Response：

```json
{
  "code": 0,

  "data": {
    "device_id": "car-001",

    "name": "Demo Car",

    "online": true,

    "mode": "mock",

    "software_version": "0.1.0",

    "config_version": 5,

    "last_seen_at": "2026-09-29T10:30:00Z",

    "sensors": {
      "distance": "ok",
      "camera": "ok",
      "gyro": "ok",
      "temperature": "ok"
    }
  }
}
```

---

# 31. GET /api/v1/devices/{device_id}/state

获取最新状态。

Response：

```json
{
  "code": 0,

  "data": {
    "device_id": "car-001",

    "timestamp": "2026-09-29T10:30:00Z",

    "speed_kmh": 32.4,

    "distance_m": 18.0,

    "temperature_c": 27.5,

    "yaw_rate_deg_s": 12.8,

    "lane": {
      "valid": true,
      "offset_ratio": 0.18,
      "direction": "right"
    },

    "risk_level": "normal"
  }
}
```

---

# 32. GET /api/v1/devices/{device_id}/telemetry

历史遥测。

Query：

```text
start
end
limit
```

例如：

```text
GET /api/v1/devices/car-001/telemetry?limit=100
```

Response：

```json
{
  "code": 0,

  "data": {
    "items": [
      {
        "timestamp": "2026-09-29T10:30:00Z",
        "speed_kmh": 32.4,
        "distance_m": 18.0,
        "temperature_c": 27.5,
        "yaw_rate_deg_s": 12.8,
        "lane_offset_ratio": 0.18,
        "risk_level": "normal"
      }
    ]
  }
}
```

---

# 33. GET /api/v1/devices/{device_id}/events

Query：

```text
type
level
start
end
limit
```

例如：

```text
GET /api/v1/devices/car-001/events?type=obstacle_warning&limit=20
```

---

# 34. Event Response

```json
{
  "code": 0,

  "data": {
    "items": [
      {
        "event_id": "evt-001",
        "type": "obstacle_warning",
        "level": "danger",
        "action": "started",
        "timestamp": "2026-09-29T10:30:12Z",

        "data": {
          "distance_m": 6.8,
          "speed_kmh": 32
        },

        "snapshot_id": "snap-001"
      }
    ]
  }
}
```

---

# 35. GET /api/v1/devices/{device_id}/config

返回完整期望规则和当前期望版本的同步结果，HTTP 200/code 0。未知设备为 404/40401。

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "desired_version": 6,
    "reported_version": 6,
    "rules": {
      "obstacle": {"warning_distance_m": 20, "danger_distance_m": 8},
      "lane_departure": {"offset_threshold": 0.35, "duration_ms": 1000},
      "sharp_turn": {"yaw_threshold_deg_s": 30, "min_speed_kmh": 20, "duration_ms": 300},
      "temperature": {"trigger_c": 35, "recover_c": 33, "duration_ms": 5000}
    },
    "status": "success",
    "ack_at": "2026-09-29T10:30:01.100Z",
    "applied_at": "2026-09-29T10:30:01Z",
    "error": null
  }
}
```

- desired_version 表示平台期望版本；reported_version 只由合法成功 config-ack 单调前进。设备 API 的 config_version 仍来自 status 报告，不能代替配置 Ack。
- status 为当前 desired_version 的 pending/success/failed。旧版本成功不把最新版本标为 success；失败保留上一成功版本。
- ack_at 是平台接收首个有效回执的 UTC 时间；applied_at 只有 success 时有值，error 只有 failed 时有值。其余情况返回 null。
- 初始种子 desired_version=1、reported_version=0，未收到成功回执时不得宣称生效。配置可长期 pending，不采用动作命令的 10 秒截止。

---

# 36. PUT /api/v1/devices/{device_id}/config

提交刚读取的 desired_version 作为 expected_version。支持部分规则字段，未提交字段保留原值；合并后严格校验完整配置。

```json
{
  "expected_version": 6,
  "rules": {"obstacle": {"warning_distance_m": 20}}
}
```

只允许 expected_version/rules。拒绝 null、空 rules、空规则子对象、未知字段、尾随 JSON 和类型错误。非法字段返回 HTTP 400/code 40001，error.field 标明如 rules.obstacle.danger_distance_m 的路径。过期 expected_version 返回 HTTP 409/code 40902；同旧版本并发更新只有一次成功。

规则校验：0 < danger_distance_m < warning_distance_m；0 < offset_threshold <= 1；yaw_threshold_deg_s > 0；min_speed_kmh >= 0；recover_c < trigger_c。所有 duration_ms 为正整数，数值有限并能由服务端类型表示；温度允许为负数。

平台执行：

```text
核对 expected_version，合并和校验规则
↓
事务内 version + 1，保存完整期望配置与 pending 版本记录
↓
提交后返回 HTTP 202/pending
↓
异步 QoS 1 retained 发布 config
↓
等待 config-ack；通过 GET 查询结果
```

HTTP 202 Response：

```json
{"code":0,"message":"ok","data":{"config_version":7,"status":"pending"}}
```

- 每个有效 PUT 创建新版本，包括提交同值的请求；没有 HTTP 幂等键。
- 已登记设备离线仍可受理，供上线时读取 retained 最新规则；MQTT 不可用在写入前返回 503/50302。未知设备为 404/40401；数据库故障为 500/50001，且不得先发布再落库。
- 受理后的发布失败或交付不确定保持 pending，记录诊断，不制造设备失败。只在合法 success/failed config-ack 到达后保存设备结果；每版本首个有效终态不被重复、冲突或迟到发布回调覆盖。
- 启动和 Broker 重连先恢复 telemetry/status/command-acks/events/config-acks，再从 SQLite 补发各设备最新期望规则，含已完成版本，用于重建 Broker retained 数据。版本、规则和 issued_at 不变；不重发动作命令。
- 配置使用专用版本记录校验 Ack 归属。非法身份/协议/结果结构、未知/未来/跨设备版本不更新；旧成功版本最多补充确认事实，不回退 reported_version 或改写新版本结果。
- 本轮通过 GET 查询同步结果，不新增 WS config_update。

---

# 37. POST /api/v1/devices/{device_id}/commands

通用下行命令 API。

成功受理返回 HTTP **202 Accepted**，data.status 固定为 pending。平台先落库再异步下发；查询时可能已经进入 sent 或终态。请求只允许 type/params，拒绝额外字段、缺失或非法参数和尾随 JSON。设备不存在为 404/40401，离线为 503/50301，发布通道不可用为 503/50302；前置拒绝不生成记录。

Request：

```json
{
  "type": "buzzer.test",

  "params": {
    "duration_ms": 1000
  }
}
```

平台自动生成：

```text
command_id
issued_at
expires_at
```

Response：

```json
{
  "code": 0,

  "data": {
    "command_id": "cmd-001",
    "status": "pending"
  }
}
```

---

# 38. POST scenario.start

Request：

```json
{
  "type": "scenario.start",

  "params": {
    "scenario": "obstacle_approach",
    "speed": 1.0
  }
}
```

---

# 39. POST camera.snapshot

```json
{
  "type": "camera.snapshot",

  "params": {
    "quality": 80
  }
}
```

---

# 40. GET /api/v1/commands/{command_id}

Response：

```json
{
  "code": 0,

  "data": {
    "command_id": "cmd-001",

    "device_id": "car-001",

    "type": "buzzer.test",

    "status": "success",

    "issued_at": "2026-09-29T10:30:00Z",

    "expires_at": "2026-09-29T10:30:10Z",

    "ack_at": "2026-09-29T10:30:01Z",

    "executed_at": "2026-09-29T10:30:01Z",

    "result": {
      "message": "buzzer test completed"
    },

    "error": null
  }
}
```

---

命令查询还包含 params。ack_at 为平台收到回执的时间，executed_at 为设备报告的执行时间；未收到回执时两者为 null。result/error 无值为 null；不存在的命令返回 404/40402。WS command_update 的 data 与命令查询 DTO 相同。

# 41. 图片上传 API

## POST /api/v1/devices/{device_id}/frames

方向：

```text
Raspberry Pi -> Go
```

Content-Type：

```text
multipart/form-data
```

字段：

```text
file
frame_id
captured_at
type
```

type：

```text
preview
snapshot
event
```

---

# 42. 图片上传 Response

```json
{
  "code": 0,

  "data": {
    "snapshot_id": "snap-001",

    "device_id": "car-001",

    "type": "snapshot",

    "captured_at": "2026-09-29T10:30:00Z"
  }
}
```

---

# 43. GET /api/v1/devices/{device_id}/frame

用于获取最新预览。

可以返回：

```text
image/jpeg
```

或者：

```json
{
  "code": 0,

  "data": {
    "url": "/api/v1/snapshots/snap-001/content"
  }
}
```

推荐第二种，方便前端缓存和扩展。

---

# 44. GET /api/v1/snapshots/{snapshot_id}/content

直接返回：

```text
Content-Type: image/jpeg
```

---

# 45. WebSocket

连接：

```text
GET /api/v1/ws
```

前端连接后接收平台推送。

---

# 46. WebSocket 通用格式

```json
{
  "type": "telemetry",

  "timestamp": "2026-09-29T10:30:00Z",

  "data": {}
}
```

type 第一版：

```text
telemetry

event

device_status

command_update

config_update

frame_update
```

---

# 47. WebSocket telemetry

```json
{
  "type": "telemetry",

  "timestamp": "2026-09-29T10:30:00Z",

  "data": {
    "device_id": "car-001",

    "speed_kmh": 32.4,

    "distance_m": 18.0,

    "temperature_c": 27.5,

    "yaw_rate_deg_s": 12.8,

    "lane": {
      "valid": true,
      "offset_ratio": 0.18,
      "direction": "right"
    },

    "risk_level": "normal"
  }
}
```

---

# 48. WebSocket event

```json
{
  "type": "event",

  "timestamp": "2026-09-29T10:30:12Z",

  "data": {
    "event_id": "evt-001",
    "device_id": "car-001",
    "event_type": "obstacle_warning",
    "action": "started",
    "level": "danger"
  }
}
```

---

# 49. WebSocket command_update

```json
{
  "type": "command_update",

  "timestamp": "2026-09-29T10:30:01Z",

  "data": {
    "command_id": "cmd-001",
    "device_id": "car-001",
    "status": "success"
  }
}
```

---

# 50. WebSocket device_status

```json
{
  "type": "device_status",

  "timestamp": "2026-09-29T10:30:01Z",

  "data": {
    "device_id": "car-001",
    "online": false
  }
}
```

---

# 51. WebSocket frame_update

```json
{
  "type": "frame_update",

  "timestamp": "2026-09-29T10:30:01Z",

  "data": {
    "device_id": "car-001",
    "snapshot_id": "snap-001",
    "type": "preview"
  }
}
```

前端收到后再更新：

```text
/api/v1/snapshots/snap-001/content
```

避免直接把大图片塞进 WebSocket JSON。

---

# 52. HTTP 通用响应

成功：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

失败：

```json
{
  "code": 40001,

  "message": "invalid parameter",

  "error": {
    "field": "duration_ms"
  }
}
```

---

# 53. HTTP 状态码

建议：

```text
200 OK

201 Created

400 Bad Request

404 Not Found

409 Conflict

422 Unprocessable Entity

500 Internal Server Error

503 Service Unavailable
```

---

# 54. 业务错误码

建议：

```text
0
OK

40001
INVALID_PARAMS

40401
DEVICE_NOT_FOUND

40402
COMMAND_NOT_FOUND

40901
COMMAND_DUPLICATED

40902
CONFIG_VERSION_CONFLICT

50301
DEVICE_OFFLINE

50302
MQTT_UNAVAILABLE

50001
INTERNAL_ERROR
```

---

# 55. 设备离线时下发命令

第一版建议：

```text
动作命令：
设备离线直接拒绝
```

例如：

```text
buzzer.test

camera.snapshot

scenario.start
```

HTTP：

```text
503
```

业务码：

```text
50301 DEVICE_OFFLINE
```

而配置：

```text
config.update
```

可以允许保存。

等设备重新上线后通过 retained config 获取最新配置。

---

# 56. 数据新鲜度

Go 平台不能只看最后一条数据值。

还要判断：

```text
sample_at
```

例如：

```text
超声波超过 1 秒未更新
```

应认为：

```text
distance stale
```

温度可以允许：

```text
5 秒
```

摄像头检测结果可以：

```text
1 秒
```

---

# 57. 前端显示规则

前端必须区分：

```text
正常

告警

数据过期

传感器故障

设备离线
```

例如距离显示：

```text
18.0 m
```

如果数据超过 TTL：

```text
-- / 数据过期
```

不能继续显示旧的：

```text
18.0m
```

并误导用户认为它是当前值。

---

# 58. 消息顺序

Telemetry 中加入：

```text
seq
```

例如：

```text
1024
1025
1026
```

平台收到：

```text
1026
```

以后又收到：

```text
1025
```

可以判断是旧消息。

第一版可以：

```text
数据库允许写历史
但 LatestState 不回退
```

---

# 59. Event 幂等

平台数据库：

```text
event_id UNIQUE
```

如果树莓派断网补传造成重复：

```text
INSERT
```

发生唯一键冲突：

```text
视为已经处理
```

避免重复 Event。

---

# 60. Command 幂等

树莓派：

```text
command_id
```

作为幂等键。

缓存：

```text
command_id
status
result
executed_at
```

重复命令：

```text
不重新执行
```

直接重发 Ack。

---

# 61. 安全建议

课设阶段最低实现：

```text
MQTT username/password

HTTP 基本鉴权

设备只能访问自己的 Topic
```

Mosquitto ACL：

```text
car-001
```

只允许：

```text
publish:
car/car-001/telemetry
car/car-001/events
car/car-001/status
car/car-001/command-acks

subscribe:
car/car-001/commands
car/car-001/config
```

---

# 62. 平台 MQTT 权限

Go 平台允许：

```text
subscribe:
car/+/telemetry
car/+/events
car/+/status
car/+/command-acks
car/+/config-acks

publish:
car/+/commands
car/+/config
```

---

# 63. Preview 图片频率

推荐：

```text
OpenCV 本地处理：
5~10 FPS
```

但 HTTP Preview：

```text
1~2 FPS
```

分辨率：

```text
640x360
或
640x480
```

JPEG Quality：

```text
60~80
```

---

# 64. 数据频率建议

| 数据 | 采集频率 | MQTT 上报 |
|---|---:|---:|
| Distance | 10 Hz | 2 Hz |
| Gyro | 50 Hz | 2 Hz |
| Temperature | 1 Hz | 1~2 Hz |
| Lane Result | 5~10 Hz | 2 Hz |
| Speed | 10 Hz 或场景更新 | 2 Hz |
| Status | 状态变化 + 10s 心跳 | 0.1 Hz |
| Preview | 5~10 FPS 本地 | 1~2 FPS HTTP |

---

# 65. API 调用示例：启动障碍物场景

前端：

```http
POST /api/v1/devices/car-001/commands
Content-Type: application/json
```

Body：

```json
{
  "type": "scenario.start",
  "params": {
    "scenario": "obstacle_approach",
    "speed": 1.0
  }
}
```

Go：

```text
生成 cmd-001
↓
保存 commands
↓
MQTT publish
```

MQTT：

```text
car/car-001/commands
```

Payload：

```json
{
  "command_id": "cmd-001",
  "device_id": "car-001",
  "type": "scenario.start",
  "params": {
    "scenario": "obstacle_approach",
    "speed": 1.0
  }
}
```

Pi：

```text
ScenarioEngine.Start()
```

Ack：

```json
{
  "command_id": "cmd-001",
  "status": "success"
}
```

前端 WebSocket：

```json
{
  "type": "command_update",
  "data": {
    "command_id": "cmd-001",
    "status": "success"
  }
}
```

---

# 66. API 调用示例：修改阈值

前端：

```http
PUT /api/v1/devices/car-001/config
```

Body：

```json
{
  "expected_version": 6,
  "rules": {
    "obstacle": {
      "warning_distance_m": 20,
      "danger_distance_m": 8
    }
  }
}
```

Go：

```text
GET 读取 desired_version，并提交 expected_version
↓
事务内核对条件版本，合并规则并 version + 1
↓
数据库保存 desired config
↓
publish retained config
```

Pi：

```text
收到 config
↓
校验
↓
写本地配置
↓
更新 Rule Engine
↓
返回 config ack
```

---

# 67. API 调用示例：摄像头拍照

Vue：

```http
POST /api/v1/devices/car-001/commands
```

```json
{
  "type": "camera.snapshot",
  "params": {
    "quality": 80
  }
}
```

Pi 收到后：

```text
CameraSource.read()
↓
JPEG encode
↓
POST /frames
```

Frame API：

```http
POST /api/v1/devices/car-001/frames
```

然后 Ack：

```json
{
  "command_id": "cmd-snapshot-001",
  "status": "success",

  "result": {
    "snapshot_id": "snap-001"
  }
}
```

---

# 68. 推荐数据库映射

## devices

```text
device_id UNIQUE
name
online
last_seen_at
software_version
reported_config_version
```

## telemetry

```text
id
device_id
timestamp
seq
speed
distance
temperature
yaw_rate
lane_offset
risk_level
```

## events

```text
event_id UNIQUE
device_id
type
action
level
payload
timestamp
snapshot_id
```

## commands

```text
command_id UNIQUE
device_id
type
params
status
issued_at
expires_at
ack_at
executed_at
result
error
```

## device_configs

```text
device_id UNIQUE
desired_version
reported_version
config_json
updated_at
```

## config_revisions

```text
(device_id, config_version) UNIQUE
config_json  // 不可变的完整签发规则
issued_at
status       // pending/success/failed，首个有效回执终态
ack_at       // 平台时间，可空
applied_at   // 设备成功时间，可空
error_json   // 设备失败，可空
```

## snapshots

```text
snapshot_id UNIQUE
device_id
type
path
captured_at
created_at
```

---

# 69. 最小联调顺序

建议严格按以下顺序：

```text
1. Pi -> MQTT -> Go telemetry

2. Go -> WebSocket -> Vue

3. Vue -> HTTP -> Go command

4. Go -> MQTT -> Pi

5. Pi -> Ack -> Go -> Vue

6. Event 上报

7. Config 下发

8. Camera Frame HTTP 上传
```

不要第一天就同时调所有模块。

---

# 70. 第一版必须保证的闭环

最低要求：

```text
Pi 模拟温度
↓
MQTT telemetry
↓
Go
↓
Vue 显示
```

以及：

```text
Vue 点击蜂鸣器测试
↓
HTTP
↓
Go
↓
MQTT
↓
Pi
↓
执行
↓
Ack
↓
Go
↓
Vue 显示 success
```

如果这两个闭环跑通，整个系统的网络架构基本成立。

---

# 71. 推荐最终网络架构总结

```text
                   ┌────────────────────┐
                   │      Vue Web       │
                   └───────┬────────────┘
                           │
             REST + WebSocket
                           │
                           ▼
                   ┌────────────────────┐
                   │     Go Backend     │
                   │                    │
                   │ REST API           │
                   │ WebSocket          │
                   │ MQTT Client        │
                   │ SQLite             │
                   └───────┬────────────┘
                           │
                         MQTT
                           │
                           ▼
                   ┌────────────────────┐
                   │     Mosquitto      │
                   └───────┬────────────┘
                           │
                         MQTT
                           │
                           ▼
┌──────────────────────────────────────────────────┐
│               Raspberry Pi 4B                   │
│                                                  │
│ Sensors -> Processing -> Rules -> Actuator      │
│                    │                             │
│                    ├── Telemetry -> MQTT         │
│                    ├── Event -> MQTT             │
│                    ├── Status -> MQTT            │
│                    │                             │
│ MQTT Command ------┤                             │
│                    ↓                             │
│              Command Handler                     │
│                    │                             │
│                    └── Ack -> MQTT               │
│                                                  │
│ Camera Frame ---------------------- HTTP -------> │
└──────────────────────────────────────────────────┘
```

---

# 72. 结论

本规范将系统网络交互统一为：

```text
MQTT：
设备实时数据与双向控制

HTTP：
平台 API 与图片上传

WebSocket：
前端实时推送
```

核心约束：

```text
所有命令都有 command_id

所有告警都有 event_id

所有实时数据都有 timestamp

Telemetry 使用 seq 防止状态回退

QoS 1 消息必须支持幂等

动作命令必须有 expires_at

配置和设备实际状态分离

平台不能把 MQTT publish 成功视为命令执行成功

摄像头图片不通过 MQTT 高频传输
```

按照这份协议定义后，Raspberry Pi、Go 后端和 Vue 前端可以分别独立开发，并通过统一 JSON 数据结构完成联调。
