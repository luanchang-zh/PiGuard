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

2026-10-22 兼容扩展：`lane` 可增加 `source` 和 `status`，枚举分别与其他采样对象一致，用于区分电脑/真实摄像头、合成图像以及采集故障。新增边缘端同时发送这两个字段；后端继续接受二者都缺省的旧报文，并将元数据传入 `state.samples.lane` 与 WebSocket 状态视图。`valid` 仍独立表示车道检测是否有效：摄像头采集正常但画面无标线时为 `status=ok, valid=false`。

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

可选 `snapshot_id` 或 `frame_id` 二者互斥，也可都省略。snapshot_id 为 1—64 非空白字符；frame_id 为 1—64 ASCII 字母/数字/下划线/连字符，等于同设备上传身份，用于先告警后图片。显式 null、类型错误、非法身份、同时给两种引用或未知字段均拒绝。设备 snapshot_available 只允许 bool，是兼容输入，不作为平台可读事实，也不保存或参与幂等。

每条 event_id 全局唯一，保存原始 snapshot_id/frame_id；重复按设备、类型、动作、等级、时间、data JSON 语义和原引用比较，相同忽略，冲突保留首条。图片未知或引用错误仍保存合法告警；解析关联不改写原引用，不自动从 telemetry.risk 或图片上传创建告警。

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

当前 Go commands API 支持 `buzzer.test`、`indicator.test`、`camera.snapshot`、`scenario.start`、`scenario.stop`，统一由平台签发 10 秒有效期。未知类型返回 400/40001。配置已使用独立 HTTP config 接口和 config/config-acks Topic，不通过普通 commands API；拍照代码已交付，运行验收后置。

commands 类型：

```text
buzzer.test

indicator.test

camera.snapshot

scenario.start

scenario.stop
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

params 必须为对象，quality 可省略，缺省 80；显式 quality 为 1—100 整数。null、非整数、越界或额外字段拒绝；平台在持久化/下发时补齐缺省 quality。截止沿用 issued_at+10 秒，QoS 1、retained=false。

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

成功 result 严格为仅含非空 snapshot_id 的对象。平台确认该 ID 已提交、属于命令设备、type=snapshot 且文件可打开后，才接受成功回执。未知/跨设备/preview/event/缺文件或非法 result 不改状态、不推送，继续等待合法 Ack 或到期 timeout；读取/DB 故障不能算有效图片。failed 回执沿用 error.code/message，软件上传故障用 SNAPSHOT_UPLOAD_FAILED。

只有平台在有效处理时的接收时间早于 expires_at 才准时；文件校验后重新检查截止，不能用设备 captured_at/executed_at 延长截止。首个终态保持不变，重复/冲突/迟到回执不改写。已上传图片不因命令失败/超时回滚，不要求设备采集时间窗口或一图只能被一命令引用。

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

type/level/start/end 同时筛选，时间含端点，limit 默认 100、范围 1—1000；按 timestamp DESC/event_id DESC 取窗口内最新记录。无记录 items=[]；未知设备 404/40401，非法参数 400/40001，DB/其他存储故障 500/50001。

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

        "frame_id": "alarm-frame-001",
        "snapshot_id": "snap-001",
        "snapshot_available": true,
        "snapshot_url": "/api/v1/snapshots/snap-001/content",
        "snapshot_state": "available"
      }
    ]
  }
}
```

---

HTTP items、WS event 和 event_image_update 使用一致的图片字段。frame_id 为原输入或 null；snapshot_id 为平台校验后的有效 ID，仅同设备、type=snapshot/event、已提交且安全普通文件可打开时提供。snapshot_url 同样只在可读时提供稳定内容 URL，其余为 null；snapshot_available 是平台计算 bool。

snapshot_state 为 none（无引用）、pending（尚无已提交记录）、available（可读）、invalid（跨设备/preview 类型）、missing（有记录但缺文件）、unavailable（通知时暂时读取故障）。GET 遇到 DB/其他存储故障返回 500，不当作 pending。新告警保存后解析失败仍发 event，标记 unavailable 并记录诊断。每次 GET 重新解析持久原引用/元数据/文件，重启后仍可查询，外部删图反映 missing；不依赖内存关联表。

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

沿用命令创建 API 的在线、MQTT 可用性及 HTTP 202/pending 受理规则。`{"type":"camera.snapshot","params":{}}` 与 quality=80 等效；没有 HTTP 幂等键，每次合法 POST 创建新命令。

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

camera.snapshot 成功查询/command_update 的 result 为 `{"snapshot_id":"snap-001","url":"/api/v1/snapshots/snap-001/content"}`，URL 由平台派生，原始设备 Ack 只含 snapshot_id，供语义重复判断。之后图片缺失不会改写历史成功，由内容 API 如实返回 404。frame_update 随图片提交产生，command_update 随有效命令状态改变产生，上传图片本身不结束命令。

# 41. 图片上传 API

## POST /api/v1/devices/{device_id}/frames

方向：Raspberry Pi → Go，Content-Type 为 multipart/form-data。正文严格包含一个 file、frame_id、captured_at、type；未知、重复或缺失字段、多文件和错误 multipart 返回 400/40001，并指出 error.field。

| 字段 | 契约 |
| --- | --- |
| file | 非空有效 JPEG，以实际字节、尺寸和完整解码判断，保留原始字节；文件名和 MIME 声明不影响有效性 |
| frame_id | 设备侧 1—64 字符标识，仅 ASCII 字母、数字、下划线、连字符 |
| captured_at | 带时区 RFC3339/RFC3339Nano，归一 UTC；不额外限制设备时钟偏移 |
| type | preview / snapshot / event |

JPEG ≤2 MiB（2,097,152 字节），总 HTTP 正文 ≤2 MiB+64 KiB，单边 ≤4096，总像素 ≤16,777,216。字节、正文或尺寸超限返回 413/40001；无效、损坏或截断 JPEG 返回 400/40001。拒绝请求不会落库或推送。

只接受已登记设备，未知设备 404/40401。设备离线或运行中 MQTT 暂时断线不额外拒绝图片；上传不改变 online、last_seen_at、命令或配置版本。初次启动仍要求 Broker 连接成功。

---

# 42. 图片上传 Response

新上传返回 HTTP 201/code 0：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "snapshot_id": "snap-001",
    "device_id": "car-001",
    "type": "snapshot",
    "captured_at": "2026-09-29T10:30:00Z"
  }
}
```

snapshot_id 由服务端生成，稳定且不可变，不暴露数据库 ID。以 (device_id, frame_id) 为幂等身份；相同归一后的采集时间、类型、原始字节重试返回 HTTP 200/code 0 和原 ID，不重复保存或通知；内容或元数据冲突返回 HTTP 409/40903，并保留首个结果。跨设备同 frame_id 独立，并发重试只有一份成功持久记录。

文件位于配置截图目录，SQLite 只保存元数据。文件写入、持久化与最终发布完成后才提交可查询记录；提交前写入/数据库失败或取消会回收本请求的临时与未提交文件，不推进预览或通知。提交成功后 HTTP 断连不撤销图片，同身份重试可找回结果。重启保留已提交图片；临时/未引用文件不作为有效图片，不自动清理用户文件或历史图片。

---

# 43. GET /api/v1/devices/{device_id}/frame

返回最新 preview 的 JSON；snapshot/event 不覆盖预览：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "snapshot_id": "snap-001",
    "type": "preview",
    "captured_at": "2026-09-29T10:30:00Z",
    "url": "/api/v1/snapshots/snap-001/content"
  }
}
```

按 captured_at 选择最新，同时间按平台成功提交顺序。迟到旧图保留独立 ID 和可读内容，但不覆盖当前 preview 或发送 frame_update。已登记但无 preview 返回 404/40403；未知设备返回 404/40401。URL 对应不可变图片，不是可变的“当前文件”。

---

# 44. GET /api/v1/snapshots/{snapshot_id}/content

返回 HTTP 200、Content-Type: image/jpeg 与保存的原始字节。未知 ID 或已登记但文件缺失返回 404/40403；数据库或其他读取错误返回 500/50001。不接受任意文件路径，不使用客户端文件名/frame_id 构造路径，不泄露服务端目录。

本轮不提供 PNG、视频、缩略图、重压缩、历史分页、图片删除或 TTL 清理。图片持久保留。camera.snapshot 通过有效 Ack 关联已提交 snapshot；告警按同设备原 snapshot_id/frame_id 解析可读的 snapshot/event 图片，新图片提交后通过 event_image_update 更新已有告警（见第 48 节）。上传本身不创建命令结果或新告警。图片普通文件同步、不可覆盖发布保留；Linux 另外同步目录，Windows 避免不支持的目录 Sync。当前交付代码与编译，双平台及完整链路运行验证后置。

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

event_image_update

device_status

command_update

frame_update
```

配置同步状态通过 GET config 查询，当前没有 config_update WS 类型。

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
    "level": "danger",
    "timestamp": "2026-09-29T10:30:12Z",
    "data": {"distance_m": 6.8, "speed_kmh": 32},
    "frame_id": "alarm-frame-001",
    "snapshot_id": null,
    "snapshot_available": false,
    "snapshot_url": null,
    "snapshot_state": "pending"
  }
}
```

---

图片先到，首次 event 即可包含有效 ID/URL；告警先到，新图片安全提交后发送以下更新已有告警，前端按 event_id 合并图片字段，不新增另一条告警：

```json
{
  "type": "event_image_update",
  "timestamp": "2026-09-29T10:30:13Z",
  "data": {
    "event_id": "evt-001",
    "device_id": "car-001",
    "frame_id": "alarm-frame-001",
    "snapshot_id": "snap-001",
    "snapshot_available": true,
    "snapshot_url": "/api/v1/snapshots/snap-001/content",
    "snapshot_state": "available"
  }
}
```

只对新提交图按同设备原 snapshot_id/frame_id 查询相关已存告警；不重发 event，不回写引用，幂等/拒绝/冲突上传不重复通知。preview 引用可以更新为 invalid。创建和图片更新按一致次序推送，避免可读更新后收到旧 pending 创建帧；按有界批次读取，文件 I/O 不持 Monitor 锁或长 DB 事务。提交后通知故障不回滚图片/告警，由 GET 恢复；WS 不提供历史重放或持久 outbox。

软件演示命令 `go run ./cmd/simulator -duration 60s -event-image -api http://127.0.0.1:8080`：默认 -event-image=false 保留无图告警，启用后先发布两次相同、带稳定 frame_id 的事件，再异步上传 event JPEG。上传复用身份/时间/字节最多 3 次，整体 10 秒、每次 HTTP 6 秒，响应进程取消，失败日志不伪造可读。真实网络接收顺序可能变化，两种顺序均支持；遥测/配置/拍照继续运行。

本轮只做代码、gofmt、go build ./... 及独立代码核对；旧测试中直接返回未验证 snapshot_id 的断言保留，V1 按有效引用/新 DTO 更新并补测双顺序、并发/幂等/错误引用/存储故障/取消/重启/双 WS，再运行 test/vet/race。Broker/Docker/Python/Vue/真实摄像头与硬件在 V2 验证。

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
    "type": "preview",
    "captured_at": "2026-09-29T10:30:00Z",
    "url": "/api/v1/snapshots/snap-001/content"
  }
}
```

文件和元数据可读并成功提交后，新的 snapshot/event，以及成为当前 preview 的图片，发布一次通知。timestamp 为平台时间，captured_at 为设备采集时间；幂等重试、拒绝、失败和迟到旧 preview 不通知，预览通知按采集时间/同时间提交顺序保持不回退。双客户端收到一致 DTO，慢客户端沿用有界队列和关闭策略。

前端收到后读取 data.url；WS 不传 JPEG 大对象。保留 telemetry/event/device_status/command_update 原有语义，告警图片关联增加 event_image_update，不新增 config_update。

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

202 Accepted

400 Bad Request

404 Not Found

409 Conflict

413 Content Too Large

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

40101
UNAUTHORIZED

40301
FORBIDDEN

40401
DEVICE_NOT_FOUND

40402
COMMAND_NOT_FOUND

40403
SNAPSHOT_NOT_FOUND

40901
COMMAND_DUPLICATED

40902
CONFIG_VERSION_CONFLICT

40903
FRAME_IDENTITY_CONFLICT

50301
DEVICE_OFFLINE

50302
MQTT_UNAVAILABLE

50303
SESSION_CAPACITY

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

# 61. 认证接入

Go 默认开启 HTTP Basic/浏览器会话与认证 Broker，无默认密码。第 27—44 节业务 HTTP API 先检查 Origin，再认证授权；health 与合法 OPTIONS 预检公开。平台 Basic 或 Cookie 可访问业务 API/WS/图片，设备 Basic 仅可 POST 自己的 frames（用户名=device_id），不能跨设备、查询、控制、配置、读图或建立 WS；凭据不注册设备。缺失/错误/失效身份 401/40101，有效身份越权或非法 Origin/CSRF 为 403/40301；显式 Authorization 错误不能回退 Cookie。拒绝发生在数据库/文件/MQTT/Hub 副作用之前，响应 no-store；授权后保留既有业务错误和 DTO。

POST /api/v1/auth/session 仅平台 Basic，201/code0 返回 expires_at/csrf_token，设置随机 HttpOnly、host-only、Path=/api/v1、SameSite=Strict Cookie；直接 HTTPS 才 Secure。GET 返回 username/role/auth_mode 及当前有效会话字段；Basic 未携会话时到期/CSRF 为 null，不创建会话。DELETE 撤销所携会话、清 Cookie，204。设备不能使用这些接口。会话默认 30 分钟、可配置 1—1440、固定到期，最多 1024；满容量 503/50303。到期/退出/重启失效并关闭关联 WS；WS 同时最多 128，容量拒绝 503/50001。无注册/找回密码/JWT/SSO 或持久会话。

Vue 与 API 使用同 IP/域名、同 scheme、不同端口，真实 origin 通过 http.auth.allowed_origins / PIGUARD_HTTP_ALLOWED_ORIGINS 的 JSON 数组配置，默认仅后端同源。HTTP/WS 共用精确来源判断；拒绝 */null/非法/多个/未知 Origin，CLI 无 Origin 仍须认证授权，不信任任意 X-Forwarded-*。许可 CORS 回显来源、Allow-Credentials=true、正确 Vary；合法 OPTIONS 204，允许 GET/POST/PUT/DELETE 与 Authorization/Content-Type/X-CSRF-Token，不调用业务。Cookie 写请求带 X-CSRF-Token（包括命令、配置、平台上传、退出）；Basic 无需 CSRF，仍检查 Origin。浏览器 fetch 用 credentials:include，WS/图片携 Cookie，稳定 URL 不附凭据。跨站 Cookie/TLS/公网代理不在本轮。

配置/Compose 缺少平台、seed 设备凭据或非法身份/配对/TTL/origin 时在外部初始化之前失败；显式 http.auth.disabled 仅允许 loopback 开发，会话接口不注册，来源保护仍在。MQTT 默认禁匿名，平台/设备/健康身份互不冲突。具体 .env、初始化、原生 Go/Compose、curl、设备上传、浏览器登录/退出、模拟器独立 MQTT/HTTP 参数与轮换步骤见 [Go README](../go-backend/README.md)。以下请求 Body 和业务信封例子承接本节认证前置，401/403 不执行后续业务。

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
car/car-001/config-acks

subscribe:
car/car-001/commands
car/car-001/config
```

---

# 62. 平台 MQTT 权限

使用显式 user ACL，默认拒绝，不使用给所有角色生效的全局 %u 模式。平台只能读五类上行、写两类下行；设备只能写自己的五类上行、读自己的 commands/config，status 包括离线遗嘱；健康仅可读 $SYS/broker/uptime，无业务读写权限。订阅申请结果与实际消息授权分开判断，不承诺不同 Broker 相同 SUBACK 拒绝表述。认证失败不能算就绪；原 QoS、retained、五类重连订阅和 SQLite 配置补发保持。

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
Cookie: piguard_session=<平台登录Cookie，由浏览器携带>
X-CSRF-Token: <当前会话csrf_token>
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
Content-Type: application/json
Cookie: piguard_session=<平台登录Cookie，由浏览器携带>
X-CSRF-Token: <当前会话csrf_token>
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
Content-Type: application/json
Cookie: piguard_session=<平台登录Cookie，由浏览器携带>
X-CSRF-Token: <当前会话csrf_token>
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
Authorization: Basic <car-001与其HTTP密码编码，不放URL>
```

然后 Ack：

```json
{
  "schema_version": 1,
  "command_id": "cmd-snapshot-001",
  "device_id": "car-001",
  "status": "success",
  "executed_at": "2026-09-29T10:30:01Z",

  "result": {
    "snapshot_id": "snap-001"
  }
}
```

先按 Go README 导出 PIGUARD_SIM_MQTT_USERNAME/PASSWORD 与 PIGUARD_SIM_HTTP_USERNAME/PASSWORD 两组设备凭据，再使用 `go run ./cmd/simulator -duration 60s -api http://127.0.0.1:8080`，按 quality 生成 320×180 JPEG 后上传 snapshot。命令工作队列与上传独立于 MQTT 回调；同 command_id 重投递只重发缓存结果。上传响应丢失或临时故障最多 3 次同身份/时间/字节重试，受 10 秒命令截止和进程取消约束，失败返回 SNAPSHOT_UPLOAD_FAILED；failed/none 模式不上传。实际 CameraSource.read 仍由 Python/树莓派后续接入。

当前仅交付代码、编译及独立代码核对；测试/test/vet/race、完整软件链路与实际 Broker/Docker/摄像头/硬件运行验证集中在后续 V1/V2，不将示例当作已运行证据。

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
frame_id
```

events 的 snapshot_id/frame_id 保存原输入并保持不可变；同 device_id 的引用索引用于有界查询。平台输出中的有效 snapshot_id、URL 和状态每次解析，不作为消息事实回写。

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
frame_id  // 与 device_id 构成唯一身份，旧占位模型迁移行允许 NULL
type
path       // 服务端生成的根目录内相对文件名，不接受客户端路径
sha256
size
captured_at
captured_seconds     // 精确排序，覆盖 RFC3339 全年份
captured_nanosecond
created_at
```

JPEG 原始字节放在 storage.snapshot_dir，不存入 SQLite。预览按采集秒、纳秒、自增提交 ID 降序查询；平台提交与 frame_update 顺序一致。

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
