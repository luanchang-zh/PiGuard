# 控制命令与设备回执

本 capability 定义 PiGuard 第一版动作命令的完整目标行为。用户于 2026-10-08 已确认完整 Shape；实现与验收进度由 Native Runtime 保存。

## 1. 范围与协议

平台提供两个 HTTP API、MQTT commands / command-acks 与 WS command_update，并持久记录命令事实。支持 buzzer.test、indicator.test、scenario.start、scenario.stop。camera.snapshot 和独立配置同步不属于本 capability 当前范围。

MQTT 下发主题 `car/{device_id}/commands`，QoS 1、retained=false：

```json
{
  "schema_version": 1,
  "command_id": "cmd-<unique-id>",
  "device_id": "car-001",
  "type": "buzzer.test",
  "issued_at": "2026-10-08T02:00:00Z",
  "expires_at": "2026-10-08T02:00:10Z",
  "params": {"duration_ms": 1000}
}
```

所有命令 ID 由平台生成，唯一且不可重用；时间为 UTC RFC3339Nano，expires_at=issued_at+10 秒。10 秒是本轮推荐统一默认，覆盖最长 5 秒的执行并给发送与回执留出余量；网络规范示例中的 5 秒不视为硬性默认。

## 2. 参数

| 类型 | 合法 params |
| --- | --- |
| buzzer.test | 必填整数 duration_ms，100≤值≤5000 |
| indicator.test | 必填 color∈green/yellow/red 与整数 duration_ms，100≤值≤5000 |
| scenario.start | 必填 scenario∈normal_drive/obstacle_approach/lane_departure/sharp_turn/high_temperature/sensor_failure；必填 speed∈0.5/1.0/2.0 |
| scenario.stop | 空对象 `{}` |

HTTP body 必须为单个 JSON 对象，只有 type/params；params 必须为对象且只包含该类型规定的字段。拒绝缺失、null、错误类型、非法值、额外字段和尾随 JSON。未知或本轮不支持的类型均返回 400/40001，不下发占位命令。

indicator duration 范围是本轮明确补充约定。平台只验证并传递场景参数；不在 Go 后端实现边缘场景播放或 RuleEngine。

## 3. HTTP 创建与拒绝

POST `/api/v1/devices/{device_id}/commands`：

1. 校验 JSON 和参数；查明已登记设备及其当前在线事实；检查 MQTT 连接和本轮命令工作过程可受理。
2. 未知设备 → HTTP 404/code 40401；已知离线 → 503/50301；在线但 MQTT 不可用 → 503/50302；非法参数 → 400/40001。前置拒绝不写 commands、不发布。
3. 生成 ID 和时间，先在 SQLite 保存完整 pending 记录，再交给进程内受控异步下发；数据库失败 → HTTP 500/code 50001，不能先发布后落库。
4. 成功受理返回 HTTP 202 和既有 code/message/data 信封：

```json
{"code":0,"message":"ok","data":{"command_id":"cmd-<unique-id>","status":"pending"}}
```

pending 是受理结果，不能承诺设备已执行。GET 可能在 POST 返回时已经读到 sent 或终态；这是异步进展，不是状态回退。

每个有效 POST 创建一条新命令。未提供客户端幂等键，不自动将重复 HTTP 请求折叠；前端不可把无响应的 POST 盲目重试等同一次动作。

受理后的 MQTT 失败不能改写已返回的 HTTP 202：通过 GET/WS 呈现后续状态。明确未发布成功的错误尝试将仍 pending 的记录改 failed，error 表示 MQTT_UNAVAILABLE（发送阶段故障）。publish 等待超时/取消导致交付不确定时，保留 pending；无有效 Ack 则到期 timeout。诊断日志说明发送不确定，不报告设备执行失败。没有合法 Ack 的 sent 只代表 Broker 接受。

不建立持久 outbox 重试体系；本轮只尝试一次应用层下发，Paho 的 QoS 1 协议重传由 command_id 幂等防护。平台重启或网络恢复不自动重发动作。

## 4. 持久状态机

commands 保存 ID、设备、类型、参数、状态、issued_at、expires_at、平台 Ack 接收时间、设备 executed_at、result/error。优先复用现有表，仅为明确新增的执行时间事实作必要模型迁移；result/error 为结构化 JSON，外部 DTO 解码输出。

| 当前状态 | 触发 | 新状态 |
| --- | --- | --- |
| pending | Broker publish 已确认且记录仍 pending | sent |
| pending / sent | 平台在截止前接受合法 success Ack | success |
| pending / sent | 平台在截止前接受合法 failed Ack | failed |
| pending | 明确发布失败且尚无合法 Ack 终态 | failed（发送故障） |
| pending / sent | 平台时间已达到 expires_at 且无有效 Ack | timeout |
| success / failed / timeout | 任何后续回调或 Ack | 保持终态 |

所有变更使用数据库当前状态条件更新。合法 Ack 可以直接结束 pending，解决极速设备 Ack 早于 publish 完成的竞争；发布完成不能覆盖已结束状态。多条并发 Ack 只允许首个有效终态成功写入。

进程内 Ack 接收处理与截止处理需使用一致的临界次序；接收时间取平台进入有效 Ack 处理时的时间，不取设备 executed_at。`received_at < expires_at` 才可用 Ack 结束，等于或晚于截止进入 timeout/迟到处理。不要让定时扫描的延迟把截止后 Ack 算成准时，也不要让扫描覆盖已在截止前成功接受的 Ack。

超时处理从数据库扫描 pending/sent，适用平台重启；正常运行的过期记录在截止后最多 1 秒内变 timeout。GET 不应长期暴露已经过期的 pending/sent，读取前确保过期事实已落库。timeout 只表示平台未及时确认，设备可能已经执行。

每次真正的状态改变才广播一次 command_update；状态写入失败不能对外广播不存在的事实。日志区分数据库故障、发送失败、发布不确定、非法/重复/冲突/迟到 Ack。

## 5. Ack 验证与幂等

订阅 `car/+/command-acks`，QoS 1；设备回执 retained=false。初次订阅必须成功，Broker 重连后恢复 telemetry、status 和 command-acks 全部主题。

成功示例：

```json
{"schema_version":1,"command_id":"cmd-<unique-id>","device_id":"car-001","status":"success","executed_at":"2026-10-08T02:00:01Z","result":{"message":"buzzer test completed"}}
```

失败示例：

```json
{"schema_version":1,"command_id":"cmd-<unique-id>","device_id":"car-001","status":"failed","executed_at":"2026-10-08T02:00:01Z","error":{"code":"ACTUATOR_UNAVAILABLE","message":"buzzer unavailable"}}
```

有效 Ack 必须满足：JSON 合法；schema_version=1；command_id/device_id 非空；主题和 payload 设备相同；命令已存在且归属同一设备；status 仅 success/failed；executed_at 是有效 UTC 时间；success 的 result 为对象，failed 的 error 有非空 code/message；不能同时给相互矛盾的 result/error。平台 ack_at 为收到有效 Ack 的 UTC 时间，另存 executed_at，二者不得混用。

非法版本/JSON/身份、未知 command_id、跨设备 Ack 均丢弃并记录原因，不改命令，也不广播。设备错误码按协议建议支持 COMMAND_EXPIRED、COMMAND_DUPLICATED、UNSUPPORTED_COMMAND、INVALID_PARAMS、DEVICE_BUSY、DEVICE_NOT_SUPPORTED、SENSOR_UNAVAILABLE、ACTUATOR_UNAVAILABLE、INTERNAL_ERROR；其他非空设备码可作为诊断结果保留，不据此运行平台动作。

同一终态的重复 Ack 幂等，不重复保存或广播；冲突 Ack 保留首个终态并记录诊断。timeout 后到达的 Ack 不将 timeout 改为 success/failed；只保留日志中的迟到执行事实。本轮不增加 UI 可查询的迟到结果字段。

## 6. HTTP 查询与 WebSocket

GET `/api/v1/commands/{command_id}` 返回 HTTP 200/code 0 和明确 DTO：command_id、device_id、type、params、status、issued_at、expires_at、ack_at、executed_at、result、error。没有 Ack 时 ack_at/executed_at 为 null，无结果/错误时为 null。不存在 → HTTP 404/code 40402。不直接序列化 GORM 模型。

WS 复用 `/api/v1/ws`，格式为 `{type:"command_update",timestamp:<platform UTC time>,data:<command DTO>}`。data 至少包含 command_id/device_id/status，其余查询字段保持一致。每个持久状态转换产生一次更新；非法/重复回执不产生更新。首次连接不保证重放全部历史，客户端仍以 GET 为最终查询入口。

沿用现有有界队列、慢连接驱逐、写超时与心跳；断开或不读取的客户端不阻塞另一个客户端、MQTT 接收或 Monitor。

## 7. 启动、重连与关闭

启动时数据库中的 success/failed/timeout 保留；扫描 pending/sent，已经到期的转 timeout，未到期继续等待合法 Ack，不重新发布动作。重启前 pending 未发出去也不能自动补发，以免在原调用上下文结束后触发动作；到期后查询仍有明确结果。

Broker 断开后使用既有在线失效机制；命令前置校验必须检查自身发布可用性。重连恢复 Ack 订阅，新受理命令可正常完成。MQTT 等待必须有限且响应进程 context；不在 callback 中等待设备执行，不在 Monitor 锁内发布。

关闭时停止新命令受理，取消发布工作、截止扫描和相关连接。未完成记录留在数据库，按下一次启动的恢复规则处理；不得留下永久依赖内存 timer 的状态。

## 8. 模拟器与验收

模拟器在原 status/telemetry 基础上新增 commands 订阅，支持成功、设备失败、不回执三种模式。它检查版本、设备 ID、类型/参数和 expires_at。首次有效命令记录模拟执行与结果；同 command_id 重投递只重发缓存 Ack，不再执行；过期动作不执行并按协议回报 COMMAND_EXPIRED。

模拟场景启停命令在本轮只验证接收、模拟处理与 Ack；不承诺等同 Python 场景播放器或真实硬件行为。模拟器保留执行次数等可检查输出，便于证明幂等。

本轮软件组件验收串联 HTTP→模拟发布器→实际模拟设备执行器→Ack→SQLite/GET/两个 WS，覆盖成功、设备失败、无 Ack、离线、发布不可用、持久恢复、极速 Ack、重复/冲突/迟到/跨设备 Ack。单元/组件检查覆盖边界与状态竞争；监控已有行为作为回归。

完整验收列表及逐项编号以同目录上级 brief A1—A10 为准。用户于实现期间明确本轮不需要 Docker/硬件测试，正式检查采用 Go test/race/vet 与软件组件联调。真实 Mosquitto、网络重连和实际后端进程生命周期联调保留为后续检查，未执行不得认定通过。

## 9. 依据和变更边界

既有协议来源：项目根目录 `docs/adas_network_api_spec.md` 第 5、15—26、37—40、46、49—50、52—55、60—62、67、69—70 节。

本轮已确认 HTTP 202 异步受理、统一 10 秒截止、indicator duration 范围、Ack/publish 竞争、终态不变和恢复不重发。Build 同步修订网络规范相关示例以保持文档一致，不修改既有 realtime-monitoring 行为。
