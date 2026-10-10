# Outcome

完成告警与有效同设备图片的关联，支持告警/图片先后到达，让 HTTP/WS 明确报告图片是否可读取及稳定内容 URL。保留告警原始消息、幂等及筛选行为；先交付代码，测试与实际环境联调后置。

# Scope

- 告警输入继续支持可选 snapshot_id；新增可选 frame_id，规则沿用上传的 1—64 ASCII 字母/数字/下划线/连字符。两个引用互斥，也可以都省略；显式 null/非法值拒绝。snapshot_available 继续仅接收合法 bool，不信任为平台事实，也不保存或参与幂等。
- 原始 snapshot_id/frame_id 持久保存并参与重复消息比较，不在关联成功后改写，保持同 event_id 首条事实。未知/错误图片引用不丢弃合法告警。
- HTTP 告警条目与 WS event 新增 frame_id、snapshot_available、snapshot_url、snapshot_state；snapshot_id 变为通过校验的有效 ID，不可用时为 null。只允许同设备、类型 snapshot/event、已提交且普通文件可打开的图片；URL 未通过校验时为 null。
- snapshot_state 为 none（无引用）、pending（尚未提交）、available（可读）、invalid（跨设备/类型错误）、missing（有记录但缺文件）、unavailable（暂时读取故障）。GET 遇到 DB/其他存储故障返回 500，不把故障当可读；新告警已保存后解析暂时失败仍推送告警并标 unavailable。
- 告警先到可用 frame_id 等待后续提交；新图片提交后对相关既有告警发 event_image_update，包含 event_id/device_id 及一致的图片字段，不再插入/广播一条新告警。图片先到则首次 event 已可读。GET 从持久引用/元数据/文件重新计算，恢复后可查，WS 不补推历史。
- 软件模拟器新增可选 -event-image：在原演示告警中先发布 frame_id，再上传软件 event JPEG，以展示乱序关联。默认仍保留原无图告警；拍照/遥测/配置行为继续保持。同步 README/网络规范，整理既存 config_update WS 列表歧义。

# Non-goals

- 不实现鉴权/ACL/CORS、真实摄像头/Python/Vue/硬件、图片删除/TTL/历史分页、图片重压缩/视频、持久 WS outbox 或后台扫描全部图片。
- 不通过图片时间窗口、告警类型、采集邻近时间或文件名猜测关联；不把上传本身变成告警创建或命令成功。
- 不信任设备 snapshot_available；不让关联改写原始告警幂等身份；外部删图后无需主动文件监控，GET 重新反映状态。
- 不新增或运行 test/race/vet、软件链路、Broker/Docker、双平台及硬件测试；保留已有断言，V1 统一按新增契约更新补测。

# Acceptance examples

- A1：告警协议接受互斥可选 snapshot_id/frame_id，严格校验类型/空值/长度和 frame_id ASCII 规则；保留原事件校验及 snapshot_available 不作为事实的行为。
- A2：SQLite 持久保存原引用，重复比较包含原 snapshot_id/frame_id，关联不回写这些列；保留全局 event_id 首条、并发唯一约束、冲突不新增/推送及筛选排序。
- A3：列表与首次 event 提供一致的有效 snapshot_id/frame_id/available/url/state；同设备 snapshot/event 且文件可打开才 available，未知/跨设备/preview/缺文件不提供有效 ID/URL，临时故障不误报可读，告警记录仍保留。
- A4：告警先到及图片先到均有明确路径；新图片提交后按同设备原引用定位告警并发 event_image_update，不重复创建告警；图/事件提交与推送次序不会将已发布可读状态回退为旧 pending，幂等图片重试不重复关联通知。
- A5：重启后 GET 用持久原引用/图片记录重新解析；不依赖内存关联表，不重放 WS 历史，外部缺文件反映 missing；保留图片不可变 ID、帧幂等、预览排序、命令状态及内容读取契约。
- A6：模拟器 -event-image 可先发带稳定 frame_id 的合法告警再上传 event JPEG，重复事件内容一致，上传失败不伪造可读；工作受取消/超时约束，MQTT 回调、遥测/配置继续可用，默认无图模式保持。
- A7：Go 文件 gofmt，go build ./... 可编译；完整 Spec、README/网络规范及 WS 类型一致，写明有效 ID 行为、图片状态/乱序更新、已有旧断言的 V1 更新及未运行验证，不将代码交付当运行通过。

# Constraints and invariants

- 使用现有 EventService、FrameService、SQLite 与 Hub，不新增告警实例状态机或跨设备关联。
- 图片安全提交之后才关联；不持 Monitor 锁或长 SQLite 事务等待文件 I/O，相关记录读取和工作规模有界。
- 告警内容与原引用不可变，图片可用性为平台派生视图，不参与消息幂等。GET 为最终查询入口，WS 连接/慢客户端策略保留。
- 存储修复和拍照闭环已归档，运行验证均延后；本 change 不改变已接受的 quality、命令终态和存储保障。

# Decisions

- 用户已确认功能优先路线及拍照代码交付；当前承接 F2，在进入 Build 前集中确认新增告警接口行为。
- 图片 ID 由平台生成；为可靠支持告警先到，使用设备已知的 frame_id 显式关联，不用时间猜测或改变已有服务端 ID。
- 新图片视图只提供平台验证后的 ID/URL；原引用持久保存供重复判断，未知引用仍保存合法告警。
- 采用独立 event_image_update 更新已有告警的图片状态；不复用 event 创建通知、不新增 config_update。模拟演示启用 -event-image 后运行，默认不增加上传。
- 告警关联紧耦合事件/帧持久化及推送，保持单 change，不建立 Supervisor；F3 鉴权交付仍单独推进。
- 创建时拍照变更已归档并提交，当前目录干净，沿用 go后端开发/current。
- 用户于2026-10-10明确选择“确认，按此完整方案实施”，确认互斥frame_id/snapshot_id、有效ID/URL及六种图片状态、event_image_update、可选软件演示、A1—A7代码交付和延期测试/非目标。

# Open questions

无。完整目标、范围、关键决定、验收及非目标已确认。

# Verification expectations

- 用户确认完整 Shape 后再实施；先读 brief 与完整 alarm-events/frame-storage Spec，保留现有测试，不新增或运行测试。
- Runtime 当前只运行 go build ./...，开发期格式/差异核对；新只读 Verifier 逐项审查 A1—A7，代码未实际满足就判 failed，不因延期运行测试本身阻断。
- V1 更新受新增 DTO/有效引用契约影响的旧断言，补测双顺序/并发/幂等/错误引用/文件故障/取消/重启及 HTTP/双 WS，集中执行 test/vet/race。V2 再验证实际 Broker/Docker、Python/Vue 和硬件。
