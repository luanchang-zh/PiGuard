# 告警事件上行

本 capability 定义 PiGuard 第一版告警事件的完整目标行为。用户于 2026-10-08 确认本规格和 brief 的 A1—A7，并要求按该范围实现。

## 1. 范围与协议

平台订阅 `car/+/events`，QoS 1、retained=false，把每条合法消息存成一行，并用 HTTP 查询和 WS 推送同一持久事实。

```json
{
  "schema_version": 1,
  "event_id": "evt-001",
  "device_id": "car-001",
  "type": "obstacle_warning",
  "action": "started",
  "level": "danger",
  "timestamp": "2026-10-08T12:00:00.000Z",
  "data": {"distance_m": 6.8, "speed_kmh": 32.0, "headway_s": 0.76},
  "snapshot_available": true,
  "snapshot_id": "snap-001"
}
```

`snapshot_available` 和 `snapshot_id` 都可省略。示例里的具体 ID、时间和 data 数值不是强制输入。

五类 type：`obstacle_warning`、`lane_departure`、`sharp_turn`、`high_temperature`、`sensor_failure`。

三种 action：`started`、`level_changed`、`recovered`。不接受 `updated` 或课设草案里的 `ALARM_*` 名称。

level 只允许 `warning` 和 `danger`。telemetry 的 `normal`/`unknown` 不是事件等级。危险解除用 `action=recovered` 表达，recovered 消息仍携带结束前的 warning 或 danger。

## 2. 输入校验

合法消息必须是单个 JSON 对象，且只包含下列顶层字段：

| 字段 | 规则 |
| --- | --- |
| schema_version | 整数 1 |
| event_id | 非空字符串，长度 1—64，不含空白 |
| device_id | 与主题 `car/{device_id}/events` 完全一致，长度规则沿用现有身份校验 |
| type | 五类之一 |
| action | 三种之一 |
| level | warning 或 danger |
| timestamp | 带时区的 RFC3339 时间，保存为 UTC |
| data | JSON 对象，允许 `{}`；不允许 null、数组、字符串或数字 |
| snapshot_id | 可省略；出现时规则与 event_id 相同 |
| snapshot_available | 可省略；出现时必须是布尔值 |

未知顶层字段、类型错误、尾随 JSON 和空对象都拒绝整条消息。data 内部字段不按告警类型拆成必填清单：网络规范只给出部分示例，平台原样保存数字、字符串、布尔、对象和数组，不把缺失测量补成 0，也不重算距离、时距或风险。

拒绝时记录主题和原因，不插入、不广播、不让进程退出。未知设备同样丢弃，不自动注册。

设备当前 online=false 时仍接受合法事件。事件不修改 online、last_seen_at、最新遥测或命令状态。telemetry 的 `risk.active_events` 只保留在监控视图里，不派生 events 行。

## 3. 持久化与幂等

`event_id` 全局唯一，表示一条 MQTT 消息，不表示一个会被后续消息更新的告警实例。因此：

- `started`、`level_changed` 和 `recovered` 是三行，只要 event_id 不同。
- 不维护 started_at、ended_at 或 incident_id。recovered 行的 timestamp 就是该条恢复消息的发生时间。
- 时间较早的新 event_id 仍然插入。查询排序不要求入库顺序等于发生顺序。

比较重复消息时，只看设备、type、action、level、同一 UTC 时刻、data 的 JSON 语义，以及 snapshot_id。`snapshot_available` 不参与比较，也不写入数据库。

| 到达的消息 | 结果 |
| --- | --- |
| 新的 event_id | 插入一行，并广播一次 |
| 相同 event_id，比较字段一致 | 保持原行，不广播 |
| 相同 event_id，比较字段不同 | 保持第一行，记录冲突，不广播 |
| 相同 event_id 被另一设备使用 | 按冲突处理，第二台设备不新增行 |

并发插入以数据库唯一约束决定第一行。只有实际插入成功的那次调用可以广播。写入失败时记录错误并且不广播；不把未落库的事件伪装成已保存。

现有 Paho 客户端在回调返回后会确认 QoS 1。本 capability 不改成手动确认，也不做 outbox。数据库短暂失败后的补偿是发送方重放，重放命中上述幂等规则。

payload 保存 data 的 JSON。对外 DTO 把 data 解码成对象。snapshot_id 列保存可选引用；不要求 snapshots 表里已经存在该 ID。

## 4. HTTP 查询

`GET /api/v1/devices/{device_id}/events`

查询参数只有 type、level、start、end、limit。未列出的参数忽略，不因此报错。

- 未知设备：HTTP 404/code 40401。
- type 或 level 不在允许集合、start/end 不是 RFC3339、start 晚于 end、limit 不是 1—1000 的整数：HTTP 400/code 40001，error 标明字段。
- limit 省略时为 100。
- start 和 end 按设备 timestamp 过滤，包含两端。多个条件同时生效。
- 先按 timestamp 降序、event_id 降序选取最多 limit 条，并按该顺序返回。
- 没有记录时 `items` 为 []。内部故障为 HTTP 500/code 50001。

成功信封：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "event_id": "evt-001",
        "type": "obstacle_warning",
        "action": "started",
        "level": "danger",
        "timestamp": "2026-10-08T12:00:00Z",
        "data": {"distance_m": 6.8},
        "snapshot_id": null
      }
    ]
  }
}
```

条目不包含 device_id、snapshot_available 或数据库自增 ID。不直接序列化 GORM 模型。

## 5. WebSocket

复用 `GET /api/v1/ws` 和 `{type,timestamp,data}` 信封。新插入成功后发送：

```json
{
  "type": "event",
  "timestamp": "2026-10-08T12:00:00.100Z",
  "data": {
    "event_id": "evt-001",
    "device_id": "car-001",
    "event_type": "obstacle_warning",
    "action": "started",
    "level": "danger",
    "timestamp": "2026-10-08T12:00:00Z",
    "data": {"distance_m": 6.8},
    "snapshot_id": null
  }
}
```

信封 timestamp 是平台广播时间。data.timestamp 是设备事件时间。类型字段在 WS 中保持规范原名 `event_type`，HTTP 中保持 `type`。

所有已连接客户端接收全部已登记设备的新事件，用 device_id 区分。不新增订阅过滤参数。建连不重放历史，历史以 GET 为准。

重复、冲突和非法消息不产生 event 帧。沿用 Hub 的有限队列、慢连接关闭和单写者；慢客户端不能阻塞 MQTT 回调、其他 WS 客户端、遥测或命令处理。

## 6. 订阅、重启与关闭

首次订阅必须包含 `car/+/events`（QoS 1）以及已经存在的 telemetry、status、command-acks。任一主题首次订阅失败时，接入阶段不能报告为就绪。Broker 重连后恢复这四组主题。

平台重启后，SQLite 中的事件保留并可查询。内存里没有待重放队列，重启不向 WS 补推旧事件。关闭时停止继续接收入库；已经写入的行保留。

模拟器在现有 status、telemetry 和命令回执之外，启动后至少发布一条合法 events 消息，QoS 1、retained=false；对同一 event_id 的再次发布必须发送一致内容。模拟器不负责图片，也不实现边缘规则。

## 7. 验收边界

实现需要覆盖 A1—A7：五类合法消息、非法与未知设备、离线补传、幂等和冲突、筛选排序、双 WS、snapshot_id、重启后的查询与订阅恢复，以及 Go test/race/vet。

真实 Mosquitto、Docker Compose 和硬件不在本轮通过条件内。未执行这些环境不能当成验收失败，也不能当成已经在真实链路上通过。

配置同步、图片字节、帧 API、camera.snapshot、Vue、Python 边缘端、规则引擎、设备自动注册和鉴权都不由本 capability 实现。
