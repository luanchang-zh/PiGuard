# 控制命令与设备回执完整目标规格

本 capability 定义 PiGuard 的完整命令行为，包括既有四类动作及 camera.snapshot。新增拍照代码交付的 A1—A7 以当前 brief 为准，运行验收按用户要求后置。

## 1. 范围与协议

提供 POST /api/v1/devices/{device_id}/commands、GET /api/v1/commands/{command_id}、MQTT commands/command-acks 和 WS command_update。支持 buzzer.test、indicator.test、scenario.start/stop、camera.snapshot；独立配置同步沿用 config-sync。

下发主题 car/{device_id}/commands，QoS 1、retained=false，payload 含 schema_version=1、平台唯一不可复用 command_id、device_id、type、UTC issued_at/expires_at、params。expires_at=issued_at+10 秒。

## 2. 参数与规范化

| 类型 | params |
| --- | --- |
| buzzer.test | 必填整数 duration_ms，100—5000 |
| indicator.test | 必填 color=green/yellow/red；必填整数 duration_ms，100—5000 |
| scenario.start | 必填 scenario=normal_drive/obstacle_approach/lane_departure/sharp_turn/high_temperature/sensor_failure；必填 speed=0.5/1/2 |
| scenario.stop | 空对象 |
| camera.snapshot | quality 可省略，缺省 80；显式值为 1—100 整数 |

HTTP 正文为单个 JSON 对象且只有 type/params，params 必须为单个对象且只有该类型的规定字段；缺失/null/错误类型/非法值/额外字段/尾随 JSON 拒绝。camera quality 显式 null 不等于省略。创建时补齐缺省 quality，数据库、MQTT 和 GET/WS 对此参数一致；设备收到省略 quality 也按 80 解释。未知类型不生成占位命令。

平台只传递场景参数，不实现 Python 场景播放器或规则引擎。

## 3. HTTP 受理与异步下发

创建先校验参数、已登记设备、在线事实、发布连接及有界工作队列。非法参数为 400/40001；未知设备 404/40401；已知离线 503/50301；MQTT 不可用 503/50302。前置拒绝不落库、不下发。

平台生成 ID/时间，先 SQLite 保存 pending，再交给受控异步发布。DB 故障为 500/50001，不能先发后写。成功为 HTTP 202/code 0，data={command_id,status:"pending"}；202 仅表示受理，GET 可以已读到 sent 或终态。每个有效 POST 创建新命令，无 HTTP 幂等键，不把重复 POST 等同一次动作。

明确未发布成功时尝试将仍 pending 的记录改 failed，error.code=MQTT_UNAVAILABLE；发布超时/取消而交付不确定则保留 pending，等待 Ack 或到期。不建立持久 outbox，不自动重发动作；QoS 1 重投递以 command_id 防重复执行。

## 4. 持久状态机与截止

保存 command_id、设备、类型、参数、状态、issued_at/expires_at、平台 ack_at、设备 executed_at、结构化 result/error。

| 当前状态 | 触发 | 新状态 |
| --- | --- | --- |
| pending | Broker 确认 publish，记录仍 pending | sent |
| pending/sent | 截止前接受合法 success Ack | success |
| pending/sent | 截止前接受合法 failed Ack | failed |
| pending | 明确发送失败，尚无 Ack 终态 | failed |
| pending/sent | 平台达到截止且无有效 Ack | timeout |
| success/failed/timeout | 后续 callback/Ack | 保持终态 |

条件更新只容许首个有效终态；极速 Ack 可结束 pending，publish 完成不能覆盖它。Ack 和截止处理采用一致临界次序，平台进入有效 Ack 处理的 received_at < expires_at 才准时，不使用设备时钟判断。拍照图片读取完成并进入有效处理后仍需检查截止，读取不得让过期结果成功。

数据库扫描 pending/sent，正常运行截止后最多 1 秒转 timeout；GET 先确保过期事实落库。timeout 表示未及时确认，设备可能已执行/上传。已上传图片不因命令失败、超时或迟到回执而回滚。状态写入成功且真实变化才广播一次；DB 错误不广播。

## 5. Ack 验证、图片引用与幂等

订阅 car/+/command-acks，QoS 1，设备 Ack retained=false，初次及重连恢复订阅。Ack 是单个 JSON 对象；schema_version=1；命令已存在；主题、payload、命令设备一致；command_id 非空且最长 64；status=success/failed；executed_at 有效且 UTC；success 只含对象 result，failed 只含非空 code/message 的 error。

拍照成功 result 严格为 {"snapshot_id":"snap-..."}，非空 ID、无额外字段。平台通过已提交图片元数据和受限路径读取确认：同设备、type=snapshot、文件可打开；未知 ID、跨设备、preview/event、缺文件或非法 result 均不改状态、不广播，等待合法 Ack 或 timeout。DB/存储故障不能误判为有效图片。图片校验不持有 Monitor 锁或长 DB 事务，也不覆盖命令并发终态。

图片本身独立上传，不自动创建或结束命令。平台不限制 captured_at 时钟窗口、不要求 frame_id 与 command_id 相等、不要求一图仅能引用一次。软件设备每条新拍照命令仍生成并上传一张新图。

普通动作 success result 保留既有对象约定。平台另存原设备 result 用于重复判断；DTO 的拍照稳定 URL 是平台派生值，不使同一原始 Ack 被误判冲突。

非法版本/JSON/身份、未知命令和跨设备回执丢弃并记录原因。设备非空 error code/message 保留，支持 COMMAND_EXPIRED、COMMAND_DUPLICATED、UNSUPPORTED_COMMAND、INVALID_PARAMS、DEVICE_BUSY、DEVICE_NOT_SUPPORTED、SENSOR_UNAVAILABLE、ACTUATOR_UNAVAILABLE、INTERNAL_ERROR 等；拍照上传失败使用 SNAPSHOT_UPLOAD_FAILED。

相同终态的语义重复 Ack 不保存/广播；冲突保留首个终态；timeout 后只记录迟到诊断，不新增可查询迟到结果或改终态。拍照终态重复不重新要求图片仍存在，不因之后文件缺失改写历史事实。

## 6. GET 与 WebSocket

GET 返回 200/code 0 和 command_id/device_id/type/params/status/issued_at/expires_at/ack_at/executed_at/result/error。无 Ack 的时间为 null，无 result/error 为 null；未知命令 404/40402。不直接序列化模型。

拍照 success 的 result={snapshot_id,url}，url=/api/v1/snapshots/{snapshot_id}/content，为相同不可变图片。内容读取继续按 frame-storage：200/image/jpeg 原字节，未知/缺文件 404/40403，其他存储故障 500/50001，不暴露目录。之后缺文件仍保留历史成功结果，由 content API 如实报告。图片上传仍产生自己的 frame_update，command_update 只随命令状态转换。

WS /api/v1/ws 发送 {type:"command_update",timestamp:<平台UTC>,data:<GET DTO>}。不重放全部历史，GET 为最终查询入口；沿用 Hub 有界队列、慢客户端驱逐、超时和心跳，不阻塞其他连接或 MQTT/Monitor。

## 7. 启动、重连、关闭

SQLite 终态保留；启动将已过期 pending/sent 转 timeout，未到期继续等 Ack，不重发。Broker 断开沿用在线失效，重连恢复订阅及新命令受理；发布等待有限且受 context 控制。关闭停止受理、取消工作/扫描/连接，未完成记录留数据库，下一启动按上述规则处理。

## 8. 软件模拟器拍照

保留 status/telemetry、commands/config 订阅及 success/failed/none 模式。协议、设备和 expires_at 校验后执行；过期命令不执行，发 COMMAND_EXPIRED。首次有效命令缓存 Ack（包括被抑制的回执），同 ID 重投递重发缓存结果，不再次执行。进程重启的执行缓存不是持久幂等保证，平台不会自动重发动作。

新增后端 HTTP API 地址参数（默认 http://127.0.0.1:8080）。success 模式收到 camera.snapshot 时按 quality 生成软件 JPEG，以 snapshot 类型提交既有 multipart API；每条命令稳定 frame_id/采集时间/原字节，有限重试不产生重复图片。只在 201/200、code=0、归属/类型及非空服务端 ID 有效时发 success；上传故障发 SNAPSHOT_UPLOAD_FAILED，不能编造 ID。failed/none 模式保留用于后续失败/超时验证。

HTTP 上传在有界工作过程中执行，MQTT callback 只排队，不等待拍照/上传；遥测及配置回调可继续。每次拍照响应进程取消并受 expires_at 截止约束；关闭取消并等待工作退出。Ack 延迟参数仍可用于后续迟到验证，不能延长平台截止。模拟图片不证明真实 CameraSource/摄像头可用。

## 9. 代码交付及延期验证

本轮格式/编译及新只读 Verifier 对 A1—A7 的代码核对；不新增/运行 test/race/vet、真实 Broker/Docker、双平台或硬件运行测试，不改已有断言。代码交付结果不表示软件或真实链路运行通过。

V1 集中补测及运行参数边界、合法/非法图片引用、状态竞争、重复/冲突/迟到/跨设备回执、上传失败/幂等重试/取消/并发、恢复、HTTP/双 WS 以及全量 test/vet/race。V2 验证真实 Broker/Docker 重连/retained/遗嘱及 Python/Vue/摄像头/树莓派。README 和协议保存例子及实际证据边界。

告警图片关联、部署鉴权/CORS/MQTT ACL、存储清理/TTL、命令历史、视频及硬件实现留后续 change。
