# 图片上传与读取完整目标规格

frame-storage capability 的完整目标规格：保留现有契约，修复 Windows/Linux 存储兼容性，为后续 camera.snapshot 功能打通存储路径。用户已确认功能优先、测试延后及 brief A1—A6 代码交付标准，实际运行测试集中在后续 V1/V2；代码验收不代表运行验收通过。

## R1：范围与身份

提供 POST `/api/v1/devices/{device_id}/frames`、GET `/api/v1/devices/{device_id}/frame`、GET `/api/v1/snapshots/{snapshot_id}/content` 和 WS frame_update。未知设备为 404/40401；已登记离线设备及运行中 MQTT 断线仍可上传，不更新 online/last_seen_at/配置或命令事实。保留初次连接 Broker 成功才启动的约定。

类型为 preview/snapshot/event，独立上传；上传 snapshot 不生成拍照命令结果，上传 event 不生成告警。服务端生成稳定不可变 snapshot_id；JPEG 放入 storage.snapshot_dir，SQLite 保存身份、时间、类型、摘要、长度及服务端路径，不保存 JPEG 大对象。

## R2：multipart/JPEG

严格接受 multipart 的一个 file、frame_id、captured_at、type，拒绝缺失、重复、多个文件、未知字段和错误 multipart。frame_id 为 1—64 个 ASCII 字母/数字/下划线/连字符，不用其或文件名构造路径；captured_at 为带时区 RFC3339/RFC3339Nano，归一 UTC，不做时钟偏差或未来窗口处理。

file 非空，按实际 JPEG 字节、尺寸及完整解码判断有效性，拒绝非 JPEG/损坏/截断内容；保存原字节，文件名/扩展名/MIME 不决定有效性。JPEG≤2 MiB，总正文≤2 MiB+64 KiB，单边≤4096，总像素≤16,777,216；有界读取，先检查尺寸再解码。

参数/JPEG 错误为 400/40001 并提供字段路径，资源超限为 413/40001。拒绝不产生记录、文件或通知；读取、复制和写入响应取消。

## R3：受理、重试与并发

文件安全保存并提交可查询元数据后，新图片返回 201/code 0，data 含 snapshot_id/device_id/type/UTC captured_at。以 (device_id, frame_id) 为幂等身份：相同归一时间、类型和原字节重试为 200/code 0 及原 ID，不新增文件/记录/通知；同键不同元数据/字节为 409/40903，保留首个结果。跨设备身份独立，并发同键只有一份成功持久结果。旧 ID 内容不可变，最终文件发布不能覆盖既有文件。

## R4：最新预览

GET frame 只查 preview，snapshot/event 不覆盖；200/code 0 的 data 含 snapshot_id/type/UTC captured_at/稳定 content URL。按采集时间选择最新，同时间按平台成功提交顺序。旧图可独立读取，不推进 preview 或通知；并发接受/通知不使预览回退。URL/ID 来自同一记录，不暴露路径或 DB ID。无 preview 为 404/40403，未知设备为 404/40401。

## R5：内容

GET content 按 snapshot_id 查元数据及文件，返回 200/image/jpeg 和原字节；稳定 URL 不指向可变当前文件，JSON/WS 不放 JPEG。未知 ID/文件缺失为 404/40403，DB/其他存储错误为 500/50001。路径被约束在配置根目录内，目标须为预期普通文件；不能用任意路径、frame_id 或客户端文件名读取，不向客户端泄露本地路径。

## R6：平台持久化、失败与重启

Windows 开发环境和 Linux 部署环境均支持完整图片流程；有效 JPEG 不因平台不支持某目录同步操作而正常失败。平台差异使用受支持持久化步骤处理，不能笼统忽略权限、空间、I/O 或普通文件同步故障。Linux 保留既有可用文件/目录同步保障，Windows 明确其保障范围，不因兼容修复统一削弱 Linux。

文件写入、适用持久化、不可覆盖发布及可读性完成后才提交 SQLite 元数据。真正的文件/发布/DB 失败和提交前取消不产生可查询半成品、预览推进或通知，回收本请求临时/未提交文件，不删除已有图片/用户文件。提交后断连保留结果供同身份重试。

关闭后重开 SQLite/目录保留成功提交身份、文件及 preview；硬中断可能留临时/未引用文件，不可查询为有效图片。此轮不后台清理、不承诺超出平台/文件系统能力的断电恢复；已持久行对应文件缺失按 R5 返回。图片 I/O 不持有 Monitor 锁或长 SQLite 事务；身份/排序遵守 R3/R4，不阻塞遥测。

## R7：frame_update

文件可读并提交元数据后发 frame_update，timestamp 为平台时间，data 含 device_id/snapshot_id/type/captured_at/url，无 JPEG。新 snapshot/event 通知，preview 仅在成为最新时通知；重复、拒绝、冲突、失败不重复通知。双 WS 得到一致 DTO，慢客户端沿用有界队列/关闭策略。保留 telemetry/event/device_status/command_update，不新增 config_update。

## R8：功能代码交付与延期验证

当前先交付平台适配代码、保留的业务契约与同步文档。修改 Go 文件遵守格式要求，`go build ./...` 可编译；必要格式/编译检查及新的只读 Verifier 代码核对可以执行，不新增或运行测试，不先配置 race/Docker/Broker 环境。保留已有图片与原业务测试，不删除或放宽断言隐藏缺陷。

本轮 Verifier 对照 brief A1—A6 核对代码实现、平台分支、不可覆盖发布、错误/补偿、持久读取与文档；结论只覆盖代码交付，编译不等同实际运行通过。双平台实际运行和测试不是本轮代码交付或后续拍照开发的前置。

后续 V1 再以实际 SQLite、文件系统、HTTP/双 WS 补充并运行上传/读取、资源边界、重试/冲突、并发/乱序、存储/DB 失败、取消、重启及全量 test/vet/race。V2 再进行真实 Broker/Docker、边缘端/前端与硬件联调。缺环境、程序启动失败和未执行检查如实记录，不算通过。README 区分历史证据、本轮代码交付和延期运行验收，记录待验事项。

## R9：后续边界

JPEG 持久保留。本轮不增加拍照命令、告警图片有效引用、PNG/视频/缩略图、图片分页/删除/TTL/清理、对象存储、鉴权、设备自动注册、Vue 或 Python。真实 Broker/Docker/摄像头/树莓派联调分别报告实际执行情况，不从软件检查推定通过。
