---
generated_from_state_version: 11
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 1
- 迭代: 2
- 验证器尝试次数: 1
- 完成时间: 2026-10-09T12:17:01.332Z
- 摘要: 独立核验当前候选的全部 A1—A10，各项恰好一次，10 项均通过。已核对并复用当前候选绑定的 Runtime test/race/vet 成功证据；A7/A8 通过实现、实际 Paho 依赖源码及 Paho/net.Pipe 软件传输回归核查。未补跑无缺口的全量检查，保留真实环境未执行等限制。Builder handoff 在独立核查后仅作交接对照。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：已登记设备 GET 返回 HTTP 200/code 0、完整四类 rules、desired_version、reported_version、status、ack_at、applied_at、error；未收到成功配置回执时不能虚增 reported_version。未知设备返回 404/40401，不暴露数据库 ID。 | ConfigView 只输出完整四类规则、期望/确认版本和当前版本结果；种子 reported_version=0，未知设备映射 404/40401。HTTP 软件闭环和服务种子测试验证无回执时不伪造确认，不暴露数据库 ID。 |
| A2 | passed | brief.md | A2：合法部分规则 PUT 按当前规则合并，保留未提交字段，并验证完整结果。非法 JSON、空 rules/null、未知字段、错误类型、非法阈值关系、非正 duration_ms 返回 400/40001 和字段路径；不改变版本、不发布。缺失/非法 expected_version 返回 400/40001，过期版本返回 409/40902。 | 协议解析拒绝非法 JSON、尾随内容、null、空规则/子对象、未知字段、错误类型和不可表示数值；显式字段合并后验证完整规则，字段错误映射 400/40001。HTTP/协议测试覆盖字段路径、条件版本缺失、过期 409/40902，拒绝路径未创建版本或发布。 |
| A3 | passed | brief.md | A3：按条件版本契约受理合法 PUT，原子分配新版本和完整配置；两次基于相同旧版本的并发更新只有一次成功。HTTP 202/code 0 返回新 config_version 与 pending；Broker publish 不得先于数据库提交，数据库失败返回 500/50001 且不发布。 | UpdateDesired 在单个 SQLite 事务内核对 expected_version、合并、保存新完整规则及 pending 修订，提交成功后才入异步发送队列。并发同旧版本测试仅一次受理；故障触发器验证事务回滚且不发布；HTTP 合法请求返回 202/code 0/new config_version/pending，数据库错误映射 500/50001。 |
| A4 | passed | brief.md | A4：下发内容包含 schema_version=1、device_id、新 config_version、UTC issued_at 和完整 rules；主题 config、QoS 1、retained=true。设备离线仍受理；MQTT 不可用的前置拒绝返回 503/50302，不写新版本。受理后的发布失败/不确定保持 pending，不伪造 success 或设备 error。 | publishLatest 使用已持久签发修订构造 schema_version=1、设备、新版本、UTC issued_at 和完整 rules，以 car/{device_id}/config、QoS1、retained=true 发布。离线设备仍受理；不可用前置检查映射 503/50302 不写入；发布完成不写设备结果，失败/不确定保持 pending。 |
| A5 | passed | brief.md | A5：合法 success Ack 只确认平台曾持久签发的对应设备/版本，保存 applied_at 与平台 ack_at，单调前进 reported_version。合法 failed Ack 保存 error，并保留上一成功版本。GET 当前 status 来自最新期望版本，旧版本结果不能把新版本标成成功或失败。 | ApplyAck 仅匹配已登记设备的持久修订；首个 success 保存 applied_at/平台 ack_at 并以 CASE 单调前进 reported_version，failed 保存 error 不推进确认版本。GET 按最新 desired_version 读取对应结果；服务与 HTTP 软件测试覆盖旧 success 与最新 failed/pending 的隔离。 |
| A6 | passed | brief.md | A6：错误 schema/主题身份/设备归属/未知或未来版本/时间/结果结构的 Ack 不更新；重复相同回执不重复转换，首个有效终态后的冲突回执不覆盖。旧 success Ack 仅在确实确认更高已签发版本时前进 reported_version，不能回退；极速 Ack 不被稍后的 publish 完成覆盖。 | 严格 Ack 解析验证 schema、主题身份、正版本、带时区时间及 success/failed 互斥结构；仓储拒绝未知/未来修订。条件 pending 转换保留首终态，重复相同结果无二次转换、冲突诊断不覆盖；快速 Ack、旧版本及跨设备拒绝测试通过，publish 回调无结果状态写入。 |
| A7 | passed | brief.md | A7：并发 PUT、发布、Ack 与恢复不造成版本重用或旧配置覆盖 retained 新配置。旧版本成功不得改变新版本 pending。GET 规则、版本和结果来自一致持久快照；发送等待不阻塞遥测处理，也不持有 SQLite 事务。 | 每设备版本分配与 GET 快照均使用持久事务；单发送工作过程合并待发送设备并串行发布，网络等待不持有配置 mutex、Monitor 锁或 SQLite 事务。并发版本/旧 Ack/在途发布与 Restore 测试通过；当前 Paho 实际软件传输测试验证取消先关闭旧传输、丢弃旧 inflight，不能在恢复新 retained 后重放旧配置或动作。 |
| A8 | passed | brief.md | A8：平台重启保留规则、版本和回执结果。启动/Broker 重连先恢复 telemetry/status/command-acks/events/config-acks，再补发最新期望配置；版本不变，不补发旧版本动作，Broker 丢失 retained 后可恢复。status.config_version 仍只更新设备报告事实，不伪造配置回执。 | 启动 EnsureRevisions 保留现有版本/终态；Subscriber 核对 telemetry/status/command-acks/events/config-acks 五组 SUBACK 及预期 QoS 后开放发布和 Restore。重连创建新 clean session/Paho 客户端/内存 Store，丢弃旧发送队列；Paho/net.Pipe 测试覆盖 v2/旧动作未确认、并发持久 v3、订阅等待及拒绝、仅恢复 v3、实际 Ack。SQLite 重开测试保留规则、版本、issued_at 和首回执，status 仍只写 devices 报告事实。 |
| A9 | passed | brief.md | A9：模拟器处理完整配置，提供 success/failed/none，重复设备/版本只重发已有 Ack、不重复应用；同版本不同内容拒绝，旧版本不回退规则。软件组件测试串联 HTTP→模拟发布器→实际软件配置接收器→Ack→GET；不新增 config_update，既有双客户端 WS 通过回归。 | ConfigExecutor 解析完整配置，支持 success/failed/none；按设备/版本缓存结果，重复重发 Ack 不重复应用，同版本内容冲突拒绝，旧版本不回退生效规则。HTTP 软件组件测试真实串联 handler→ConfigManager→模拟发布器→ConfigExecutor→Ack→GET 三模式；不发送 config_update，既有监控/命令/告警双 WS 回归包含在有效全量检查中。 |
| A10 | passed | brief.md | A10：在 go-backend 的 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 均通过，原监控、命令与告警回归通过；README 和网络规范与最终契约一致。真实 Broker/Docker/硬件明确记为未执行，不将软件模拟解释成真实链路通过。 | 核对 Runtime state/logs：candidateId、状态版本、输入指纹 gate、项目/worktree/分支及检查 operation 对应当前候选；go test -count=1 ./...、go test -race -count=1 ./...、go vet ./... 在 go-backend 均 exit 0，日志覆盖原业务及新增配置/传输测试。本轮复用有效记录未重跑全量。README 与网络规范配置章节对应最终契约，并明确软件证据及真实环境未执行。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go 全量测试 | GOCACHE=/tmp/piguard-go-build-cache go test -count=1 ./... | go-backend | passed | 0 | 6083 ms |
| Go 全量竞态检查 | GOCACHE=/tmp/piguard-go-build-cache go test -race -count=1 ./... | go-backend | passed | 0 | 7811 ms |
| Go vet | GOCACHE=/tmp/piguard-go-build-cache go vet ./... | go-backend | passed | 0 | 389 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- MQTT 配置与业务定向竞态检查: passed — env GOCACHE=/tmp/piguard-go-build-cache go test -race ./internal/mqtt ./internal/service -run 'TestMQTT|TestSubscriber|TestPublisher|TestConfig' -count=1 通过；含实际 Paho/net.Pipe 重连丢弃 inflight、仅恢复最新、订阅拒绝和取消。
- 格式与补丁检查: passed — 修改 Go 源码已 gofmt；git diff --check 通过。
- 已知限制: 真实 Mosquitto、Docker 与硬件仍按已确认范围未执行；net.Pipe 是软件传输证据。
- 已知限制: 受理后发布不确定保持 pending；没有通用 outbox 或所有历史版本持续重试，仅后续更新、重连和重启恢复最新配置。
- 已知限制: 未增加图片、摄像头、鉴权或配置 WebSocket 类型。

## 阻塞项

_无。_

## 风险与跳过的工作

- 真实 Mosquitto、Docker 与树莓派硬件未执行；Paho/net.Pipe 与 HTTP/模拟器结果仅是软件范围证据，不能作为真实 Broker 或设备链路通过。
- 受理后发送失败或不确定可以长期 pending；恢复只重建各设备最新持久期望配置，没有通用 outbox 或所有历史版本的持续定时重试。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | fail | A7, A8 | 独立核验 A1—A10，各项恰好一次：8 项通过，A7/A8 未通过。三项正式 Go 检查与当前候选绑定并全部成功，未重复执行。必须修复 Paho 自动重连对历史 QoS1 retained 配置的无序/并发重放，保证五组订阅就绪后仅恢复最新配置且旧 retained 不覆盖新版本，再提交候选验收。Builder 交接只在最后用作调查线索，没有作为正式证据。 | 2026-10-09T11:55:21.388Z |
| 1 | 2 | 1 | pass | — | 独立核验当前候选的全部 A1—A10，各项恰好一次，10 项均通过。已核对并复用当前候选绑定的 Runtime test/race/vet 成功证据；A7/A8 通过实现、实际 Paho 依赖源码及 Paho/net.Pipe 软件传输回归核查。未补跑无缺口的全量检查，保留真实环境未执行等限制。Builder handoff 在独立核查后仅作交接对照。 | 2026-10-09T12:17:01.332Z |



## 结论

独立核验当前候选的全部 A1—A10，各项恰好一次，10 项均通过。已核对并复用当前候选绑定的 Runtime test/race/vet 成功证据；A7/A8 通过实现、实际 Paho 依赖源码及 Paho/net.Pipe 软件传输回归核查。未补跑无缺口的全量检查，保留真实环境未执行等限制。Builder handoff 在独立核查后仅作交接对照。
