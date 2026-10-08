---
generated_from_state_version: 8
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 1
- 迭代: 1
- 验证器尝试次数: 1
- 完成时间: 2026-09-30T09:24:59.783Z
- 摘要: 独立只读核验当前候选全部 A1—A9，9 项通过，无失败或阻塞。已读完整 brief/目标 Spec、Runtime 全部 scope、协议来源、实现与相关测试，最后核对 Builder 交接；复用与当前候选/工作区/输入绑定的四项有效 Runtime 正式检查，未重复完整套件。真实 Broker 生命周期检查通过，Docker Compose 实际容器启动的环境限制及协议 v1 时钟依赖已保留为风险。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：已登记设备发布合法 telemetry 后，最新状态包含协议规定的速度、映射距离、温度、角速度、车道与风险，并保留采样时间、来源和传感器状态；主题与 payload 设备 ID 不符、非法 JSON 或不支持的版本不会更新任何业务状态。 | 协议 DTO 保留各信号采样时间/source/status、映射距离、车道和风险，协议解析在任何业务写入前校验 JSON、版本及主题设备身份；ParseTelemetry/Monitor 实现与 go-tests/go-race 中协议及监控测试、mqtt-integration 的实际距离/采样元数据检查相符。 |
| A2 | passed | brief.md | A2：接收 online status 后设备列表与详情显示在线及设备元数据；接收没有 timestamp 的离线遗嘱后显示离线；MQTT 重连完成后，上行接收自动恢复。 | status 更新已登记设备在线事实及元数据，时间戳缺省的离线遗嘱有效且不清空原元数据；Subscriber 首次订阅须成功，Paho 重连回调恢复 telemetry/status 订阅；mqtt-integration 实证离线遗嘱、Broker loss 和重启后自动接收恢复。 |
| A3 | passed | brief.md | A3：先发送较新的遥测，再发送较旧的遥测，HTTP 最新状态和 WS 均不回退；采样时间更新但 seq 从零重启的消息仍可成为最新状态。 | Monitor 在最新状态/HTTP/WS/历史写入前按 timestamp 优先、同时间 seq 严增比较，旧及重复消息直接返回；新 timestamp 下 seq=0 被接受。go-tests/go-race 的监控排序测试及 mqtt-integration 的乱序和 seq reset 检查通过，旧消息不会走 Publish。 |
| A4 | passed | brief.md | A4：设备启动后尚无遥测时，state 返回明确的空值/unknown/missing；距离、车道、车速和陀螺仪超过 1 秒、温度超过 5 秒时标记 stale，故障状态与设备离线分别表示，不能把缺失或故障数据映射成正常的 0。 | 空状态以 null、unknown、missing 表达；View 按每信号 sample_at 对 speed/distance/gyro/lane 应用 1 秒 TTL、temperature 应用 5 秒 TTL，非 ok 故障与 online 分开且故障/缺失值为 null。协议 TTL 边界、故障和空值测试及真实链路 expiry/fault 检查通过。 |
| A5 | passed | brief.md | A5：同一设备按 2Hz 连续发送 10 秒的合法遥测，最新状态持续更新，历史写入频率不超过 1Hz；不同设备独立计算降频，历史数值按协议单位正确映射，重复和旧消息不会重复写入。 | 每设备状态独立维护平台接收时间水位，历史写入间隔至少 1 秒，最新状态与广播每条合法新遥测都更新；原始 DTO 保存采样元数据，mapped_value 按 m 入库，旧/重复消息及重启历史水位避免重复。监控测试以 20 条 2Hz 遥测分别验证两设备各 10 条历史，真实 10 秒 Broker 链路也满足不超过 1Hz。 |
| A6 | passed | brief.md | A6：四个查询 API 返回协议信封及真实业务数据；不存在的设备返回 HTTP 404/40401；非法时间、start 大于 end 或非法 limit 返回 HTTP 400/40001；历史查询按设备、时间范围和限制返回稳定顺序的数据。 | 四查询 API 使用明确 DTO 与 code/message/data 信封；未知设备映射 404/40401，非法 start/end、start>end 与 limit 映射 400/40001 和字段；历史按设备及闭区间过滤，先取 timestamp/seq/ID 最新 limit，再逆转为稳定升序。HTTP 与仓储正式检查覆盖四接口、错误、范围和限制，读取实现一致。 |
| A7 | passed | brief.md | A7：两个 WS 客户端可以收到 telemetry 与 device_status 的统一信封；断开或持续不读取的客户端不妨碍另一客户端收取后续消息，也不阻塞 MQTT 接收；过期遥测不会作为新的正常数据推送。 | Hub 生成 type/timestamp/data UTC 事件信封；每连接有限队列、队列满即驱逐，单写入过程有 5 秒写超时和存活检测，读/写失败及 Hub 关闭均清理。Hub 饱和队列测试证明慢订阅不阻塞健康订阅，两个实际 WS 的状态/遥测、断开后继续接收及 shutdown 测试通过；过期/旧消息不作为新正常数据广播，TTL 变化发送 stale 视图。 |
| A8 | passed | brief.md | A8：平台启动和重启保持已有演示设备及期望/已报告配置版本；设备状态在新的 status 到来前视为未确认离线，最新遥测等待新消息，已保存历史仍可查询；MQTT status 中的配置版本只更新设备报告事实，不假装已收到配置成功 Ack。 | Seed 对现有设备和配置仅查询，保留名称及 desired/reported 版本；启动/失去 Broker 时仅作在线事实失效，Monitor 重建不重放历史为最新状态。status.config_version 只改 devices.reported_config_version，不改 device_configs desired/reported 或生成 Ack；服务恢复/配置断言及实际平台重启后空最新状态、历史保留检查通过。 |
| A9 | passed | brief.md | A9：可用现有 Compose Broker 和模拟发布器，在无树莓派及 Vue 页面的环境完成 telemetry/status→HTTP/WS 的联调；必要的 Go 测试、竞态检查和静态检查全部成功，异常消息不会导致进程退出。 | Compose MQTT/HTTP 端口及配置与模拟器匹配，cmd/simulator 可发布 retained online/status、遗嘱及 2Hz 完整 DTO，README 提供模拟发布及 HTTP/WS 读取步骤。当前候选 go-tests、go-race、go-vet、mqtt-integration 四项 Runtime 正式检查全部通过；真实 Mosquitto→实际后端进程→HTTP/两个 WS 链路、平台/Broker 重启和异常版本/故障消息均验证成功。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go 全部单元和组件测试 | GOCACHE=/tmp/piguard-go-cache go test -count=1 ./... | go-backend | passed | 0 | 1841 ms |
| Go 全部竞态检查 | GOCACHE=/tmp/piguard-go-cache go test -race -count=1 ./... | go-backend | passed | 0 | 29849 ms |
| Go 静态检查 | GOCACHE=/tmp/piguard-go-cache go vet ./... | go-backend | passed | 0 | 4445 ms |
| 真实 Mosquitto 后端生命周期与竞态联调 | GOCACHE=/tmp/piguard-go-cache MOSQUITTO_BIN=/tmp/piguard-mosquitto/usr/sbin/mosquitto LD_LIBRARY_PATH=/tmp/piguard-mosquitto/usr/lib/x86_64-linux-gnu go test -race -tags integration ./internal/server -count=1 -v | go-backend | passed | 0 | 18410 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- 协议/Hub/状态/存储定向测试: passed — GOCACHE=/tmp/piguard-go-cache go test ./internal/protocol ./internal/realtime ./internal/service ./internal/repo
- HTTP/WS 定向测试与入口编译: passed — handler/server 定向测试、cmd/simulator 和 cmd/server 编译通过；WS 需要本地网络权限
- 真实 Broker 开发联调: passed — Mosquitto 2.0.18 临时解包运行；TestMonitoringBrokerLifecycle 通过，含 2Hz/1Hz、两个 WS、遗嘱、TTL、seq 归零和平台/Broker 重启；之后补强 Broker loss 与子进程退出检查，最终交 Runtime 复验
- 完整正式检查计划: not-run — 由 Runtime 冻结本轮候选后执行以下四项计划
- 已知限制: 协议 v1 没有 boot_id；按已确认规格以设备 timestamp 为主排序，设备时钟回拨期间不会覆盖既有最新状态。
- 已知限制: 本机无 Docker daemon；真实 Broker 验收使用 /tmp 下 Mosquitto 2.0.18 和依赖库。集成测试在其他机器上需要可执行的 MOSQUITTO_BIN。
- 已知限制: 本 change 按已确认范围只实现实时监控；命令、事件、配置写入、图片接口保留原有 501。

## 阻塞项

_无。_

## 风险与跳过的工作

- 协议 v1 无 boot_id，顺序依赖设备 UTC timestamp；设备时钟回拨期间不会覆盖当前最新状态，这是目标规格已明确的限制。
- 本机无 Docker daemon，未执行 Docker Compose 的容器启动；正式联调使用临时解包的真实 Mosquitto 2.0.18 与实际后端进程，已核对 Compose 端口和配置。其他环境运行真实 Broker 集成检查须提供可运行的 MOSQUITTO_BIN。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | 独立只读核验当前候选全部 A1—A9，9 项通过，无失败或阻塞。已读完整 brief/目标 Spec、Runtime 全部 scope、协议来源、实现与相关测试，最后核对 Builder 交接；复用与当前候选/工作区/输入绑定的四项有效 Runtime 正式检查，未重复完整套件。真实 Broker 生命周期检查通过，Docker Compose 实际容器启动的环境限制及协议 v1 时钟依赖已保留为风险。 | 2026-09-30T09:24:59.783Z |



## 结论

独立只读核验当前候选全部 A1—A9，9 项通过，无失败或阻塞。已读完整 brief/目标 Spec、Runtime 全部 scope、协议来源、实现与相关测试，最后核对 Builder 交接；复用与当前候选/工作区/输入绑定的四项有效 Runtime 正式检查，未重复完整套件。真实 Broker 生命周期检查通过，Docker Compose 实际容器启动的环境限制及协议 v1 时钟依赖已保留为风险。
