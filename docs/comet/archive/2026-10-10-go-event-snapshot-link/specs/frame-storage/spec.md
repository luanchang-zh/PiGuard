# 图片上传与读取完整目标规格

保留 frame-storage 的全部存储、校验、身份及读取契约；图片提交可使已有告警的显式引用更新为有效图片，告警接口由 alarm-events 定义。本轮功能代码交付，实际运行验证在 V1/V2 后置。

## R1：范围与身份

提供 POST /api/v1/devices/{device_id}/frames、GET /api/v1/devices/{device_id}/frame、GET /api/v1/snapshots/{snapshot_id}/content 和 WS frame_update。未知设备404/40401；已登记离线设备及运行中MQTT断线仍可上传，不更新online/last_seen/配置，上传本身不结束命令。初次Broker连接成功才启动的约定保留。

类型 preview/snapshot/event 独立上传。snapshot上传不自动生成拍照命令结果，event上传不创建告警；新图片可以使已有显式告警引用被解析/推送更新。平台生成稳定不可变snapshot_id，JPEG位于storage.snapshot_dir，SQLite只存身份/时间/类型/摘要/长度/服务端路径。

## R2：multipart/JPEG

严格接受一个file/frame_id/captured_at/type，未知、缺失、重复、多文件或错误multipart拒绝。frame_id为1—64 ASCII字母/数字/下划线/连字符，不构造路径；captured_at为带时区RFC3339/RFC3339Nano，归一UTC，不限制设备时钟窗口。

按非空JPEG实际字节、尺寸和完整解码校验，损坏/截断/非JPEG拒绝；文件名/扩展名/MIME不决定有效性，保留原字节。JPEG≤2MiB，总正文≤2MiB+64KiB，单边≤4096，总像素≤16,777,216；有界读取、先尺寸再解码，响应context取消。参数/JPEG错误400/40001并标字段，超限413/40001；拒绝无记录/文件/通知。

## R3：幂等与不可变发布

安全提交后新图201/code0，data含snapshot_id/device_id/type/UTC captured_at。同设备frame_id为幂等身份：时间/类型/原字节一致重试200及原ID，不新增或重复通知；不同内容/元数据409/40903，保留首图。跨设备身份独立，并发同键只有一份记录/文件。最终发布不可覆盖旧文件，旧ID字节不可变，未知/缺文件不因重试编造新结果。

## R4：最新预览

GET frame只查询preview，snapshot/event不覆盖。按captured_at最新，同时间按平台提交顺序；200返回snapshot_id/type/UTC captured_at/稳定url。旧图可独立读取，不推进preview或通知，并发通知不回退。无preview404/40403，未知设备404/40401，不暴露DB ID或路径。

## R5：内容及有效引用

GET content按ID查元数据和安全普通文件，200/image/jpeg原字节。未知ID/缺文件404/40403，DB/其他存储故障500/50001，不泄露目录或接受任意路径/客户端文件名。路径须为平台生成、受配置根目录约束的预期普通文件。

拍照成功引用要求同设备snapshot类型；告警显式引用要求同设备snapshot/event类型。校验复用持久图片记录及上述安全打开路径，不能将preview、跨设备或缺文件当有效告警图。告警无引用/待上传/错误/缺文件及故障状态由alarm-events规定；不更改原始引用参与的告警幂等。

## R6：平台持久化、补偿与重启

Windows普通文件同步后不可覆盖发布，避免对不支持目录句柄调用Sync；Linux仍同步文件及发布目录，不笼统吞掉权限/空间/I/O错误。写入、适用持久化、发布及可读完成后才提交SQLite。提交前文件/DB/取消失败回收本请求临时/未提交文件，不删除已有图或用户文件、不推进预览/通知；提交后断连保留幂等结果。

重启保留身份/已提交字节/最新preview。临时/未引用文件不作为有效图，不自动清理，不承诺超出OS/文件系统支持的断电恢复；外部缺文件如实返回404。I/O不持Monitor锁或长SQLite事务，短DB接受与预览推送次序保留。

## R7：frame_update 及告警引用通知

可读并提交后，新增snapshot/event或成为最新的preview发一次frame_update；timestamp为平台UTC，data含device_id/snapshot_id/type/captured_at/url，无JPEG。重复/冲突/拒绝/失败或迟到旧preview不重复frame_update。沿用有界队列/慢连接关闭，双客户端一致。

新增图提交后通知告警引用解析，按同设备原snapshot_id/frame_id定位已存在事件并以event_image_update更新，定义见alarm-events；不创建新告警、改写原引用或重复发event。图片幂等重试不重复关联通知。告警处理I/O在短图片提交/有序frame_update步骤之后，不持Monitor锁或长DB事务；通知失败记录诊断，已提交图片不回滚，GET告警可重新解析。

保留telemetry/event/device_status/command_update，新增告警图片状态更新，不新增config_update。上传snapshot本身仍不改命令，合法设备Ack才形成拍照成功事实。

## R8：代码交付及后验

本轮按关联change的brief A1—A7进行gofmt、go build ./...和新的独立只读代码核对；不新增或运行test/race/vet、双平台/Broker/Docker/硬件测试，不放宽原断言。编译不代表运行通过。V1统一回归上传/读图/幂等/排序/补偿/取消/重启及告警双顺序、有效引用和双WS，再运行test/vet/race；V2实际Broker/Docker/Python/Vue/摄像头/硬件。缺证据与环境如实记录。

## R9：边界

JPEG持久保留，不提供PNG/视频/缩略图/重压缩/图片分页/删除/TTL/容量清理/对象存储。鉴权/ACL/CORS、自动注册、Python/Vue及真实硬件留后续；不从代码/软件检查推定真实环境通过。
