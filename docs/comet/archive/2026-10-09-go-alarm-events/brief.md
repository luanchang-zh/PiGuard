# Outcome

详细规划 PiGuard Go 后端下一步，将下一轮实现收敛为告警事件上行闭环：模拟设备 MQTT events → SQLite 按消息幂等入库 → HTTP 筛选查询 → WebSocket event 通知。

用户于 2026-10-08 要求按当前完整规格详细实现。目标、范围、关键决定、A1—A7 和非目标已确认。

# Scope

## 已核实的基线

- 使用 Gin、Paho、GORM、SQLite，沿用 handler → service → repo 分层和现有 Hub。
- 实时监控已归档：telemetry/status、最新状态、1Hz 历史、四个查询和 WebSocket。控制命令与 Ack 已归档：四类动作、pending/sent/success/failed/timeout、command_update。这两条能力本轮不重做。
- 告警仍是占位：`EventRepository.List` 固定返回未实现；`EventHandler.List` 因此不能返回真实记录；Subscriber 只订阅 telemetry、status、command-acks，没有 `car/+/events`。
- `events` 表已有全局唯一 `event_id`、设备、类型、action、level、payload、timestamp、snapshot_id，可以直接存一条不可变消息。模型注释里的 action 示例 `updated` 与网络规范不一致，以规范的 `level_changed` 为准。
- Hub 已有有限队列和慢连接驱逐。模拟器目前发布 status/telemetry 并回执命令，不发布 events。
- 配置写入、图片上传和读取仍占位。仓库没有 Vue 或 Python/树莓派实现。

主要代码依据（路径相对项目根目录）：

| 区域 | 文件 | 下一步改动 |
| --- | --- | --- |
| HTTP | `go-backend/internal/handler/event.go` | 解析 type/level/start/end/limit，返回事件 DTO |
| 业务 | `go-backend/internal/service/event.go` | 校验后的入库、幂等和查询 |
| 存储 | `go-backend/internal/repo/event.go`、`model/event.go` | 插入、冲突比较、筛选查询 |
| 协议 | `go-backend/internal/protocol/` | 新增 event DTO 与校验 |
| MQTT | `go-backend/internal/mqtt/subscriber.go` | 订阅 events，重连时与原有主题一起恢复 |
| 推送 | `go-backend/internal/realtime/hub.go` | 复用，发送 event |
| 联调 | `go-backend/cmd/simulator/main.go` | 增加可重复的事件发布 |

## 下一轮具体范围

1. 订阅 `car/+/events`，QoS 1、retained=false。只接收已登记设备的 schema_version=1 消息。
2. 支持 `obstacle_warning`、`lane_departure`、`sharp_turn`、`high_temperature`、`sensor_failure`，以及 `started`、`level_changed`、`recovered`。level 只允许 `warning` 和 `danger`。
3. 每条 MQTT 消息按全局唯一 `event_id` 成为一行不可变记录。相同重放不重复入库、不重复推送；内容冲突保留首条。
4. 实现 `GET /api/v1/devices/{device_id}/events` 的筛选、错误码和稳定排序。
5. 首次入库后广播 WS `event`。建连不重放历史。
6. 可选保存 `snapshot_id`。不接收图片字节，也不实现帧接口。
7. 扩展模拟器，使同一 event_id 可以重复发布，并用软件测试验收。

## 实施顺序与完成门槛

| 步骤 | 工作与依赖 | 交付物 / 完成门槛 |
| --- | --- | --- |
| S1 | Shape 确认消息模型、幂等、查询和推送 | 本 brief 与 alarm-events 完整规格获确认 |
| S2 | 协议 DTO 与拒绝规则 | 非法消息不入库、不广播 |
| S3 | 仓储插入、冲突比较和查询 | 全局 event_id 唯一；重复和冲突不改首条 |
| S4 | MQTT 订阅与服务接入 | 首次订阅和重连都包含 events，且不丢掉原有主题 |
| S5 | HTTP 与 WS | 筛选结果与推送来自同一持久记录 |
| S6 | 模拟器与测试 | 覆盖合法、重复、冲突、查询和双客户端 |
| S7 | 独立 Verify | A1—A7 与 Go test/race/vet 通过；既有监控和命令测试保持通过 |

S2—S6 共用同一条事件记录，采用单个 Native change。配置同步、图片和部署仍各自独立规划，本轮不创建子 change。

## 后续路线图

| 顺序 | 范围 | 依赖 | 完成标志 |
| --- | --- | --- | --- |
| P0（已完成） | 实时监控 | MQTT、SQLite、Hub | telemetry/status 可查询、可推送 |
| P1（已完成） | 控制命令与 Ack | 在线事实、命令表 | 四类动作有明确终态 |
| P2a（本轮目标） | 告警事件 | 上行接入、events 表、Hub | event_id 幂等、筛选查询、WS event |
| P2b | 配置同步与确认 | P1 下行、device_configs | 规则校验、GET/PUT、desired/reported、config-acks |
| P3 | 图片上传、读取与关联 | P1 命令、P2a 的 snapshot_id | JPEG、latest-frame、frame_update |
| P4 | 部署与运行完善 | 业务契约稳定 | 鉴权、MQTT 凭据、保留策略、部署说明 |

不得把 P2b—P4 自动纳入本轮实现。

# Non-goals

- 不在确认前修改业务代码，也不把本次规划视为已经授权实现。
- 不实现 config.update、config-acks、图片上传、帧读取、camera.snapshot、Vue 页面、Python 边缘端或告警规则计算。
- 不把 telemetry 的 `active_events` 自动转成 events 行，也不用事件消息修改在线状态或 last_seen_at。
- 不把多条生命周期消息合并成带 started_at/ended_at 的可变告警实例，也不新增协议里没有的 incident_id。
- 不建设 MQTT 手动确认或通用 outbox。数据库写入失败时不广播；当前客户端会自动确认 QoS 1，恢复依赖发送方稍后重放，重放由 event_id 幂等接住。
- 不启动 Docker、不测真实蜂鸣器或树莓派。真实 Mosquitto 不作为本轮通过条件。
- 不提交、合并、推送或创建 PR。

# Acceptance examples

- A1：已登记设备向 `car/{device_id}/events` 发布五类 type、三种 action、level 为 warning 或 danger 的合法消息后，各保存一行。GET 返回 HTTP 200/code 0，items 含 event_id、type、action、level、UTC timestamp、data 对象和 snapshot_id。主题 QoS 1、retained=false。
- A2：非法 JSON、schema_version 不是 1、主题与 payload 设备不一致、空或超长 event_id、未知 type/action/level、非法时间、data 不是对象、未知顶层字段、未知设备，都不插入、不广播，进程不退出。设备当前离线时，合法事件仍入库。telemetry 里的 active_events 不产生 events 行，事件也不改变 online 或 last_seen_at。
- A3：同一 event_id 且设备、类型、动作、等级、时间、data、snapshot_id 一致的重放仍只有一行，并且不第二次广播。同一 event_id 的内容不同，或该 ID 被另一设备复用时，保留第一行，不覆盖、不广播。
- A4：未知设备的 GET 返回 HTTP 404/code 40401。非法 type、level、start、end、limit，或 start 大于 end，返回 HTTP 400/code 40001，并标明字段。limit 默认 100，允许 1—1000。时间过滤包含端点，条件同时生效。无记录时 items 为 []。结果按 timestamp 降序、event_id 降序。时间更早但 event_id 不同的消息仍会入库。响应不含数据库自增 ID。
- A5：只有首次成功插入才向已连接客户端发送 `{type:"event",timestamp,data}`。data 含 event_id、device_id、event_type、action、level、设备 timestamp、data 和 snapshot_id。两个客户端都能收到；新建连接不重放旧事件。持续不读取的连接被断开，且不阻塞 MQTT 或其他客户端。
- A6：消息带合法 snapshot_id 时，GET 和 WS 返回该值；省略时为 null。snapshot_available 可以出现，但不入库、不返回。本轮图片上传和读取仍不可用。平台重启后已存事件仍可按 A4 查询；Broker 重连后同时恢复 events、telemetry、status、command-acks，且不把旧事件补推到 WS。
- A7：在 go-backend 运行 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 通过。软件测试覆盖合法入库、重复、冲突、非法拒绝、筛选和双 WS。模拟器能以 QoS 1、非 retained 发布一条事件并重发同一 event_id。既有监控与命令测试保持通过。本轮不启动 Docker、不测硬件，真实 Mosquitto 未执行不算失败。

# Constraints and invariants

- 完整目标规格位于 `specs/alarm-events/spec.md`。沿用 schema_version=1、UTC RFC3339Nano，以及现有 HTTP/WS 信封。
- `event_id` 是一条 MQTT 消息的全局幂等键，不是会被后续动作改写的告警实例号。`started`、`level_changed`、`recovered` 各是独立消息。恢复时间就是 recovered 消息自己的 timestamp。
- 课设草案里的 started_at/ended_at 和模型注释里的 updated，不覆盖网络规范第 9—11、33—34、48、59 节。告警等级只用 warning/danger；回到正常由 action=recovered 表达，不用 level=normal。
- HTTP 条目的类型字段名是 type；WS 的类型字段名是 event_type。二者都来自已发布规范，本轮不改名。
- 终态记录不回退、不覆盖。平台不重新计算风险，也不校验 data 内部的物理含义；data 必须是 JSON 对象，内容原样保存。
- 只接收已登记设备。未知设备丢弃并记录原因。事件可以在设备被标为离线时入库，以便断网后补传。
- 回调里不执行可能被慢客户端堵住的广播。SQLite 保持单写连接。每次真正的新插入才广播一次。
- 不提交、合并、推送或创建 PR。

# Decisions

- 用户要求用 Comet 继续规划 Go 后端。当前没有其他进行中的 Native 需求，工作区干净，因此沿用分支 `go后端开发` 和当前目录。
- 下一轮选择告警事件，不选择配置同步。依据是已归档 P0/P1、网络规范第 69 节的联调顺序，以及命令闭环 brief 里写明的 P2a。
- 采用不可变消息日志，不用可变告警实例。依据是 HTTP 返回带 action 的消息列表，并且 event_id 有全局唯一约束。协议没有 incident_id，本轮不发明该字段。
- action 使用 started/level_changed/recovered。level 使用 warning/danger。
- 可选 snapshot_id 现在就保存，图片字节留到 P3。snapshot_available 只校验类型，不作为存储字段。
- WS 在规范第 48 节的字段之外，补上设备时间、data 和 snapshot_id，使告警列表不必为每条推送再查一次详情。
- 查询按新到旧返回，默认 limit 100、最大 1000，错误码与遥测查询一致。
- 验收使用 Go 测试和模拟器协议行为，不把 Docker 或硬件作为通过条件。这沿用 2026-10-08 命令闭环里用户明确的当前约束。
- 拆分检测结论：接入、幂等、查询和推送必须一起验收，保持单一 change。P2b/P3/P4 不创建 children.yaml。
- 用户以“详细完成一下吧”确认按该 Shape 实现，不改验收项。

# Open questions

没有未解决的用户决定。用户已确认目标、范围、关键决定、A1—A7 和非目标。

# Verification expectations

规划检查：核对正式文档链接、A1—A7 与完整规格一致，并由 Runtime 保存 Shape 摘要。确认前没有修改 `go-backend`。

Build 检查：协议边界、全局 ID 冲突、查询参数、离线补传、WS 只发一次，以及模拟器重复发布，都应有对应测试。

Verify 检查：在 go-backend 运行 Go test、race 和 vet。用软件路径核对入库、GET 和双 WS。由新的只读 Verifier 逐项验收。真实 Broker、Docker 和硬件在本轮明确不执行。

文档依据：

- [网络交互与 API 规范](../../../adas_network_api_spec.md)：第 4—5、8—12、33—34、46、48、52—54、59、69 节。
- [课设设计方案](../../../raspberry_pi_adas_course_design.md)：平台告警记录、events 表草案、告警页面需要的时间和等级；其中 started_at/ended_at 不作为本轮存储模型。
- [实时监控完整规格](../../specs/realtime-monitoring/spec.md)。
- [控制命令完整规格](../../specs/command-control/spec.md)。
