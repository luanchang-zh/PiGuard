# Outcome

补齐 Go 后端 camera.snapshot 闭环：HTTP 受理拍照命令，经 MQTT 下发，软件设备生成并上传 JPEG，平台只接受同设备、snapshot 类型且文件可读的成功回执，GET/WS 提供稳定图片 URL。先完成代码与文档，测试及真实环境联调集中后置。

# Scope

- 扩展现有 commands API、协议解析及命令状态机，保留其他四类命令的行为。
- camera.snapshot 的 params 必须为对象；quality 可省略，默认 80，显式值必须是 1—100 的整数，null、额外字段和非法值拒绝。平台保存并下发补齐后的 quality。
- 成功 Ack 的 result 为仅含非空 snapshot_id 的对象；引用必须已提交、属于命令设备、类型为 snapshot 且文件可打开。非法引用不改状态、不广播，命令继续等待合法回执或超时。
- 保留 HTTP 202、10 秒截止、首个有效终态、重复/冲突/迟到回执处理、重启不自动重发；GET/command_update 的成功 result 提供 snapshot_id 与稳定 url。
- 软件模拟器新增后端 API 地址参数，在成功模式生成有效 JPEG，按命令身份稳定上传，得到服务端 snapshot_id 后发成功 Ack；上传失败发失败 Ack，缺省失败/无 Ack 模式保留。耗时上传放在受控工作过程，MQTT 回调不等待 HTTP。
- 同步 README 和网络 API 规范，说明启动、调用、结果读取、失败及延期验证。

# Non-goals

- 不接真实摄像头、树莓派、Python 感知、Vue 或视频流；软件 JPEG 不表示真实相机已验收。
- 不在本 change 实现告警图片关联、HTTP/WS 鉴权、MQTT ACL、CORS、图片删除/TTL、命令历史或持久 outbox；这些沿用已确认的后续路线。
- 不新增或运行 test/race/vet、Broker/Docker、硬件和双平台运行测试；不放宽已有断言。
- 不以设备 captured_at 时钟判定 Ack 是否准时，不加入截图时钟窗口；不要求一张同设备有效 snapshot 只能被一条命令引用。

# Acceptance examples

- A1：HTTP 与 MQTT 解析支持 camera.snapshot，quality 省略为 80，显式整数 1—100 接受；null、非整数、越界、未知字段和非对象拒绝，保存及下发参数一致；其他命令的严格校验保留。
- A2：拍照命令复用受理、持久化、QoS 1 非 retained 下发及 10 秒截止；未知/离线设备、不可用发布器和参数错误仍在发布前拒绝，重启不自动重发。
- A3：拍照成功 Ack 只有同设备、已提交的 snapshot 类型图片且文件可读才结束命令；未知、跨设备、错误类型、缺文件或非法 result 不写成功、不广播，不把数据库/读取错误当有效图片。
- A4：GET 与 command_update 的成功结果含 snapshot_id 和同一稳定 content URL；保留原设备回执用于重复判断；首个终态、重复/冲突/迟到回执、失败和超时规则不变，已上传图片不因超时回滚。
- A5：模拟器成功模式按 quality 生成 JPEG，通过既有 multipart API 上传 snapshot 后发含服务端 ID 的 Ack；命令重投递使用缓存结果不再次拍照或上传，过期命令不执行，上传故障给失败回执，none/failed 模式保留。
- A6：模拟器上传有截止及取消约束，HTTP I/O 不在 MQTT 回调或 Monitor 锁中等待；仍能继续遥测/配置处理，关停取消并收回拍照工作过程；重试使用同一 frame_id、时间和字节，不无限重试。
- A7：变更 Go 文件经过 gofmt，go build ./... 可编译；README/协议/完整 Spec 一致并明确代码交付与未执行运行验收，保留延期验证清单及既有测试。

以上仅为代码交付验收，编译和独立代码核对不表示运行链路通过。

# Constraints and invariants

- 复用现有 CommandManager、SQLite、FrameService、Hub、图片不可变 ID 和 multipart 严格契约；不另建并行命令状态机。
- 图片上传本身不结束命令或创建告警；先持久提交图片，再发设备 Ack；命令成功结果只通过有效 Ack 产生。
- 图片 I/O 不持有 Monitor 锁或长 SQLite 事务；有界工作队列与进程 context，避免 HTTP 上传阻塞 MQTT 回调。
- Windows 存储修复已归档，文件同步/不可覆盖发布及 Linux 目录同步保障保留；图片实际跨平台运行证据仍后置。

# Decisions

- 用户已确认功能优先路线和存储修复验收；本 change 承接 F1 主功能，不等待测试或 Broker 环境。
- 前置 change 已归档；其实现与正式产物已提交，创建时当前目录干净，无其他 active change，沿用 go后端开发/current。
- 用户已明确确认完整 Shape：quality 可省略且默认 80、显式为 1—100 整数，同设备可读 snapshot 回执、稳定 URL、软件模拟器及 A1—A7 代码交付标准；测试继续后置。依据为 2026-10-10 的结构化答复“确认，按此契约实施”。
- 严格同设备、snapshot 类型、已提交且文件可读；不依赖设备时钟或 frame_id 绑定命令。命令截止及非法回执行为沿用既有 command-control 契约。
- 单一拍照闭环共同修改命令、模拟器与图片读取，保持单 change；告警关联与部署鉴权留各自 change，不启用 Supervisor。

# Open questions

无。完整目标、范围、关键契约、验收与非目标已确认。

# Verification expectations

- Build 开始前读取本 brief 与完整 command-control Spec；只在用户确认 Shape 后实施。
- 开发阶段只进行格式、编译、差异检查；不新增或运行测试，不安装 Broker/测试工具链。
- Runtime 执行 go build ./...；新的只读 Verifier 逐项核对 A1—A7 的实际实现与文档，报告真实代码问题，不将延期测试写为通过或作为本轮门槛。
- V1 集中验证参数/图片引用边界、普通及重复/冲突/迟到 Ack、设备模拟上传失败/取消/重试/并发、持久恢复、HTTP/双 WS 与全量 test/vet/race。V2 验证实际 Broker/Docker、Python/Vue、摄像头与硬件。缺失证据保持待验。
