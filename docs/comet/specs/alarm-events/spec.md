# 告警事件及有效图片关联完整目标规格

## 1. 上行与校验

订阅 car/+/events，QoS 1、retained=false；单条合法消息持久化并以 HTTP/WS 展示。顶层仅接受 schema_version、event_id、device_id、type、action、level、timestamp、data、snapshot_id、frame_id、snapshot_available。schema_version=1，身份与主题一致，已登记设备；未知设备不注册，离线设备允许补传。事件不修改在线/last_seen/遥测/命令事实，也不从 telemetry.risk 自动派生。

event_id 为 1—64 非空白字符，表示一条 MQTT 消息而非可更新告警实例。type 为 obstacle_warning/lane_departure/sharp_turn/high_temperature/sensor_failure；action 为 started/level_changed/recovered；level 为 warning/danger，recovered 仍用解除前等级。timestamp 为带时区 RFC3339，保存 UTC；data 为对象，可空，内部值原样保留，不重算风险/时距，不把缺值补 0。

snapshot_id 可省略，出现时为 1—64 非空白字符；frame_id 可省略，出现时为 1—64 ASCII 字母/数字/下划线/连字符，等于同设备上传的 frame_id。二者互斥，可以均无；显式 null、类型错误、未知字段、尾随 JSON 或非法字段拒绝整条。snapshot_available 可省略，出现时只允许 bool；仅作为兼容输入，不保存、不信任为图片事实、不参与幂等。错误记录原因，不插入/广播或退出进程。

## 2. 持久事实及幂等

event_id 全局唯一，每个不同 ID 保存一行；started/level_changed/recovered 不是同一行更新，不增加 incident_id/起止时间状态机。较旧的新消息仍插入。保存设备/type/action/level/UTC timestamp/data JSON 以及原始 snapshot_id/frame_id。

重复比较上述原始字段及 data JSON 语义，不比较派生图片状态或设备 snapshot_available。相同字段为重放，保持行且不广播；不同字段/跨设备复用 ID 为冲突，保留首条并记录原因。关联成功不回写原始引用，重启后重放仍识别重复。数据库唯一约束裁决并发首条，只有真实插入成功才广播事件；写入失败不伪装保存。Paho 保留既有回调返回后确认与发送方重放补偿，不建立 outbox。

未知或错误图片引用仍是合法告警事实，不丢弃告警，也不要求图片在告警到达前已上传。

## 3. 平台图片视图

HTTP items、WS event 和 event_image_update 使用一致图片字段：

| 字段 | 行为 |
| --- | --- |
| frame_id | 原输入 frame_id，未给为 null |
| snapshot_id | 通过校验的有效图片 ID；不可用时 null，不把原始错误引用冒充有效 ID |
| snapshot_available | 平台计算 bool |
| snapshot_url | available 时稳定 /api/v1/snapshots/{id}/content，否则 null |
| snapshot_state | none/pending/available/invalid/missing/unavailable |

无引用为 none。按原 snapshot_id 查记录，或按同设备 frame_id 查不可变上传身份；尚无记录为 pending。记录须已提交、同设备且类型 snapshot/event，跨设备或 preview 为 invalid。按 storage 根目录安全打开预期普通文件，缺文件为 missing；可打开为 available 并提供有效 ID/URL。GET 遇到 DB/其他存储故障返回 500/50001；新告警已经保存后解析暂时失败，记录错误并仍广播告警，图片字段为 unavailable/false/null，不把故障当不存在或可读。

图片路径/字节/内容读取继续按 frame-storage，JPEG 不进 WS；不按时间邻近、告警类型或文件名猜测。已提交图片独立持久，不因告警解除/回执超时删除。之后外部删图不改告警原引用，不要求主动文件监视；GET 重新反映 missing。

## 4. HTTP 查询

GET /api/v1/devices/{device_id}/events，参数 type/level/start/end/limit，其他忽略。未知设备 404/40401；非法type/level、非RFC3339、start>end、limit非整数1—1000为400/40001并标字段；默认limit100，时间筛选含两端，多个筛选同时生效。先按timestamp DESC/event_id DESC选最多limit条并按该序返回，无记录items=[]，其他内部故障500/50001。

条目含 event_id/type/action/level/UTC timestamp/data 和上述图片字段；不含device_id或DB自增ID，不直接序列化模型。每次从持久引用、图片元数据和文件重新解析，关联不依赖进程内缓存；保留事件正文和过滤排序。

## 5. WS 创建及图片后到更新

沿用 /api/v1/ws 信封 {type,timestamp,data}，timestamp为平台时间。真正新插入后发 type=event，data为 event_id/device_id/event_type/action/level/设备timestamp/data及图片视图，event_type与HTTP type命名保留。所有连接收到已登记设备事件，用device_id区分；不增加订阅过滤，不在连接/重启时重放历史，GET为最终入口。

图片先到：首次event已available。告警先到：先event(pending)，新图片安全提交后以同设备原引用定位已保存告警，发 type=event_image_update；data只含event_id/device_id及图片字段，更新已有告警，不再插入或发送第二个event。错误类型的新图亦可更新为invalid，不误报可读。

告警插入/解析/推送和图片关联通知按一致次序组织，避免图片更新先到而旧pending创建帧后到导致前端回退。图片或事件幂等重试不重复对应通知；冲突/拒绝/未提交图片不关联。文件I/O不持Monitor锁或长DB事务，按有界批次定位相关记录，不加载全部历史。提交后通知失败不回滚图片/告警，记录错误并以GET恢复；不承诺持久WS重放。沿用Hub有界队列、慢连接关闭及单写者，慢客户端不阻塞其他连接/遥测。

## 6. 生命周期与软件设备

初次和Broker重连恢复telemetry/status/command-acks/events/config-acks全部五组订阅；初次任一失败不能报告就绪。SQLite事件/原引用保留，重启后GET立即从已有图片解析，不重放旧事件。关闭停止接收并保留已经提交事实。

模拟器仍在启动时发布合法演示告警并重发同一内容，QoS1非retained，默认无图。可选 -event-image 先发带稳定frame_id的告警再上传软件event JPEG，使用相同后端-api地址；有限重试复用身份/采集时间/原字节，上传失败不修改原告警或伪造可读。工作响应关闭取消并有超时，不在MQTT回调等HTTP；遥测/配置/拍照继续按已有契约。软件图不等于真实摄像头/规则引擎完成。

## 7. 交付与延期验证

本轮按brief A1—A7交付代码、gofmt、go build ./...及新的独立只读核对；不新增/运行test/race/vet、软件链路、Broker/Docker、双平台/硬件测试，不修改旧断言隐藏问题。代码完成不是运行验收通过。

V1统一更新新增有效图片DTO及原引用规则相关旧用例，验证双顺序/并发/重放/错误类型/跨设备/缺文件/读取故障/取消/重启/双WS和全量test/vet/race；V2实际Broker/Docker、Python/Vue/摄像头/树莓派。鉴权/ACL/CORS、图片清理/TTL/视频、设备注册与持久outbox留后续。WS不提供config_update，协议列表同步此事实。
