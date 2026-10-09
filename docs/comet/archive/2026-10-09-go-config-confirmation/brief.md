# Outcome

检查 PiGuard Go 后端当前完成情况，规划下一阶段“配置同步与确认”：HTTP GET/PUT → SQLite 保存期望规则与版本 → retained MQTT config → 软件模拟设备 → config-acks → 查询真实生效结果。

本次交付是现状检查和 Shape 规划。用户已回答 Q1—Q3：要求条件版本、允许离线设备配置、通过 GET 查询结果；完整 Shape 仍待最终确认，尚未进入 Build。

# Scope

## 2026-10-09 已核实的现状

| 部分 | 完成情况 | 证据与边界 |
| --- | --- | --- |
| 基础服务 | 已实现 | YAML/环境变量、SQLite 六表建表、演示设备和默认规则种子、MQTT/HTTP 启停、health、Compose 文件 |
| 实时监控 P0 | 已实现并归档 | telemetry/status、最新状态、乱序/重复处理、TTL、每设备最多 1Hz 历史、HTTP、WS；原验收 9/9 |
| 控制命令 P1 | 已实现并归档 | buzzer.test、indicator.test、scenario.start/stop；pending/sent/success/failed/timeout、10 秒截止、幂等 Ack、重启不重发、GET/WS；原验收 10/10 |
| 告警 P2a | 已实现并归档 | 五类事件、三类 action、全局 event_id 幂等、冲突保留首条、筛选查询、双 WS、可选 snapshot_id；原验收 7/7 |
| 配置 P2b | 数据模型与种子已具备，业务未实现 | GET/PUT handler 不解析或输出规则；ConfigService.Get/Update 返回未实现；repo 只有 Find/Insert；没有 config-acks 订阅和配置模拟器 |
| 图片 P3 | 占位 | frames/frame/snapshot-content 接口和 snapshot 仓储未实现；camera.snapshot 仍不属于已支持命令 |
| 部署与运行 P4 | 开发骨架已具备 | Broker 匿名且 persistence=false；HTTP 鉴权、MQTT ACL、数据保留和生产部署尚待规划 |

本次在 Linux/Go 1.25.5 重新执行 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`，全部退出码为 0。首次沙箱测试因不允许本机监听端口失败，放行后完整重跑通过；该失败不是业务测试结论。测试使用 `/tmp/piguard-go-build-cache`。

本次没有启动 Docker、真实 Mosquitto 或硬件。上述结论覆盖源码和软件测试，不能扩展为实际设备链路通过。

已修正 go-backend/README.md 中“告警暂时 501”的过期描述。本次不修改业务代码。

## 下一轮推荐范围

1. GET 返回完整四类期望规则、desired_version、配置 Ack 确认的 reported_version 和当前版本同步结果。
2. PUT 接受部分规则字段，按已有规范第 66 节合并当前配置，然后校验完整结果；要求 expected_version，冲突返回 409/40902。
3. 原子递增每设备 desired_version，并持久保存该版本的完整规则、签发时间和 Ack 结果，先落库再发布。
4. 发布 `car/{device_id}/config`，QoS 1、retained=true；订阅 `car/+/config-acks`，QoS 1、回执非 retained。配置不走普通 commands，也不产生 command_id。
5. 设备离线仍可受理配置，但 MQTT 连接不可用时在落库前返回 503/50302。受理后出现发送故障仍保持持久 pending，不声称已生效。
6. 只以合法 success config-ack 前进 confirmed reported_version；failed 保留上一成功版本和设备 error。处理重复、冲突、迟到、跨设备及未来版本 Ack。
7. 启动与 Broker 重连后，从持久记录重新发布每设备最新期望配置，恢复 Broker 丢失的 retained 消息；补发不创建新版本、不重发动作命令。
8. 通过 GET 查询 pending/success/failed 和回执时间，本轮不新增 WS 消息。
9. 扩展软件模拟器接收配置、按设备/版本去重，支持 success/failed/none，保留既有监控、命令和告警行为。
10. 同步网络规范中新增的条件版本与查询字段；以软件组件链路和 Go test/race/vet 验收。

## 实施顺序

| 步骤 | 工作 | 完成门槛 |
| --- | --- | --- |
| S1 | 已回答 Q1—Q3，更新 Spec，确认 Shape | 全部范围和 A1—A10 获最终确认后 Comet 进入 Build |
| S2 | 请求/配置/Ack DTO 与规则校验 | 部分规则合并后严格校验；拒绝空/null/未知字段与尾随 JSON |
| S3 | 配置版本与回执持久化 | 版本分配、规则和 pending 同事务；条件更新防止状态回退 |
| S4 | GET/PUT 与发布工作过程 | 先落库后发布；并发更新不能让旧配置覆盖 Broker retained 新配置 |
| S5 | Ack 订阅、启动与重连恢复 | 先订阅再补发；五组上行主题一起恢复；各版本结果准确 |
| S6 | 模拟器与查询 | HTTP → 软件配置接收器 → Ack → GET 链路可重复运行 |
| S7 | 独立只读 Verify | 逐项验收 A1—A10；Go test/race/vet 和原有业务回归通过 |

配置版本、发送和 Ack 共用同一状态，适合单个 Native change。本轮不拆 Supervisor，不创建子 change 或其他会话。

## 后续路线图

| 顺序 | 能力 | 完成标志 |
| --- | --- | --- |
| P2b（下一轮） | 配置同步与确认 | GET/PUT、严格规则、retained 下发、版本区分、Ack 和重启恢复 |
| P3 | 图片上传与读取、拍照命令和引用 | JPEG 校验/落盘、latest frame、snapshot content、frame_update、camera.snapshot 和事件关联 |
| P4 | 部署和运行完善 | 鉴权、设备 ACL、Broker 持久化、就绪检查、保留策略、可重复部署 |
| 环境联调 | 已完成软件能力的真实链路 | Mosquitto 重连/遗嘱/QoS、Compose、树莓派逐项实测并记录 |

P3/P4 和真实环境联调独立规划，不自动纳入本 change。

# Non-goals

- 本次规划不实现业务；只有用户确认完整 Shape 后进入 Build。
- 不实现图片、camera.snapshot、Vue、Python 边缘规则引擎、设备自动注册、鉴权或数据库替换。
- 不把配置变成普通动作命令，不复用命令 10 秒超时；离线配置可以长期 pending。
- 不建设通用 outbox、持续离线消息队列、配置历史管理页面或任意版本回滚接口。
- 不把 status.config_version 当成 config-ack，也不声称 Broker publish 成功就是设备生效。
- 沿用此前的软件验收边界，不启动 Docker、下载 Broker 或测试硬件。
- 不提交、合并、推送、创建 PR 或自动归档。

# Acceptance examples

- A1：已登记设备 GET 返回 HTTP 200/code 0、完整四类 rules、desired_version、reported_version、status、ack_at、applied_at、error；未收到成功配置回执时不能虚增 reported_version。未知设备返回 404/40401，不暴露数据库 ID。
- A2：合法部分规则 PUT 按当前规则合并，保留未提交字段，并验证完整结果。非法 JSON、空 rules/null、未知字段、错误类型、非法阈值关系、非正 duration_ms 返回 400/40001 和字段路径；不改变版本、不发布。缺失/非法 expected_version 返回 400/40001，过期版本返回 409/40902。
- A3：按条件版本契约受理合法 PUT，原子分配新版本和完整配置；两次基于相同旧版本的并发更新只有一次成功。HTTP 202/code 0 返回新 config_version 与 pending；Broker publish 不得先于数据库提交，数据库失败返回 500/50001 且不发布。
- A4：下发内容包含 schema_version=1、device_id、新 config_version、UTC issued_at 和完整 rules；主题 config、QoS 1、retained=true。设备离线仍受理；MQTT 不可用的前置拒绝返回 503/50302，不写新版本。受理后的发布失败/不确定保持 pending，不伪造 success 或设备 error。
- A5：合法 success Ack 只确认平台曾持久签发的对应设备/版本，保存 applied_at 与平台 ack_at，单调前进 reported_version。合法 failed Ack 保存 error，并保留上一成功版本。GET 当前 status 来自最新期望版本，旧版本结果不能把新版本标成成功或失败。
- A6：错误 schema/主题身份/设备归属/未知或未来版本/时间/结果结构的 Ack 不更新；重复相同回执不重复转换，首个有效终态后的冲突回执不覆盖。旧 success Ack 仅在确实确认更高已签发版本时前进 reported_version，不能回退；极速 Ack 不被稍后的 publish 完成覆盖。
- A7：并发 PUT、发布、Ack 与恢复不造成版本重用或旧配置覆盖 retained 新配置。旧版本成功不得改变新版本 pending。GET 规则、版本和结果来自一致持久快照；发送等待不阻塞遥测处理，也不持有 SQLite 事务。
- A8：平台重启保留规则、版本和回执结果。启动/Broker 重连先恢复 telemetry/status/command-acks/events/config-acks，再补发最新期望配置；版本不变，不补发旧版本动作，Broker 丢失 retained 后可恢复。status.config_version 仍只更新设备报告事实，不伪造配置回执。
- A9：模拟器处理完整配置，提供 success/failed/none，重复设备/版本只重发已有 Ack、不重复应用；同版本不同内容拒绝，旧版本不回退规则。软件组件测试串联 HTTP→模拟发布器→实际软件配置接收器→Ack→GET；不新增 config_update，既有双客户端 WS 通过回归。
- A10：在 go-backend 的 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 均通过，原监控、命令与告警回归通过；README 和网络规范与最终契约一致。真实 Broker/Docker/硬件明确记为未执行，不将软件模拟解释成真实链路通过。

# Constraints and invariants

- 完整目标规格位于 `specs/config-sync/spec.md`，Q1—Q3 已确定，当前等待完整 Shape 最终确认。
- 三种版本事实分开：desired_version 是平台期望；devices.reported_config_version 是 status 上报；device_configs.reported_version 是成功配置 Ack 确认。GET config 使用第三种，设备 API 保留现有 status 语义。
- 合法 success config-ack 才能确认生效；配置不设 10 秒强制失败，不自动回滚规则。
- 当前 Broker persistence=false，平台必须从 SQLite 补发 retained 最新配置，不能假设 Broker 保留内容跨重启存在。
- 同一设备版本规则不可变；先提交数据库，再等待网络；恢复不得递增版本或回退当前配置。
- 网络规范是实现参考，不由本次检查自动扩展为所有功能的需求。重点核对第 5、22、27、35—36、54、66、68—69 节；既有三个 capability 作为回归约束。
- 首轮 go-config-sync 仅有空模板，用户误删了正式目录，doctor 无法从本机 overlay 恢复。残留空目录已保存在 `/tmp/piguard-native-deleted-go-config-sync-20261009`；本机旧 overlay 原样保留。通过官方 new 创建本替代 change，未手写状态文件或恢复虚假的验收进度。

# Decisions

- 用户请求检查现状并使用 Comet 规划下一步；“继续”和误删后的“你继续”沿用该规划目标，不视为确认尚未展示的实现范围。
- 项目 Native、zh-CN、batch；工作区原本干净，沿用当前目录与分支 go后端开发。
- 推荐 P2b 配置确认，依据是三个已归档能力、前两轮路线图、网络规范联调顺序，以及配置代码的实际占位。
- 部分字段合并沿用网络规范第 66 节；阈值校验关系使用完整目标 Spec，不新增任意硬件精度假设。
- 用户于 2026-10-09 明确选择 Q1：要求 expected_version，冲突返回 409；Q2：接受离线设备配置，上线后同步；Q3：通过 GET 查询同步结果。已同步到完整目标规格和 A2—A4/A9。
- 拆分检测：配置记录、发布和 Ack 紧密相关，单个 change 更适合验收；未来图片和运行完善独立推进。
- 沿用已归档记录中“目前不用真正用 docker 和硬件测试”的约束，本次也只做软件检查。

# Open questions

没有未解决的行为选择。Q1—Q3 已明确回答；由 Runtime 保存完整 Shape 摘要并等待最终确认，不把最终确认另写成 blocking 问题。

# Verification expectations

规划检查：核对代码与归档结果，运行既有 test/race/vet，检查 brief 八章节、规格、相对链接和验收对应关系；通过 Runtime 保存完整 Shape 摘要，最终确认前不进入 Build。

Build 检查：严格解析与部分合并、条件版本与事务、先落库后发布、Ack 首个终态、跨设备/乱序/快速 Ack、并发 retained 顺序、重启和订阅恢复、软件配置接收器应有针对性测试。

Verify 检查：新的只读 Verifier 独立判断 A1—A10，使用 Runtime 当前候选实现的检查凭据；真实环境未执行必须保留为限制。

参考：[网络规范](../../../adas_network_api_spec.md)、[实时监控规格](../../specs/realtime-monitoring/spec.md)、[命令规格](../../specs/command-control/spec.md)、[告警规格](../../specs/alarm-events/spec.md)。
