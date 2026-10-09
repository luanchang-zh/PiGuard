---
generated_from_state_version: 13
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 2
- 迭代: 1
- 验证器尝试次数: 3
- 完成时间: 2026-10-09T04:21:42.609Z
- 摘要: 告警事件上行在软件路径上满足 A1–A7：已登记设备的合法消息按全局 event_id 幂等入库，HTTP 筛选与双 WebSocket 推送来自同一持久记录，模拟器以 QoS 1、非 retained 重复发布同一事件。Runtime 在 go-backend 执行的 go test、go test -race 与 go vet 退出码均为 0。真实 Mosquitto、Docker 和硬件本轮未执行，验收结论只覆盖软件路径。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：已登记设备向 `car/{device_id}/events` 发布五类 type、三种 action、level 为 warning 或 danger 的合法消息后，各保存一行。GET 返回 HTTP 200/code 0，items 含 event_id、type、action、level、UTC timestamp、data 对象和 snapshot_id。主题 QoS 1、retained=false。 | uplinkFilters 将 car/+/events 订为 QoS 1，TestSubscriberRestoresAckTopicOnReconnect 断言四个主题且 events 的 QoS 为 1。PublishDemoEvent 与模拟器以 qos=1、retained=false 发布。协议测试接受五类 type、三种 action、warning/danger，并把 +08:00 转成 UTC。HTTP 测试写入后 GET 返回 200/code 0，条目含 event_id、type、action、level、timestamp、data 和 snapshot_id。 |
| A2 | passed | brief.md | A2：非法 JSON、schema_version 不是 1、主题与 payload 设备不一致、空或超长 event_id、未知 type/action/level、非法时间、data 不是对象、未知顶层字段、未知设备，都不插入、不广播，进程不退出。设备当前离线时，合法事件仍入库。telemetry 里的 active_events 不产生 events 行，事件也不改变 online 或 last_seen_at。 | 协议测试拒绝非法 JSON、错误版本、设备不一致、空或超长 event_id、未知 type/action/level、非法时间、非对象 data 和未知顶层字段。解析失败或未知设备不插入、不广播。HTTP 测试在非法消息后仍为 6 行且没有新 WS 帧。遥测 active_events 不产生 events 行；离线时合法事件仍入库，且不改变 online 与 last_seen_at。 |
| A3 | passed | brief.md | A3：同一 event_id 且设备、类型、动作、等级、时间、data、snapshot_id 一致的重放仍只有一行，并且不第二次广播。同一 event_id 的内容不同，或该 ID 被另一设备复用时，保留第一行，不覆盖、不广播。 | event_id 冲突时 DoNothing。内容一致不广播，内容不同或跨设备复用只保留首条。并发重放测试只有 1 行和 1 次广播。HTTP 测试重放、改内容和 car-002 复用同一 ID 后总数仍是 6，原记录保持 warning 与 distance_m=6.8，且没有第二帧。 |
| A4 | passed | brief.md | A4：未知设备的 GET 返回 HTTP 404/code 40401。非法 type、level、start、end、limit，或 start 大于 end，返回 HTTP 400/code 40001，并标明字段。limit 默认 100，允许 1—1000。时间过滤包含端点，条件同时生效。无记录时 items 为 []。结果按 timestamp 降序、event_id 降序。时间更早但 event_id 不同的消息仍会入库。响应不含数据库自增 ID。 | 未知设备返回 404/40401。非法 type、level、时间、start 晚于 end 和越界 limit 返回 400/40001 并标明字段。空列表为 []。过滤包含端点且可组合。结果按 timestamp 降序、event_id 降序。更早的新消息仍入库。默认 limit 100，最大 1000。响应不含数据库自增 ID。 |
| A5 | passed | brief.md | A5：只有首次成功插入才向已连接客户端发送 `{type:"event",timestamp,data}`。data 含 event_id、device_id、event_type、action、level、设备 timestamp、data 和 snapshot_id。两个客户端都能收到；新建连接不重放旧事件。持续不读取的连接被断开，且不阻塞 MQTT 或其他客户端。 | 只有插入成功才广播 event。两个客户端都收到完整 data，含 event_type、设备时间和 snapshot_id。重放、冲突和非法消息不再推送。新建连接不重放历史。慢客户端会被断开，且不阻塞其他客户端和入库。 |
| A6 | passed | brief.md | A6：消息带合法 snapshot_id 时，GET 和 WS 返回该值；省略时为 null。snapshot_available 可以出现，但不入库、不返回。本轮图片上传和读取仍不可用。平台重启后已存事件仍可按 A4 查询；Broker 重连后同时恢复 events、telemetry、status、command-acks，且不把旧事件补推到 WS。 | 合法 snapshot_id 会出现在 GET 和 WS，省略时为 null。snapshot_available 不入库、不返回。图片接口仍是 501。重启后历史仍可按 A4 查询，新连接不补推旧事件。重连恢复 events、telemetry、status 和 command-acks。 |
| A7 | passed | brief.md | A7：在 go-backend 运行 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 通过。软件测试覆盖合法入库、重复、冲突、非法拒绝、筛选和双 WS。模拟器能以 QoS 1、非 retained 发布一条事件并重发同一 event_id。既有监控与命令测试保持通过。本轮不启动 Docker、不测硬件，真实 Mosquitto 未执行不算失败。 | Runtime 正式结果：go test -count=1 ./...、指定 gcc 的 go test -race -count=1 ./... 和 go vet ./... 退出码都是 0。软件测试覆盖合法入库、重复、冲突、非法拒绝、筛选和双 WS。模拟器以 QoS 1、非 retained 重发同一 event_id。既有监控与命令测试保持通过。真实 Mosquitto、Docker 和硬件未执行，按规格不记失败。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go 全部测试 | test -count=1 ./... | go-backend | passed | 0 | 15283 ms |
| Go 全部竞态检查 | -lc export CC='C:/Users/23156/AppData/Local/Temp/piguard-race-toolchain/w64devkit/bin/gcc.exe' && go test -race -count=1 ./... | go-backend | passed | 0 | 23630 ms |
| Go vet | vet ./... | go-backend | passed | 0 | 2295 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- go test -count=1 ./...: passed — 在 go-backend 全部包通过，覆盖协议拒绝、离线补传、重复与冲突、筛选排序、双 WS、重启不重放、默认 limit，以及既有监控和命令测试。
- go test -race -count=1 ./...: passed — 默认 MinGW 8.1 无法启动 Go 1.25 竞态运行时。使用 CC=C:/Users/23156/AppData/Local/Temp/piguard-race-toolchain/w64devkit/bin/gcc.exe 后全部包通过。
- go vet ./...: passed — go vet ./... 无输出。
- 已知限制: 未启动 Docker、真实 Mosquitto 或硬件。数据库写入失败后的 QoS 1 恢复依赖发送方重放，当前 MQTT 客户端仍自动确认。
- 已知限制: 图片上传和读取仍返回未实现。多次生命周期消息不会合并成可变告警实例。

## 阻塞项

_无。_

## 风险与跳过的工作

- 真实 Mosquitto 上的订阅恢复与 QoS 1 投递，以及 Docker、硬件，本轮都没有执行；现有证据来自进程内假客户端、SQLite、HTTP 和 WebSocket。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 0 | recovery | — | Native Shape artifacts changed | 2026-10-08T14:27:06.392Z |
| 2 | 1 | 3 | pass | — | 告警事件上行在软件路径上满足 A1–A7：已登记设备的合法消息按全局 event_id 幂等入库，HTTP 筛选与双 WebSocket 推送来自同一持久记录，模拟器以 QoS 1、非 retained 重复发布同一事件。Runtime 在 go-backend 执行的 go test、go test -race 与 go vet 退出码均为 0。真实 Mosquitto、Docker 和硬件本轮未执行，验收结论只覆盖软件路径。 | 2026-10-09T04:21:42.609Z |



## 结论

告警事件上行在软件路径上满足 A1–A7：已登记设备的合法消息按全局 event_id 幂等入库，HTTP 筛选与双 WebSocket 推送来自同一持久记录，模拟器以 QoS 1、非 retained 重复发布同一事件。Runtime 在 go-backend 执行的 go test、go test -race 与 go vet 退出码均为 0。真实 Mosquitto、Docker 和硬件本轮未执行，验收结论只覆盖软件路径。
