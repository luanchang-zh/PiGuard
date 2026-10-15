# Outcome

规划 PiGuard Go 后端下一步：先交付图片上传与读取，形成软件闭环 `HTTP multipart JPEG → 文件与 SQLite 元数据 → latest preview/content → frame_update → 双 WS 客户端`。

当前只做 Shape 规划。用户已确认 Q1 幂等重试、Q2 采集时间排序和 Q3 2 MiB 上限；完整 Shape 尚待确认，未进入 Build，不改业务代码。

## 已核实的现状

- 当前分支 `go后端开发`，本次规划前工作区干净，继续使用当前目录。
- 实时监控、动作命令/Ack、告警事件、配置同步均已归档，分别验收 9/9、10/10、7/7、10/10。
- `internal/handler/frame.go` 的三条路由已注册，但 service/repo 仍返回未实现；`model.Snapshot` 只有基本路径与时间字段，还没有上传幂等身份。
- `storage.snapshot_dir` 已有路径解析和启动目录准备；实时 Hub 已支持双客户端、有限队列和慢客户端关闭。
- 网络规范第 69 节将 Frame HTTP 上传排在 Config 之后；上轮路线图的 P3 是图片、拍照命令和引用。

# Scope

本轮建议收敛为 P3a 图片基础能力，新增 `frame-storage` capability：

1. POST `/api/v1/devices/{device_id}/frames`，严格读取 file、frame_id、captured_at、type，支持 preview/snapshot/event，实际验证 JPEG 后落盘和保存元数据。
2. GET `/api/v1/devices/{device_id}/frame`，返回最新 preview 的 snapshot_id、captured_at、type 和稳定 content URL；snapshot/event 上传不覆盖预览。
3. GET `/api/v1/snapshots/{snapshot_id}/content`，读取持久 JPEG 原始字节。
4. 明确上传重试、乱序预览和上传大小契约，处理文件/数据库失败、并发、请求取消和重启。
5. 提交成功后发布 frame_update；通知不包含图片字节，本轮不新增 config_update。
6. 使用临时 SQLite/文件目录、实际 HTTP multipart/JPEG、双 WS 客户端和失败注入进行软件验收，更新 README 与网络规范。

## 来源与覆盖

本次文档用于仓库现状与接口设计依据，覆盖图片相关段落及共享响应约束，不将整份项目设计纳入本 change。

| 来源条目与位置 | 读取状态 | 需要保留的内容 | Spec 位置 | 验收 ID | 覆盖状态 | 理由 |
| --- | --- | --- | --- | --- | --- | --- |
| 网络规范第 41—42 节 | complete | multipart 四字段，preview/snapshot/event，返回 snapshot_id/device/type/captured_at | R1—R3 | A1—A3 | covered | 用户确认同设备/frame_id 幂等，内容冲突 409 |
| 第 43 节 | complete | 最新预览返回 content URL 的 JSON | R4 | A4 | covered | 用户确认采集时间排序，迟到旧图不覆盖 |
| 第 44 节 | complete | content 直接返回 image/jpeg | R5 | A5 | covered | 读取已持久图片，不暴露路径 |
| 第 46、51 节 | complete | 通用 WS 信封与 frame_update 标识，不传大图片 | R7 | A7 | covered | 上传持久成功后才通知 |
| 第 52—54 节 | complete | HTTP 响应信封、字段错误和状态码 | R2—R5 | A1—A5 | covered | 在 Build 同步图片 404/409/413 错误码补充 |
| 第 63、64 节 | complete | 预览推荐 640×360/480，JPEG quality 60—80，HTTP 1—2 FPS | R2、R8 | A2、A8 | background | 用于软件样本；不在本轮新增生产限频或强制该分辨率 |
| 第 68 节 snapshots | complete | 图片文件与 SQLite 元数据分离 | R6 | A6 | covered | 扩展身份和摘要字段，不把 JPEG 存入数据库 |
| 第 69 节与上轮 brief 后续路线图 | complete | Config 后接 Frame，后续部署与环境联调 | R8 | A8—A9 | background | 确定优先级，旧的“P2b 下一轮”已经完成 |
| 第 19、39、67 节 | complete | camera.snapshot 执行后上传再 Ack 引用 snapshot_id | — | — | non-goal | 作为 P3b，依赖本轮图片基础；本轮不提前宣称支持该命令 |
| 第 61—62 节 | complete | 鉴权与 MQTT ACL | — | — | non-goal | 独立的 P4，保持现有开发环境接口边界 |

## 后续路线

| 顺序 | 建议能力 | 完成标志 |
| --- | --- | --- |
| P3a（本轮） | 图片上传、读取、frame_update | 上传返回 ID、最新预览可读、原始 JPEG 可读、双 WS 可见 |
| P3b | camera.snapshot 与图片引用 | 下发拍照 → 软件设备上传 → Ack 返回该设备有效 snapshot_id；补告警引用查询约定 |
| P4 | 部署与运行完善 | HTTP 鉴权、设备 ACL、就绪检查、Broker 持久化、图片保留与清理策略 |
| 环境联调 | 真实 MQTT/容器/树莓派 | 逐模块补真实环境证据，不将软件模拟记成硬件通过 |

# Non-goals

- 不实现 camera.snapshot、Python 驱动、Vue 页面、规则引擎、自动设备注册或新的鉴权方案。
- 不修改已归档监控/命令/事件/配置业务语义；图片上传不更新 online、last_seen_at 或配置确认版本。
- 不引入 PNG/视频流、MQTT 图片传输、对象存储、缩略图、重新压缩、图片删除 API 或后台 TTL 清理。
- 图片本轮持久保留；容量保留策略放到 P4。上传的旧预览也可用其稳定 ID 读取。
- 真实 Broker、Docker、摄像头和树莓派不作为本轮通过条件，沿用已确认的软件验收边界。

# Acceptance examples

- A1：已登记设备上传合法 JPEG、frame_id、带时区 captured_at 和 preview/snapshot/event，返回 HTTP 201/code 0、稳定且不暴露数据库 ID 的 snapshot_id/device_id/type/captured_at。元数据归一 UTC，JPEG 落入配置目录，离线设备与 MQTT 断线不额外拒绝图片请求，不改设备在线或配置事实。
- A2：拒绝缺失/重复/未知字段、多个 file、错误 multipart、空或非法 frame_id、非法时间/type、空文件、伪 JPEG、损坏或截断 JPEG；返回 400/40001 和字段路径。单张 JPEG 最大 2 MiB，总请求最大 2 MiB+64 KiB，单边≤4096、总像素≤16,777,216；超限返回 413/40001，进行有界读取；任何拒绝不新增图片、记录或 WS 通知。
- A3：同设备/frame_id 的同元数据同字节重试返回 HTTP 200 和原 snapshot_id，只有一份文件/记录且不重复推送；同键冲突返回 409/40903，不覆盖首个内容。并发同键也只有一份成功持久记录，其他依相同重试/冲突规则响应；不同设备的相同 frame_id 相互独立。
- A4：GET frame 返回 HTTP 200/code 0 及当前 preview 的 ID、UTC captured_at、type 和稳定 URL；只上传 snapshot/event 不产生 preview。无预览返回 404/40403，未知设备 404/40401。按 captured_at 选择最新，迟到旧图不覆盖；采集时间相同时按平台成功提交顺序决定。乱序与并发查询的 URL 和当前记录一致。
- A5：GET content 返回 HTTP 200/image/jpeg 和保存的原始字节；未知 ID 或已登记但文件缺失返回 404/40403，其他读取故障 500/50001。不使用上传文件名或 frame_id 构造路径，不接受任意文件路径，不暴露本地目录。
- A6：文件写入/最终落盘失败、数据库提交失败或提交前请求取消不产生可见的不完整记录、不推进最新预览或通知；正常失败回收本请求创建的临时与未提交文件。已成功提交后连接断开不撤销图片，幂等重试可找回原结果。重启保留已提交图片和元数据，临时/未引用文件不被当成有效图片；不自动清理用户文件或历史图片。
- A7：每个新接受的 snapshot/event、以及成为当前 preview 的图片，在文件和元数据可读后发送一次 frame_update；timestamp 为平台时间，data 有 device_id/snapshot_id/type/captured_at/url，没有 JPEG 字节。幂等重试与拒绝不重复通知；迟到旧 preview 不发送 frame_update，按采集时间/同时间提交顺序保证通知不使预览回退。双客户端收到一致信息，慢客户端不阻塞上传与既有业务。
- A8：软件组件测试真实串联 HTTP multipart 上传→文件/SQLite→最新预览 URL→content 原始 JPEG→双 WS，覆盖三种图片类型、并发、重试、乱序、失败与数据库/文件重开。既有监控、命令、事件、配置 DTO/WS 类型与业务语义回归保持通过。
- A9：在 go-backend 执行 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 均通过；README 和网络规范同步实际契约与新增错误码，写清 camera.snapshot 仍在下一轮。真实 Broker/Docker/硬件明确未执行，不混淆软件证据。

# Constraints and invariants

- 上传接受后图片不可变；内容 URL 指向该图片，不指向可变的“当前文件”。文件位于 storage.snapshot_dir，数据库保存服务端生成的路径和必要元数据。
- 文件系统和 SQLite 不是同一事务，必须设计可见性顺序、失败补偿和重启后的只读检查；文件保存成功不能自动当成数据库提交成功。
- multipart 读取、JPEG 校验和网络内容读取不持有 Monitor 锁或长 SQLite 事务；有界读写并响应请求取消，不将图片字节塞入 Hub。
- 最新规则、重试身份与资源上限均已由用户确认。取得完整 Shape 确认前不进入 Build。
- 本轮使用一个 Native change：HTTP、文件、数据库和通知共同完成一个结果，拆开独立实现容易留下读不到图片的成功响应；不建立 Supervisor 子任务。

# Decisions

- 根据已归档能力、代码占位和网络规范顺序，推荐先做 P3a 图片基础，再规划 P3b 拍照命令与关联。本轮尚待整体 Shape 确认。
- 图片类型沿用 preview/snapshot/event，内容沿用 JPEG；latest 接口选择规范推荐的 JSON URL。
- 复用当前 storage.snapshot_dir、SQLite 和实时 Hub；不改 MQTT 或引入对象存储。
- 只做软件验收；每轮 Build 后由新的只读 Verifier 按 A1—A9 判断，正式 test/race/vet 由 Runtime 执行。
- Q1 已确认：同设备/frame_id 幂等；同元数据同字节重试返回原 snapshot_id、不重复保存或通知；不同内容或元数据冲突返回 409/40903。
- Q2 已确认：用户明确要求按推荐方案，以 captured_at 选择最新预览；迟到旧图不覆盖，相同采集时间按平台提交顺序决定。
- Q3 已确认：用户明确要求按推荐方案，JPEG ≤2 MiB、总请求 ≤2 MiB+64 KiB、单边≤4096且总像素≤16,777,216。

# Open questions

无未解决的产品决定。Q1—Q3 已回答并同步到完整规格，等待整体 Shape 确认。

# Verification expectations

Shape：核对图片相关来源、已确认 Q1—Q3 和完整规格，保存 A1—A9；用户确认完整目标后才进入 Build。

Build：协议/校验、真实临时文件和 SQLite、并发幂等与最新查询、故障注入和重启、multipart/content/双 WS 定向测试；以程序生成小 JPEG 测试样本，无需实际摄像头。

Verify：冻结候选后由 Runtime 执行 Go test/race/vet，新的只读 Verifier 独立核对 A1—A9；重复检查只在证据缺失或失效时补跑。真实环境只记录未执行。

参考：网络规范第 41—44、46、51—54、63—64、68—69 节；拍照后续依赖第 19、39、67 节；上轮归档 brief 的后续路线图。
