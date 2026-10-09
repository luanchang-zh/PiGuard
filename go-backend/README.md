# Go 监控与控制后端

在 `go-backend` 目录运行。已实现 telemetry/status MQTT 接入、最新状态、历史查询、动作命令/Ack、告警事件、配置同步、JPEG 图片上传/读取与 WebSocket。

## 软件验收

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

默认测试使用临时 SQLite、模拟发布器、实际软件设备执行器、HTTP/WS 客户端及 Paho/net.Pipe 软件 MQTT 传输，覆盖命令成功/失败/超时、参数与身份校验、重复回执、快速 Ack、持久恢复、告警事件幂等/筛选/推送、配置并发版本/回执/恢复、图片并发幂等/乱序预览/存储失败/重启/双 WS 及监控回归。无需 Docker、真实 Broker 或硬件。实际 MQTT 网络与设备执行行为需在后续环境联调确认。

Windows 的竞态检查需要启用 CGo，并使用包含 MinGW-w64 8 或更新运行库的 C 编译器，见 [Go 官方要求](https://go.dev/doc/articles/race_detector#Requirements)。使用便携工具链时，可在 Git Bash 中仅为本次检查指定编译器：

```sh
CC='C:/path/to/w64devkit/bin/gcc.exe' go test -race -count=1 ./...
```

## Broker 环境联调（后续验证）

启动 Broker 与后端：

```sh
docker compose up --build
```

另开终端，发布演示设备的在线状态与 2Hz 遥测：

```sh
go run ./cmd/simulator -duration 30s
```

模拟器正常结束会发布离线状态；异常断连由 Broker 发布离线遗嘱。可用 `-broker tcp://host:1883` 和 `-device car-001` 选择地址及已登记设备，未知设备不会自动注册。

查看查询结果：

```sh
curl http://127.0.0.1:8080/api/v1/devices
curl http://127.0.0.1:8080/api/v1/devices/car-001
curl http://127.0.0.1:8080/api/v1/devices/car-001/state
curl 'http://127.0.0.1:8080/api/v1/devices/car-001/telemetry?limit=10'
```

浏览器开发者控制台可查看实时事件：

```js
const ws = new WebSocket('ws://127.0.0.1:8080/api/v1/ws');
ws.onmessage = event => console.log(JSON.parse(event.data));
// 使用完后：ws.close();
```

也可启动独立 Mosquitto，然后 `go run ./cmd/server -config config.yaml`。配置文件中的相对路径以该文件目录为基准，`PIGUARD_*` 环境变量可覆盖配置。

## 动作命令

模拟器在线后可创建命令：

```sh
curl -X POST http://127.0.0.1:8080/api/v1/devices/car-001/commands \
  -H 'Content-Type: application/json' \
  -d '{"type":"buzzer.test","params":{"duration_ms":1000}}'
curl http://127.0.0.1:8080/api/v1/commands/返回的command_id
```

- 四类动作：buzzer.test、indicator.test、scenario.start、scenario.stop。蜂鸣器/灯时长 100—5000ms，灯色 green/yellow/red；场景名称与播放速度遵循网络规范。摄像头/配置不走本轮命令接口。
- POST 返回 202/pending，表示已保存并异步受理；MQTT QoS 1、non-retained。Broker 确认后为 sent，设备回执决定 success/failed；10 秒内未收到有效回执则 timeout。
- GET 和 WS command_update 使用同一 DTO，包括参数、签发/截止、平台 ack_at、设备 executed_at 和 result/error。缺少回执的时间为 null。
- 离线设备拒绝为 503/50301；在线但发布通道不可用为 503/50302。每次有效 POST 创建新命令，没有 HTTP 重试幂等键。
- 快速/重复/冲突/迟到 Ack 不回退终态。重启保留记录、恢复截止时间，不自动重发动作。
- 模拟器 `-ack-mode success|failed|none` 选择结果，`-ack-delay 12s` 可模拟迟到回执；它按 command_id 缓存结果，重复投递只重发回执，不重复模拟执行。

## 配置同步

```sh
curl http://127.0.0.1:8080/api/v1/devices/car-001/config
curl -X PUT http://127.0.0.1:8080/api/v1/devices/car-001/config \
  -H 'Content-Type: application/json' \
  -d '{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}}}'
```

- PUT 的 expected_version 使用刚读取的 desired_version；旧版本返回 409/40902。支持部分规则字段，合并后校验完整四类规则；未知字段、null、空规则或不合法的阈值返回 400/40001 和字段路径。每次有效受理创建下一版本，包括值相同的请求。
- HTTP 202/pending 表示完整规则和版本已持久保存。设备离线仍可提交；MQTT 不可用则在写入前返回 503/50302。异步发送失败或结果不确定保持 pending，等待重连或重启补发。
- 配置独立发布到 car/{device_id}/config，QoS 1、retained=true。合法 config-acks success 才推进 reported_version；failed 保存设备 error，保留上一成功版本。配置不采用动作命令的 10 秒截止。
- GET 返回完整期望 rules、desired_version、Ack 确认的 reported_version、当前版本的 pending/success/failed、ack_at、applied_at、error；没有回执的字段为 null。通过 GET 查询结果，本轮没有 config_update WS 类型。
- 设备接口的 config_version 仍是 status 报告，和配置 Ack 确认事实分开。每版本首个有效结果保持不变，旧版本 Ack 不改变当前版本结果或回退已确认版本。
- 平台启动及 Broker 重连先恢复五组上行订阅，再补发每设备最新配置，版本和 issued_at 不变。即使 Broker 未持久保存 retained 数据，也可从 SQLite 恢复。补发不触发动作命令重发。
- 每次重连使用新的 clean session，丢弃旧 MQTT 客户端的未确认发送队列；只有五组订阅获得预期 QoS 后才恢复发布。发布不确定或取消会先关闭旧传输，再建立新会话，避免旧 retained 配置晚于新配置重放。
- 模拟器新增 `-config-ack-mode success|failed|none` 与 `-config-ack-delay 2s`。它按设备/版本缓存结果；重复配置只重发回执，同版本内容冲突和旧版本不回退规则。none 会应用配置但不回 Ack，用于模拟确认丢失。

## 图片上传与读取

```sh
curl -X POST http://127.0.0.1:8080/api/v1/devices/car-001/frames \
  -F 'file=@preview.jpg' \
  -F 'frame_id=preview-001' \
  -F 'captured_at=2026-10-09T12:00:00.123Z' \
  -F 'type=preview'
curl http://127.0.0.1:8080/api/v1/devices/car-001/frame
curl http://127.0.0.1:8080/api/v1/snapshots/返回的snapshot_id/content -o downloaded.jpg
```

- multipart 严格包含一个 `file/frame_id/captured_at/type`，拒绝未知、缺失或重复字段。`frame_id` 为 1—64 个 ASCII 字母、数字、下划线或连字符；`captured_at` 使用带时区 RFC3339/RFC3339Nano，返回 UTC；类型为 preview/snapshot/event。
- 文件按实际 JPEG 字节校验，包括尺寸和完整解码；文件名、扩展名和 MIME 声明不决定是否有效。保存原始字节，JPEG ≤2 MiB，总正文 ≤2 MiB+64 KiB，单边 ≤4096、总像素 ≤16,777,216。参数/JPEG 错误返回 400/40001 和字段；资源超限返回 413/40001。
- 新图片返回 201 和稳定 `snapshot_id/device_id/type/captured_at`。同设备、同 `frame_id` 且采集时间、类型、原始字节相同的重试返回 200 和原 ID；冲突返回 409/40903。并发重试只有一份记录/文件，且不重复通知；跨设备相同 frame_id 独立。
- 最新预览接口返回 `snapshot_id/type/captured_at/url` 的 JSON。按 captured_at 选最新，同时间按平台成功提交顺序；迟到旧图仍保存且可按 ID 读取，但不覆盖预览或推送。snapshot/event 不覆盖 preview。
- content 返回 200/image/jpeg 和原始字节。无预览、未知图片或图片文件缺失返回 404/40403；未知设备返回 404/40401。存储/数据库其他错误返回 500/50001，不暴露本地路径。
- 图片文件在 `storage.snapshot_dir`，SQLite 保存不可变身份、摘要、路径与时间元数据。文件持久成功后才提交元数据；提交前失败/取消补偿本次文件，提交后 HTTP 断连保留结果供幂等重试。重启保留最新预览和重试身份，未引用文件不作为有效图片。
- 文件和元数据可读后，新增 snapshot/event 以及成为最新的 preview 发布一次 `frame_update`，data 包含 device_id/snapshot_id/type/captured_at/url，不含 JPEG；timestamp 为平台时间。慢客户端沿用 Hub 的有界队列与关闭策略。
- 已登记的离线设备及运行中 MQTT 断线均可上传，图片不更新 online、last_seen_at 或配置版本。平台初次启动仍要求 Broker 连接成功。
- 本轮图片持久保留，没有 TTL 或删除接口；`camera.snapshot` 命令、告警引用完整闭环、鉴权和容量清理留后续。真实 Broker、Docker、摄像头和树莓派尚未执行图片联调。

## 状态约定

- 最新数据按设备 timestamp、同时间下的 seq 排序，旧消息不回退状态；更新时间戳允许设备 seq 归零。设备时钟回拨时，回拨期间的消息不会覆盖已有最新状态。
- `online` 来自 status/遗嘱；遥测不会将离线设备自动标为在线。平台启动或 Broker 断开后，在线状态等待重新确认。
- `samples` 保留采样元数据，`freshness` 区分 fresh/stale/fault/missing。车速、距离、陀螺仪和车道 TTL 为 1 秒，温度为 5 秒；缺失和故障值为 null。
- HTTP 与 WS 使用相同状态视图；WS 在新遥测、状态变化和 TTL 到期时推送。慢客户端会关闭，不阻塞其他连接。
- 历史按每设备最多 1Hz 保存。历史 freshness 表示记录接收时的状态，不随查询时间变成 stale。
- 历史 start/end 接受 RFC3339 时间，包含端点；limit 默认 100，上限 1000。返回窗口内最新 N 条，按时间升序排列。
- 平台重启保留历史和设备配置；最新遥测等待新消息。status 的 config_version 是设备报告，不能代替配置 Ack。

## Broker 集成检查（后续验证）

实际 Broker 的集成检查会临时启动 Mosquitto 与后端子进程，并覆盖两个 WS 客户端、离线遗嘱、seq 重启、数据过期、平台重启和 Broker 重连：

```sh
MOSQUITTO_BIN="$(command -v mosquitto)" go test -tags integration ./internal/server -count=1 -v
```

该检查必须提供可运行的 Mosquitto；没有 Broker 不会将检查跳过后算作通过。
