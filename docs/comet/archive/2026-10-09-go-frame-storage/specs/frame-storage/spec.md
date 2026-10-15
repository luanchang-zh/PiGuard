# 图片上传与读取完整目标规格

本 capability 为 go-frame-storage 的 P3a 完整目标规格。用户已确认幂等重试、采集时间排序与 2 MiB 上限；完整 Shape 尚待确认，确认后才进入 Build。

## R1：范围、身份与结果

实现 POST `/api/v1/devices/{device_id}/frames`、GET `/api/v1/devices/{device_id}/frame`、GET `/api/v1/snapshots/{snapshot_id}/content` 和 WS frame_update。

只接受已登记设备，未知设备为 404/40401。图片与 online、last_seen_at、命令和配置版本事实分开；设备离线或已经运行的后端暂时失去 Broker 连接时，不因 MQTT 状态拒绝合法图片上传。保留当前后端初次连接必须成功的启动约定。

图片类型为 preview（预览）、snapshot（拍照图片）、event（告警图片）。本轮允许三种类型独立上传；不会因为上传 snapshot 自动生成 camera.snapshot 命令结果，也不会因为上传 event 自动生成告警消息。

成功上传返回稳定、服务端生成、不可变的 snapshot_id。JPEG 文件存入 storage.snapshot_dir；SQLite 保存 device_id、frame_id、type、captured_at、创建/接收时间、内容摘要和服务器文件路径等必要元数据，不保存 JPEG 大对象。

## R2：multipart 与 JPEG 校验

POST Content-Type 必须为 multipart/form-data。正文严格包含一个 file、frame_id、captured_at、type；拒绝缺失、重复字段/多个文件、未知字段和错误 multipart。

frame_id 是非空 1—64 字符的设备侧标识，允许 ASCII 字母、数字、下划线和连字符，不使用该标识或客户端文件名构造文件路径。captured_at 是合法带时区 RFC3339/RFC3339Nano，归一 UTC；本轮不增加设备时钟偏移估算或未来时间窗口。type 只允许 preview/snapshot/event。

file 不能为空。依据实际字节、JPEG 头和完整解码结果确认其为有效 JPEG，拒绝非 JPEG、损坏与截断内容。保留已验证的原始字节，不重新压缩或做缩略图。客户端提供的文件名和 MIME 声明不能作为内容有效性的依据；实际有效 JPEG 不因文件扩展名或 MIME 声明不同而被拒绝。

用户已确认单 JPEG ≤2 MiB（2,097,152 字节），总 HTTP 正文≤2 MiB+64 KiB。先读取 JPEG 尺寸，限制每边≤4096且总像素≤16,777,216，再完整解码验证，避免小压缩文件触发无界内存读取。

参数/JPEG 错误返回 400/40001，error.field 使用 file/frame_id/captured_at/type 等字段路径；正文、文件字节或尺寸超限返回 413/40001。拒绝时不创建可见记录或通知。校验、复制和磁盘写入响应请求取消。

## R3：上传受理、重复与并发

新图片先验证并安全保存文件，再在 SQLite 提交可查询元数据；完成后返回 HTTP 201、code 0：

```json
{"code":0,"message":"ok","data":{"snapshot_id":"snap-001","device_id":"car-001","type":"preview","captured_at":"2026-10-09T12:00:00Z"}}
```

用户已确认幂等契约：以 (device_id, frame_id) 唯一识别。相同身份、归一后的 captured_at、type 和原始文件字节重复上传，返回 HTTP 200/code 0 和首次 snapshot_id，不新建文件/记录、不修改原内容/时间、不重复通知。同键不同字节或元数据返回 409/40903；跨设备相同 frame_id 独立处理。并发同键只有一份成功持久记录，其他按一致的重试/冲突规则处理。

图片内容不可变；后续新图片具有独立 URL，不能把旧 snapshot_id 对应文件覆盖为另一张图。

## R4：最新预览

GET frame 查询该设备的 preview，不让 snapshot/event 覆盖。返回 HTTP 200/code 0：

```json
{"code":0,"message":"ok","data":{"snapshot_id":"snap-001","type":"preview","captured_at":"2026-10-09T12:00:00Z","url":"/api/v1/snapshots/snap-001/content"}}
```

用户已确认按 captured_at 选择最新 preview，相同时间以平台成功提交顺序决定。迟到旧图可以保存并按其 ID 读取，但不推进当前预览、不发送 frame_update；并发接受与通知顺序保持当前预览单调。

查询使用一致的持久记录，不返回本地文件路径或数据库 ID。已登记但无 preview 返回 404/40403；未知设备 404/40401。

## R5：图片内容

GET content 按服务器 snapshot_id 查找元数据与已保存文件，返回 HTTP 200、Content-Type=image/jpeg 和原始字节。客户端用稳定 URL 读取实际图片，不将内容塞入 JSON 或 WS。

未知 snapshot_id 或对应文件缺失返回 404/40403；数据库或其他读取错误为 500/50001。任意路径、客户端文件名或 frame_id 都不能被用来访问本地文件。存储路径必须在配置根目录内；不向客户端暴露服务端路径。

本轮不增加图片变换、分页历史查询、删除 API、缓存失效策略或视频流。当前图片 ID 指向不可变内容，可以作为后续命令/告警引用的基础。

## R6：持久化、失败与重启

文件系统和 SQLite 提交分别处理；必须保证成功对外可见时文件与元数据都可以读取。普通文件写入/最终落盘失败、DB 提交失败或提交前请求取消返回对应错误，不推进当前 preview、不通知、不留本请求的可见半成品；回收本请求创建的临时/未提交文件，不删除其他已提交图片或用户文件。已成功提交后连接断开不撤销图片，客户端用同一身份重试找回原结果。

平台重启后保留所有已成功返回的文件、元数据、幂等身份和最新 preview 选择。硬中断可能留临时/未引用文件，但它们不能被查询成有效图片；本轮不对用户目录或历史图片做后台清理。已持久行对应文件丢失按 R5 报告，不凭元数据宣称内容可读。

JPEG 读取/校验/文件 I/O 不持有 Monitor 锁或长 SQLite 事务。并发同身份遵守幂等/冲突契约；latest 选择与通知按已确认采集时间/同时间提交顺序，图片上传不阻塞既有遥测处理。

## R7：frame_update

可读内容与元数据提交完成后，通过现有实时 Hub 发通知：

```json
{"type":"frame_update","timestamp":"2026-10-09T12:00:01Z","data":{"device_id":"car-001","snapshot_id":"snap-001","type":"preview","captured_at":"2026-10-09T12:00:00Z","url":"/api/v1/snapshots/snap-001/content"}}
```

timestamp 为平台接受时间；captured_at 为设备采集时间。新 snapshot/event 图片均通知；preview 只有按采集时间/同时间提交顺序成为当前预览时通知。幂等重试不重复通知，冲突/拒绝/失败不通知。双客户端收到同一份已提交 DTO；慢客户端沿用有界队列与关闭策略，不阻塞上传。

保留 telemetry/event/device_status/command_update 既有语义。只增加 frame_update，不增加 config_update，不传图片字节。

## R8：交付与后续边界

Build 交付实现、协议/仓储/服务/HTTP/文件/双 WS 软件组件测试和文档。生成小 JPEG 样本即可验证上传、读取与失败，无需摄像头。Runtime 执行全量 Go test/race/vet；正式 Verifier 按 brief A1—A9 逐项验收。

本轮 JPEG 持久保留，不做 TTL、容量清理、PNG、视频、对象存储、鉴权、自动设备注册、Vue、Python 或 camera.snapshot。P3b 再连接拍照命令、软件设备上传与有效图片引用；P4 再规划运行与清理。真实 Broker、Docker 和硬件明确未执行。

本 Spec 不新增重复 Scenario；brief A1—A9 是验收索引。Q1—Q3 已全部同步，准备完整 Shape 确认；确认前不进入 Build。
