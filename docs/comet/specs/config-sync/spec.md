# 配置同步与确认

这是 go-config-confirmation 的完整目标规格。用户已选择条件版本校验、离线受理和 GET 查询结果；完整 Shape 仍待最终确认。本规格描述待实现目标，不代表已有实现。

## R1：能力边界与版本事实

实现 GET/PUT `/api/v1/devices/{device_id}/config`、MQTT config/config-acks、软件配置接收器和启动/重连恢复。使用 schema_version=1、UTC RFC3339Nano、现有 HTTP 响应信封。

| 字段 | 事实来源 | 更新条件 |
| --- | --- | --- |
| desired_version | 平台期望规则版本 | 合法 PUT 原子递增；恢复重发不递增 |
| reported_version | 配置 Ack 确认的生效版本 | 平台签发版本的合法 success Ack，单调前进 |
| 设备 API config_version | 设备 status 的报告 | 保留实时监控现有语义，不修改 config Ack 结果 |

默认种子规则和 desired_version=1 保留；没有成功配置回执时 reported_version=0。启动可持久登记并发布尚未具有版本记录的种子配置，不能凭种子把它标记成功。

配置不属于普通 commands；不生成 command_id、不使用 command_update、不采用动作命令 10 秒截止。pending 表示等待当前期望版本确认，可一直等待离线设备。

## R2：HTTP GET

已登记设备返回 HTTP 200/code 0：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "desired_version": 2,
    "reported_version": 1,
    "rules": {
      "obstacle": {"warning_distance_m": 15, "danger_distance_m": 8},
      "lane_departure": {"offset_threshold": 0.35, "duration_ms": 1000},
      "sharp_turn": {"yaw_threshold_deg_s": 30, "min_speed_kmh": 20, "duration_ms": 300},
      "temperature": {"trigger_c": 35, "recover_c": 33, "duration_ms": 5000}
    },
    "status": "pending",
    "ack_at": null,
    "applied_at": null,
    "error": null
  }
}
```

rules 是完整的当前期望配置。status 为该 desired_version 的 pending/success/failed；ack_at 为平台收到首个有效结果的时间；applied_at 仅 success 有值；error 仅 failed 有值。初始未知的 Ack 字段为 null，不伪造时间。

未知设备返回 404/40401，数据库故障为 500/50001。查询来自一致持久快照，不暴露 GORM、自增 ID 或内部发布队列。

status/reported_version/ack_at/applied_at/error 的可观察契约是对网络规范第 35 节的补充，Build 时同步说明。

## R3：PUT、部分合并与校验

按网络规范第 66 节允许部分规则，采用用户已选择的条件版本契约：

```json
{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}}}
```

JSON 必须是一个对象，仅允许 expected_version 和 rules；拒绝尾随 JSON、未知字段、null、空 rules、空的规则子对象、错误类型和非有限数值。只修改显式提交的字段，省略值保留当前配置；合并后验证全部规则。显式 0 与省略不同，不用 Go 零值判断是否提供。

| 规则 | 合法关系 |
| --- | --- |
| obstacle | `0 < danger_distance_m < warning_distance_m` |
| lane_departure | `0 < offset_threshold <= 1`，duration_ms 为正整数 |
| sharp_turn | yaw_threshold_deg_s > 0，min_speed_kmh >= 0，duration_ms 为正整数 |
| temperature | recover_c < trigger_c，duration_ms 为正整数，温度可为负值 |

各字段必须可由服务端类型安全表示；不新增未说明的传感器量程上限。非法请求返回 400/40001，error.field 使用如 rules.obstacle.danger_distance_m 的字段路径，不创建版本、不发布。

expected_version 为正整数且等于当前 desired_version；缺失/非法返回 400/40001，过期返回 409/40902。事务内核对版本、读取并合并规则、递增 desired_version、保存新版本完整内容和 pending。同旧版本并发请求只能一个成功。

每次有效受理都创建下一版本，包括提交值相同的请求；不引入 HTTP 幂等键或同值折叠。显式再次提交可作为设备 failed 后创建新版本重试。

数据库提交后返回 HTTP 202/code 0，data 为 `{config_version:<新版本>,status:"pending"}`。后续 GET 可能已经是 success/failed。202 只表示已持久受理，不承诺网络交付或规则生效。

## R4：受理与网络发送

设备 online=false 时也接受合法配置，前提是 MQTT 连接与配置工作过程可受理；retained 保留最新规则供设备上线接收。MQTT 不可用前置拒绝返回 503/50302，不落库。未知设备返回 404/40401。

先落库，再使用有界、可停止的异步发送工作过程发布：

```json
{
  "schema_version": 1,
  "device_id": "car-001",
  "config_version": 2,
  "issued_at": "2026-10-09T12:00:00Z",
  "rules": {"obstacle":{"warning_distance_m":20,"danger_distance_m":8},"lane_departure":{"offset_threshold":0.35,"duration_ms":1000},"sharp_turn":{"yaw_threshold_deg_s":30,"min_speed_kmh":20,"duration_ms":300},"temperature":{"trigger_c":35,"recover_c":33,"duration_ms":5000}}
}
```

主题 `car/{device_id}/config`，QoS 1，retained=true。发送成功只代表 Broker 确认，不改变 pending 为 success。

受理后发送失败/超时/取消时，记录发送诊断，保持 pending，不制造设备 failed。当前进程后续成功重连和平台重启会补发最新配置；不承诺持续定时重试所有历史版本，也不建设通用 outbox。

同设备发布要序列化并合并到最新期望版本。较旧的在途发布结束后，新版本必须随后发布；新 retained 值生效后不能再被旧任务覆盖。等待 MQTT 时不持有数据库事务或 Monitor 锁。

## R5：持久版本与 Ack

除当前 device_configs 外，需要能持久核对每个实际签发版本的完整不可变规则、issued_at、状态和结果。可新增以 device_id/config_version 唯一的专用版本记录；具体仓储和迁移结构由实现决定。不能只凭整数处于 1..desired_version 就把未知 Ack 当成已签发。

订阅 `car/+/config-acks`，QoS 1。软件设备发布回执 QoS 1、retained=false。

成功结构：

```json
{"schema_version":1,"device_id":"car-001","config_version":2,"status":"success","applied_at":"2026-10-09T12:00:01Z"}
```

失败结构：

```json
{"schema_version":1,"device_id":"car-001","config_version":2,"status":"failed","error":{"code":"INVALID_CONFIG","message":"invalid threshold"}}
```

严格校验单个 JSON 对象、允许字段、schema_version=1、主题/payload 设备一致、设备已登记、正整数版本属于该设备已签发记录、status 为 success/failed。success 必须有合法带时区 applied_at，归一 UTC，不带 error；failed 必须有非空 error.code/message，不带 applied_at。失败回执没有设备执行时间，ack_at 使用平台接收时间。

每版本首个有效 Ack 从 pending 进入 success/failed，持久事务内转换。相同重放不重复转换；其后的冲突结果保留首条并诊断。不能由晚到 publish 回调覆盖快速 Ack。

success 更新 confirmed reported_version 为此前确认版本与该成功版本的较大值。older success 可以补充之前未确认的已签发版本事实，但不能回退 reported_version、最新规则或最新版本状态。failed 不推进 reported_version。未来/未知/跨设备版本拒绝。

最新 desired_version 的 GET status 只读取该版本结果。设备状态消息、在线/offline 和 telemetry 不修改该结果；config Ack 不修改 online/last_seen_at 或风险规则计算。

## R6：恢复与设备幂等

启动/Broker 重连先恢复五组上行订阅：telemetry（QoS 0）、status、command-acks、events、config-acks（其余 QoS 1）。确保 Ack 接收就绪后再补发每台登记设备最新已持久期望配置，含已成功或失败的最新版本，以重建 Broker retained 内容。补发沿用版本、规则和 issued_at，不创建新配置版本，也不重发动作命令。

设备按 device_id/config_version 去重。同版本同内容只重发已缓存结果，不重新应用；同版本不同内容拒绝，不能覆盖已缓存配置；更旧版本不能回退规则。设备对新版本 success 才更新模拟生效版本，failed 保留旧规则。success/failed/none 与延迟回执必须可测试。

关闭时停止发送与回调，保存已提交的配置，不遗留无限等待。重启后 confirmed 版本和每版本首个回执保留，不因重新发布改回 pending。

## R7：结果观察

GET 提供完整当前状态；本 capability 不增加 WS 类型，既有 WS 四种业务消息保持回归通过。客户端可再次 GET 查询 pending/success/failed，不把 HTTP 202 当成设备生效。

## R8：验收与非目标

brief A1—A10 是本 capability 验收索引，不在本 Spec 添加重复 Scenario。要求针对校验、事务/条件版本、发布顺序、首个 Ack、版本事实、恢复和实际软件配置接收器做测试，运行 Go test/race/vet，并回归监控、命令与告警。

没有真实 Broker、Docker 或树莓派硬件验收；这些留给后续环境联调，不能写为已通过。没有图片、摄像头命令、Vue、Python、规则引擎、设备注册或鉴权改造。完整 Shape 最终确认前不进入 Build。
