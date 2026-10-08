# Go 实时监控闭环完整目标规格

本 capability 描述下一轮拟实现的完整实时监控行为。它以 PiGuard 现有协议 v1 和 Go 分层为基础；用户当前只要求规划，尚未授权通过 Shape 确认进入 Build。

## R1：遥测协议与输入校验

平台接收 `car/{device_id}/telemetry`，QoS 0、非 retained。完整消息包括 schema_version、message_id、device_id、timestamp、seq，以及 speed、distance、temperature、gyro、lane、risk。

- timestamp 和 sample_at 为带时区的 RFC3339 时间，对外归一为 UTC；schema_version 为 1。
- 设备必须已登记，主题 device_id 必须与 payload 一致。
- 保留 speed 的 value/unit/source/status/sample_at；distance 的 raw_value/mapped_value/unit/scale/source/status/sample_at；temperature 的 value/unit/source/status/sample_at；gyro 的 x/y/z/yaw_rate/unit/source/status/sample_at；lane 的 valid/offset_ratio/direction/confidence/sample_at；risk 的 level/active_events。
- 单位映射为 km/h、m、C、deg/s，距离对外使用 mapped_value，车道使用 offset_ratio；枚举遵循协议第 7 节。
- 数值缺失、无效或传感器 status 非 ok 不能通过 Go 零值伪装成正常测量；lane.valid=false 不能当成车道居中。
- 非法 JSON、缺失必要身份/排序字段、不支持版本、设备身份不符、无效时间或非法枚举，整条消息拒绝并记录原因，不写历史、不更新设备或最新状态、不广播。
- 协议 DTO、HTTP/WS DTO 与 GORM 模型各自表达对应职责；历史表不能成为完整协议解析结构。

## R2：设备状态及 MQTT 生命周期

平台订阅 `car/+/status`（QoS 1）及 `car/+/telemetry`（QoS 0），首次订阅失败不得把该接入阶段报告为就绪。连接重建后自动恢复订阅；回调中不执行可能被慢客户端无限阻塞的广播。

合法 status 包含 schema_version、device_id 和 online；在线报告还包含 timestamp、software_version、config_version、mode、sensors 和 mqtt。在线状态按报告更新，last_seen_at 使用平台收到合法上行消息的时间；完整 status 带来的时间和传感器信息单独保留。

必须兼容协议的离线遗嘱：`schema_version=1`、`device_id`、`online=false`、`reason=connection_lost`，没有 timestamp 仍然有效，离线状态立即生效。离线消息省略的设备元数据不被清空。相同状态的重复投递不产生虚假的状态变化。

status.config_version 表达设备当前报告版本，设备 API 的 config_version 映射该事实；不修改 desired_version，不据此伪造 config-ack 或配置成功事件。

首次联调使用已登记的 `car-001`。未知设备不自动建立数据库记录，消息丢弃并记录设备标识和原因。后续自动注册属于独立需求。

平台启动时不直接沿用 SQLite 中可能过时的 online=true：登记设备在获得新的 status 前为离线/未确认，retained status 到来后再更新。此行为不重置名称、规则或版本。平台重新连上 Broker 后同样等待状态确认；Broker 连接失去时，本机在线事实视为未确认离线。

## R3：最新状态与消息顺序

最新状态按设备隔离并安全并发读写，保存完整遥测及平台接收时间。比较 `(timestamp, seq)`：采样时间更新时接受；时间相同时只接受更大的 seq；更旧时间或相同时间下重复/更小 seq 不替换状态、不广播、不重复存历史。

设备重启后 seq 可以归零，只要 timestamp 更新即可接受。协议 v1 没有 boot_id；第一版采用上述跨重启规则，依赖设备 UTC 时钟，时钟回拨后的旧时间不会覆盖平台已有最新状态。

telemetry 更新最新测量与 last_seen_at，不覆盖 status 的 online=false。平台进程重启后，最新状态等待新遥测；已保存的低频历史用于历史查询，不作为新的实时数据重放。

## R4：状态视图与新鲜度

state 与 WS telemetry 使用同一归一化视图。既有字段为 device_id、timestamp、speed_kmh、distance_m、temperature_c、yaw_rate_deg_s、lane、risk_level。另附 online、各信号的 sample_at/source/status 和 freshness 元数据，避免缺失采样信息。

- freshness 的取值为 fresh、stale、fault、missing；status 为 timeout/unavailable/error 时为 fault，缺少可用值或 sample_at 时为 missing，否则按 TTL 判断 fresh/stale。
- speed、distance、gyro、lane 的 TTL 为 1 秒，temperature 为 5 秒；超过 TTL 才判定 stale。新鲜度由每个信号自己的 sample_at 计算。
- 空状态返回 HTTP 200/code=0，timestamp 和测量值为 null，lane.valid=false，risk_level=unknown，freshness=missing；不返回假造的正常零值。
- 故障与缺失测量返回 null。陈旧但合法的上一测量可以保留，但必须同时标记 stale。设备离线单独由 online=false 表达。
- 风险等级由边缘端上报，平台不重新实现预警规则；不得把无遥测状态标为 normal。
- HTTP 在请求时计算新鲜度。WS 为已订阅客户端在信号跨越 TTL、设备状态改变时发送更新的状态视图，使静默设备的最后一帧不会永远被保留为 fresh；重复未变化的状态不需要不断广播。

## R5：历史快照与 SQLite

对可成为最新状态的遥测，每台设备最多每秒写入一次历史快照：首条可立即落库，后续写入之间的平台时间间隔至少 1 秒。不同设备独立限频。接入与最新状态仍按合法遥测频率更新，默认 2Hz，不因低频入库而降为 1Hz。

记录包含 device_id、设备 timestamp、seq，以及 speed、mapped distance、temperature、gyro、yaw_rate、lane offset、risk。保留解释历史测量是否可用所必需的采样元数据；缺失和故障测量不写成正常零值。可按现有 GORM AutoMigrate 扩充表表达，SQLite 保持单写连接。

没有新的遥测时不定时重复写入上一帧；重复/乱序消息不重复落库。数据库失败必须留下错误记录，不能报告历史写入成功；HTTP 历史查询失败按内部错误返回。已保存快照跨平台重启保留。

## R6：HTTP 查询

| 路由 | 行为 |
| --- | --- |
| `GET /api/v1/devices` | 返回真实登记设备列表；空集合为 []，包含 device_id/name/online/last_seen_at/config_version |
| `GET /api/v1/devices/{device_id}` | 返回登记信息、状态报告的 mode/software_version/config_version/sensors，以及 online/last_seen_at |
| `GET /api/v1/devices/{device_id}/state` | 返回 R3—R4 的完整最新状态视图 |
| `GET /api/v1/devices/{device_id}/telemetry` | 以 `{items: [...]}` 返回 R5 保存的历史快照及解释可用性所需的元数据 |

历史参数：start/end 可选，为 RFC3339 时间；时间过滤包含端点。limit 默认 100，范围 1—1000。先选取时间窗口内最新的 limit 条，再按 timestamp、seq、ID 升序返回，便于图表绘制；设备之间不混查。无记录时 items=[]。

成功信封 `{code:0,message:"ok",data:...}`。未知设备为 HTTP 404/code=40401；非法时间、start>end、非整数或范围外的 limit 为 HTTP 400/code=40001，error 标明字段；内部故障为 HTTP 500/code=50001。对外不输出数据库自增 ID 或 GORM 内部字段。

## R7：WebSocket

`GET /api/v1/ws` 升级连接，推送协议信封 `{type,timestamp,data}`。本 capability 发送 telemetry 与 device_status；timestamp 为平台 UTC 事件时间。

- telemetry.data 使用 R4 视图，包含 device_id，合法最新消息到来和新鲜度变化时推送。
- device_status.data 至少包含 device_id/online，仅对有效的状态变化推送。
- 本轮广播全部已登记设备，通过 device_id 区分；不新增前端订阅协议或设备过滤 API。
- 每个连接只有一个写入执行者，配置有限发送队列与写超时；持续消费不及的客户端关闭，释放连接和队列。慢连接不能阻塞 MQTT 接收或其他 WS 客户端。
- 客户端断开、写失败、读失败以及平台关闭时及时退出相关执行过程；设置存活检测和超时。
- HTTP 获取初始视图，WS 推送后续变化；WS 建连不重放全部历史数据。Vue 实现不属于本 capability。

## R8：联调与交付

在现有 Go 目录结构和 Compose Broker 上工作。提供可重复的模拟发布与 HTTP/WS 读取步骤，涵盖合法遥测、在线 status、无时间戳遗嘱、乱序与重复、seq 重启、TTL 到期、传感器故障、未知设备、非法历史参数、MQTT 重连、WS 慢连接以及平台重启。

验收不依赖真实树莓派、Python 边缘端或 Vue 页面，可使用测试客户端。`go test ./...`、`go test -race ./...`、`go vet ./...` 以及真实 Broker 的模拟链路检查必须成功；独立只读 Verifier 逐项核对 brief 的 A1—A9。

## 能力边界

命令与 Ack、事件、配置更新与 Ack、摄像头图片、前端页面、规则计算、设备自动注册和鉴权扩展均不由本 capability 实现，后续按 brief 的 P1—P4 逐步推进。
