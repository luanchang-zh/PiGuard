# Outcome

详细规划 PiGuard Go 后端下一步，将下一轮实现收敛为控制命令与设备回执闭环：HTTP 请求 → SQLite 命令记录 → MQTT commands → 模拟设备 → command-acks → HTTP 查询 / WebSocket 通知。

用户于 2026-10-08 在阅读完整规划后明确要求完成实现，已确认本 brief、完整目标规格及 A1—A10。交付命令/Ack 闭环的业务代码、联调客户端、检查和独立验收。

# Scope

## 已核实的基线

- 使用 Gin、Paho、GORM、SQLite，沿用 handler → service → repo 分层和 server 集中依赖组装。
- 实时监控已完成：telemetry/status 接入、最新状态、TTL、每设备最多 1Hz 历史、四个 HTTP 查询和 WebSocket。2026-09-30 的 `go-realtime-uplink` 已归档，验收 A1—A9 全部通过；本次未重跑这些业务检查。
- 下一阶段的缺口是业务闭环：命令 handler 不解析请求体；CommandService.Create、CommandRepository.Find、MQTT Publisher.Publish 仍占位；Subscriber 尚无 command-acks 订阅。
- commands 表已有 command_id 唯一索引、设备、类型、参数、状态、签发/到期时间、AckAt、Result、Error，可以直接承载本轮生命周期。
- Hub 已有有限队列、慢连接驱逐和连接清理，复用它发送 command_update；模拟器目前只发 status 和 telemetry。
- 告警、配置写入、图片接口也仍占位；仓库没有实际 Vue 或 Python/树莓派实现，继续使用模拟设备独立验收。

主要代码依据（路径相对项目根目录）：

| 区域 | 文件 | 下一步改动 |
| --- | --- | --- |
| HTTP | `go-backend/internal/handler/command.go`、`response.go` | 解析请求、返回 DTO、映射 40001/40401/40402/50301/50302 |
| 业务 | `go-backend/internal/service/command.go` | 创建、异步下发、Ack、条件转换、持久超时与恢复 |
| 存储 | `go-backend/internal/repo/command.go`、`model/command.go` | 新增写入/查询/条件更新/待决扫描，保留已有表 |
| 协议 | `go-backend/internal/protocol/` | 新增 command 和 command-ack DTO/校验 |
| MQTT | `go-backend/internal/mqtt/publisher.go`、`subscriber.go` | 真正发布、订阅 Ack、重连恢复 |
| 生命周期 | `go-backend/internal/server/app.go` | 组装依赖、启动和停止命令工作过程 |
| 联调 | `go-backend/cmd/simulator/main.go`、`internal/server/` | 模拟命令执行、幂等回执、真实 Broker 检查 |

## 下一轮具体范围

1. 实现 POST `/api/v1/devices/{device_id}/commands` 与 GET `/api/v1/commands/{command_id}`。
2. 支持 `buzzer.test`、`indicator.test`、`scenario.start`、`scenario.stop`；参数在完整目标规格中列明。
3. 平台生成 command_id / issued_at / expires_at，先持久化再下发；命令主题 QoS 1、retained=false。
4. 接入 `car/+/command-acks`，校验设备身份、协议版本、命令归属和结果结构。
5. 实现 pending / sent / success / failed / timeout 生命周期，处理发布失败、未收到回执、重复/冲突/迟到 Ack、Ack 与 publish 完成竞争。
6. 持久恢复未完成命令的截止时间；平台重启不自动重发动作命令。
7. GET 和 WS command_update 显示相同的持久状态；扩展模拟器与完整验收。

## 实施顺序与完成门槛

| 步骤 | 工作与依赖 | 交付物 / 完成门槛 |
| --- | --- | --- |
| S1 | Shape 确认协议、响应、截止时间和失败语义 | 本 brief 与 command-control 完整规格获确认；不把示例时间或隐含假设当强制契约 |
| S2 | DTO、参数校验、领域错误与 HTTP 映射 | 四类请求和 Ack 能严格校验；非法请求不创建记录；不直接输出 GORM 模型 |
| S3 | 命令仓储与状态机 | command_id 唯一；按当前状态条件更新；重复回执和竞争不能回退状态 |
| S4 | MQTT Publisher 与 Ack 订阅 | Publish 有限等待/取消；QoS 1 non-retained；首次与重连都恢复 Ack 订阅 |
| S5 | 创建/异步下发/超时/恢复/查询 | 接通 HTTP 和持久状态；pending/sent 到期会结束；重启不触发动作重放 |
| S6 | WS 与模拟器 | 两个客户端收到 command_update；模拟器有成功、失败、无 Ack 模式及命令 ID 去重 |
| S7 | 独立 Verify | 全部验收项、Go test/race/vet 与 HTTP/WS 软件组件联调通过，并保留监控回归；真实环境测试延期 |

S2—S6 共用命令状态和同一消息链路，采用单个 Native change。调查阶段可并行读代码；本轮不创建 Supervisor/children。后续不同能力再独立建 change。

## 后续路线图

| 顺序 | 范围 | 依赖 | 完成标志与提前要澄清的契约 |
| --- | --- | --- | --- |
| P0（已完成） | 实时监控 | 既有 MQTT/SQLite/Hub | telemetry/status → HTTP/WS 与历史；沿用已归档规格 |
| P1（本轮目标） | 控制命令与 Ack | 在线事实、命令表、Publisher、Hub | 四类动作可查 success/failed/timeout，异常和重启有明确结果 |
| P2a | 告警事件 | 上行解析、事件仓储、Hub | QoS 1 event_id 幂等、筛选查询、WS event；先区分每条消息 ID 与告警实例 ID，统一 level_changed 命名 |
| P2b | 配置同步与确认 | P1 下行能力、已有 device_configs | 规则校验、GET/PUT、desired/reported 分离、config-acks；明确 expected_version 并发契约和 Broker 重启补发策略 |
| P3 | 图片上传、读取与关联 | P1 命令、P2a 事件 | JPEG 校验/原子落盘、latest-frame、snapshot-content、camera.snapshot、frame_update；明确 frame_id 去重和 event/command 关联字段 |
| P4 | 部署与运行完善 | 业务契约稳定 | HTTP 鉴权、MQTT 凭据/设备 ACL、就绪检查、数据保留、持久卷与可重复部署说明 |

P2a/P2b 是候选独立 change，不是本次已授权的子 change。不得自动将路线图全部纳入本轮实现。

已发现的后续问题：当前 Mosquitto `persistence false`，retained 配置在 Broker 重启后会丢失；Event 示例和图片上传缺少明确关联字段；Snapshot 模型缺 frame_id；配置 PUT 尚无明确条件版本字段。这些应在相应 Shape 中先解决。

# Non-goals

- 不推送、合并或创建 PR；工作区交付遵循 Native 收尾流程。
- P1 不实现 camera.snapshot、config.update、告警入库、图片上传、Vue 页面、Python/真实执行器或边缘 RuleEngine。
- 不将 HTTP 网络重试视为自动幂等：本轮没有客户端幂等键，每个有效 POST 都是新命令；只保证同一 MQTT command_id 的设备/Ack 幂等。
- 不新增微服务、通用任务框架、数据库替换或完整 outbox 框架；复用现有具体服务与仓储。
- 不以模拟器成功代替真实蜂鸣器/指示灯/树莓派硬件验收。

# Acceptance examples

- A1：四类合法请求通过校验。在线且 Broker 可用的设备收到请求后，HTTP 202/code 0 返回 command_id 和 pending；数据库已存在唯一记录。MQTT 下发含 schema_version=1、同一 ID/设备、类型、参数及 UTC 签发/到期时间，QoS 1、retained=false。
- A2：非法 JSON、缺失/越界参数、未知或本轮不支持的类型返回 HTTP 400/40001；未知设备返回 404/40401；未知 command_id 返回 404/40402。上述无效创建请求不入库、不发布。
- A3：已知离线设备返回 HTTP 503/50301；设备在线但 Broker 不可用返回 503/50302。两种前置拒绝均不创建命令或发布。受理后发布明确失败写 failed，发布结果不确定时保留待决并最终超时；不得宣称设备执行成功。
- A4：单独收到 Broker publish 成功，GET 与 WS 显示 sent；合法 success Ack 才写 success 和 result，合法 failed Ack 写 failed 和设备 error，两类都保留执行时间和平台接收时间。
- A5：Ack 早于 publish 完成时仍可从 pending 进入终态；随后 publish 完成不能覆盖回 sent。重复相同 Ack、冲突 Ack 不重复改变终态/广播；非法版本、主题身份不符、未知命令或跨设备 Ack 不更新记录。
- A6：在签发后 10 秒截止前未收到有效 Ack，pending 和 sent 均变为 timeout；迟到 Ack 不改终态，只留诊断。timeout 表示未及时确认，不宣称设备一定没有执行。边界时间采用平台接收有效 Ack 的时间，不信任设备时钟决定超时。
- A7：平台重启后保留已结束命令；从数据库恢复 pending/sent，已过截止时间变 timeout，未到期继续等待 Ack；不自动重新发布动作。Broker 重连后可继续接收新命令的 Ack。
- A8：GET 返回 command_id/device_id/type/params/status/issued_at/expires_at/ack_at/executed_at/result/error；没有 Ack 时 ack_at/executed_at 为 null。WS command_update 使用统一 type/timestamp/data 信封，状态来自同一持久记录；两个客户端可见更新，慢或断开连接不阻塞命令或遥测链路。
- A9：模拟器订阅 commands，提供成功、设备失败、不回执三种模式，按 command_id 缓存结果；重复命令只重发同一 Ack，不再次执行，已过 expires_at 的新动作不执行。软件组件联调核对 HTTP/模拟发布器/模拟设备执行器/GET/双 WS 的同一 ID 及成功、失败、超时和持久恢复行为。
- A10：必要的 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 通过；既有单元与组件层的监控排序、TTL、1Hz 历史和双 WS 行为保持通过。按用户最新约束，本轮不启动 Docker、不做硬件测试，真实 Mosquitto 和实际 Broker/平台进程生命周期检查留待后续且明确标为未执行，不算通过。

# Constraints and invariants

- 完整目标规格位于 `specs/command-control/spec.md`。沿用 schema_version=1、UTC RFC3339Nano 和现有 HTTP/WS 信封。
- HTTP 202 只表示已持久受理，响应固定 pending；查询可能立即读到 sent 或终态。与现有规范 POST pending 示例一致，新增明确异步受理语义。
- 四类动作使用 10 秒总截止时间。规范示例的 5/10 秒不是强制默认；蜂鸣器/指示灯最长执行 5 秒，预留发送和回执时间。indicator duration 沿用已确认的 100—5000ms。
- 终态 success/failed/timeout 不回退。设备执行结果只能由合法 Ack 给出；平台发送错误必须明确表示发送阶段故障。
- 前置连接检查不能保证随后发送一定成功；Paho 等待超时/取消不能证明设备没收到消息。不盲目补发，不把不确定交付等同明确执行失败。
- 不持有 Monitor 监控锁做命令发布或等待；命令处理和超时扫描可停止，使用现有 SQLite 单写连接。并发事实由条件更新保证。
- 每个真正持久状态变化才广播一次 command_update；非法、重复和迟到回执留日志诊断，不创造成功事实。
- 2026-10-08 已按用户明确授权修复历史归档兼容：仅移除当前 CLI 不支持的 document_constraints_version 字段，state_version=8、9/9 验收、hash/history/report 全保留。原文备份：`C:/Users/23156/AppData/Local/Temp/piguard-comet-native-repair.YGzYN0/comet-state.yaml`。官方 status 已恢复 archive/done；该动作不代表重跑原业务验收。

# Decisions

- 用户要求使用 Comet 详细规划下一步 Go 后端，并明确授权修复旧 Comet 数据为正确 Native 格式。
- 用户在实现期间明确“目前不用真正用docker和硬件测试”：本轮以 Go 单元/组件及 HTTP/WS 软件模拟联调验收，真实环境联调列为后续验证，不下载或安装 Broker 来扩大本轮测试。
- 项目配置解析结果为 Native，产物根目录 docs、语言 zh-CN、澄清 batch；当前目录和分支 go后端开发 用于本次规划。
- 本轮推荐 P1 命令/Ack，覆盖四类无媒体依赖动作；依据是已归档 P0 与原路线图、当前代码占位和网络规范联调顺序。
- 已确认异步 HTTP 202/pending、10 秒截止、终态不变、不自动重发、无 HTTP 幂等键。用户的“你来完成一下吧”授权按当前完整 Shape 实现。
- 拆分检测结论：本轮状态机、下行、Ack、持久恢复与 WS 紧耦合，单一 change 更适合独立验收；后续事件/配置/图片独立规划，不创建 children.yaml。

# Open questions

无未解决的用户决定；目标、范围、关键行为、A1—A10 和非目标均已确认。

# Verification expectations

规划检查：核对正式文档链接、全部范围和 A1—A10 与 Spec 一致；通过官方 Runtime 保存 Shape 摘要。旧归档兼容修复已由另一只读 agent 对备份和改后文件按字节核验，只有一处字段删除。

Build 检查：参数边界、Ack 身份/格式、仓储条件转换、Ack/publish 竞争、超时边界与重启恢复、HTTP 错误和 WS 更新都应有针对性测试；模拟器证明同 ID 幂等与过期拒绝。

Verify 检查：在 go-backend 运行 Go test/race/vet；以模拟发布器和实际模拟设备执行器进行 HTTP→软件命令发送→Ack→GET/双 WS 联调，并检查持久恢复。由新的只读 Verifier 独立逐项验收；真实 Broker、Docker 和硬件测试在本轮明确未执行。

历史环境限制：2026-09-30 归档使用 Linux 临时 Mosquitto，未启动 Docker Compose。当前 Windows 的 Go/CGo/Mosquitto/Docker 可用性在本次 Build 中重新检查，实际结果由 Runtime 的候选验收记录保存。

文档依据：

- [网络交互与 API 规范](../../../adas_network_api_spec.md)：第 5、15—26、37—40、46、49—50、52—55、60—62、67、69—70 节。
- [课设设计方案](../../../raspberry_pi_adas_course_design.md)：设备控制、模拟场景、命令去重与课程演示边界。
- [实时监控完整规格](../../specs/realtime-monitoring/spec.md)。
- [历史验收报告](../../archive/2026-09-30-go-realtime-uplink/verification.md)。
