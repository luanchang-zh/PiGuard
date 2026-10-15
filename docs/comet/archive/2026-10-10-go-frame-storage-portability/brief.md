# Outcome

先补齐 Go 后端业务功能，再集中进行测试和环境联调。下一项主要功能为 camera.snapshot 闭环；当前 change 的 Windows 图片上传修复作为该功能的必要前置小项，不再要求先完成跨平台测试、race 工具链或 Broker 环境。用户已确认按此路线实施。

## 2026-10-10 后端现状

基线：`go后端开发`，`origin/go后端开发` 从 `0e65a53` 快进至 `8379665`，拉取前工作区干净。依据为课程设计、网络 API 规范、当前实现、五份已归档 Native 验收记录及本机重新运行的检查。只评估 Go 后端，不把 Vue、Python 感知/规则引擎和硬件驱动算入分母。

**工程完成度估算约 80%（参考区间 75%—85%）。** 权重是本次规划对工作量与交付重要性的估计，不是测试覆盖率、验收通过率或代码行比例；加权值 79/100，取整表达为约 80%。

| 能力 | 权重 | 完成分值 | 证据与缺口 |
| --- | ---: | ---: | --- |
| 设备、遥测、freshness、历史和实时 WS | 25 | 25 | telemetry/status、TTL、历史及双 WS 软件测试已有实现；真实网络证据在交付项单列 |
| 四类动作命令/Ack | 15 | 15 | buzzer.test、indicator.test、scenario.start/stop；终态、重复/迟到 Ack、超时和恢复已有软件验收 |
| 告警事件 | 10 | 9 | 幂等、筛选、历史和 WS 已实现；snapshot_id 只记录引用，尚无有效图片关联闭环 |
| 配置同步 | 15 | 15 | 条件版本、retained 下发、回执、恢复最新版本及旧传输隔离已有软件验收 |
| 图片上传、存储、读取和 WS | 15 | 10 | 已归档验收覆盖 JPEG、幂等、排序、失败补偿及重启；Windows 实测出现上传回归 |
| camera.snapshot 完整闭环 | 10 | 0 | 当前只支持四类动作；拍照命令、软件设备上传及有效图片 Ack 尚未接通 |
| 运行交付与真实网络联调 | 10 | 5 | 已有 Docker/健康检查和遥测 Broker 集成测试入口；完整命令/配置/图片真实链路、HTTP 鉴权和 Broker ACL 尚缺 |
| 合计 | 100 | 79 | 约 80%，后续按新验收证据更新 |

### 本轮实测及环境

- `go test -count=1 ./...`：失败。handler/service 图片测试在目录 `Sync()` 返回 `Access is denied`，合法上传为 500；定位到 `go-backend/internal/service/frame_store.go` 目录同步步骤。其他有测试的包在该次普通测试输出中通过。
- `go vet ./...`：通过；串行命令中退出 0 后才启动 race。
- `go test -race -count=1 ./...`：测试程序启动退出 `0xc0000139`，没有有效竞态验证。当前竞态运行环境不可用，具体缺失运行库尚未确认；不宣称代码存在竞态或检查通过。
- Go 1.25.1 / windows/amd64，CGo 已启用。Docker CLI 存在，但 Docker Desktop Linux engine 不可连接；PATH 未发现 Mosquitto。本轮未执行真实 Broker、Docker 容器或树莓派联调。
- Comet CLI 0.4.1 无法识别远程配置/图片归档状态中的 `document_constraints_version`。保留原状态及验收报告，历史报告仅作为原候选的软件证据。新 change 正常创建、选择和读取；旧状态的兼容性应由兼容版本 Runtime 处理，不手改状态删字段。
- 检查日志：FastCtx 作业 `j-96g16t`（普通测试）、`j-bhla3n`（vet 后接 race）。本轮没有提交、推送或修改后端实现。

# Scope

本 change 只实现图片上传功能修复，为后续拍照闭环打通存储路径：

- Windows/Linux 均能上传、读取、恢复已提交 JPEG；正常上传不因平台不支持目录同步返回 500。
- 调整平台相关持久化步骤，区分不支持的操作与真正的文件/磁盘失败，不能笼统忽略存储错误。
- 保留严格校验、并发幂等、不可覆盖发布、预览排序、失败补偿、原字节读取和 WS 语义。
- 同步 README 与完整 frame-storage Spec 的平台及持久化边界。开发阶段只保留格式、编译检查和代码核对；不新增或运行测试，不先处理 race、Docker 或 Broker 测试环境。

## 功能优先路线（具体功能契约在对应 change 的 Shape 中确认）

| 优先级 | 建议 change | 交付结果 | 开始条件 |
| --- | --- | --- | --- |
| F1 前置小项 | go-frame-storage-portability（本轮） | 修复 Windows 正常上传 500，保留现有 JPEG/幂等/读取契约；完成代码与文档后继续功能开发 | 确认本轮修订 Shape；不以完整测试或外部环境为前置 |
| F1 主功能 | go-camera-snapshot-loop | commands API 接受 camera.snapshot；MQTT 下发；软件设备生成/上传 JPEG；Ack 返回同设备有效 snapshot_id；GET/WS 返回结果并提供内容 URL；实现失败、超时及重复命令处理 | 存储代码修复完成；在该 Shape 确认 quality 范围、缺省值及图片归属/迟到结果契约；不等待测试阶段 |
| F2 | go-event-snapshot-link | 实现告警与同设备图片关联，给前端明确图片可读状态，处理事件/图片先后到达及错误引用 | 图片及拍照功能代码完成；确认乱序关联规则 |
| F3 | go-demo-delivery | 实现演示部署需要的 HTTP/WS/图片鉴权、MQTT 用户/ACL 配置与前端跨源访问配置；完善启动参数、操作示例和协议文档 | 主业务代码完成；认证方式与部署拓扑在该 Shape 确认 |
| V1（统一后验） | go-backend-functional-verification | 集中补齐并运行单元/组件回归、并发/故障/恢复检查、test/vet/race，准备 Windows/Linux 所需环境并处理发现的问题 | F1—F3 功能代码完成后再开始，不作为前三项的前置门槛 |
| V2（统一后验） | go-broker-end-to-end | 真实 Mosquitto/Docker 验证重连、retained、遗嘱与完整上/下行，再接 Python edge、Vue 和硬件联调 | 功能代码完成且进入统一验证阶段 |
| 后续增强 | go-storage-retention / command-history | 按演示需求增加图片保留/容量清理、命令历史列表等增强 | 主业务功能与统一验证完成后再确定；不扩大本轮范围 |

F1—F3 按代码功能交付顺序推进，V1/V2 集中收集运行证据。测试延后不改变既有功能契约，也不把未测试功能写为“运行验收通过”。后续 change/worktree 本轮不预先创建；当前存储修复仍为单 change，后续主要功能使用各自 Native change，不建立未经确认的 Supervisor。

# Non-goals

- 本轮不实现 camera.snapshot、告警图片关联、Python/Vue、硬件、鉴权、CORS、TTL/删除、容量清理、对象存储或视频。
- 不改变已有 JPEG、身份、排序、DTO、错误码及 WS 契约；保留已有测试，不删改断言来隐藏问题。
- 本轮不补测、不跑 test/race/vet、不安装或排查测试工具链，不运行真实 Broker/Docker/硬件联调；这些统一后移到 V1/V2。
- 不手改 comet-state.yaml、删除未知状态字段或重开已归档 change。
- 不承诺超出平台/文件系统支持的硬断电恢复，不把历史或软件验收当成本轮真实环境通过。

# Acceptance examples

- A1：代码完成 Windows/Linux 的持久化适配，合法图片上传不再无条件走 Windows 不支持的目录 Sync 分支；普通文件同步与 Linux 目录持久化保留，不能笼统吞掉权限/磁盘错误。
- A2：代码保留 (device_id, frame_id) 幂等与冲突处理、不可覆盖发布、preview 排序以及提交后 WS 通知路径；不更改现有 DTO、JPEG 限制或错误码。
- A3：代码保留文件/DB 失败与提交前取消的补偿，提交后断连仍保留可重试结果；路径约束及未知/缺文件 404、其他存储错误 500 的映射不被移除。
- A4：代码继续从持久 SQLite/图片目录读取身份、预览与原字节，未改为仅内存状态；没有加入图片清理或删除行为。
- A5：修改的 Go 文件格式符合项目要求，`go build ./...` 可以编译当前实现；不以 test/race/vet、Broker、Docker 或另一平台的测试环境阻塞后续业务功能开发。编译仅证明可构建，不代替运行验收。
- A6：README/Spec 明确功能代码完成与运行验证延后的区别，并记录 V1/V2 的待验项目；已有测试保留，未运行项目不得记作通过。

这些是本轮代码交付标准。HTTP 实际返回、并发/故障/重启与跨平台运行结果属于 V1 的运行验收，当前不宣称通过。

# Constraints and invariants

- 使用现有 SQLite、storage.snapshot_dir、不可变 ID 和 Hub。
- 文件可读且完成适用持久化后才提交元数据，提交成功后通知；图片 I/O 不持有 Monitor 锁或长 SQLite 事务。
- 支持目标保留 Windows 开发与 Linux Docker 部署；当前只实施平台兼容代码，实际双平台验证延后。
- Linux 已有可用文件/目录持久化保障不得因 Windows 修复被统一削弱；Windows 用受支持方式并写清保障范围。

# Decisions

- 用户于 2026-10-10 回复“做吧”，确认修订后的功能优先路线及当前存储修复 A1—A6；不再重复询问同一范围的实施确认。
- 创建前目录干净且无 active change，按工作区参考使用 current，绑定 `go后端开发`。
- 用户于 2026-10-10 明确要求“测试延后，目前主要是要求完成功能，重新规划”。据此移除先跑跨平台/race/Broker 检查的开发前置；用户决定已同步 brief/Spec，尚未开始实施。
- Windows 上传故障会阻塞拍照链路，保留为 F1 的必要小修复；主功能优先补 camera.snapshot，其后补告警图片引用。
- 普通实现选择由 Builder 处理，新只读 Verifier 只核对本轮代码交付范围，完整运行验证在 V1/V2 进行。
- 本轮围绕单一图片存储契约，不做 Supervisor 拆分。

# Open questions

无。功能优先路线及本 change 的修订代码交付标准已获确认。

# Verification expectations

- Builder 先读取本 brief、完整 Spec 及 A1—A6，不修改 Runtime 状态。
- Runtime 当前只执行与代码交付相关的必要格式/编译检查；本轮不安排 test/race/vet、真实 Broker 或双平台运行验证。
- 新只读 Verifier 根据完整代码、Spec 和编译结果逐项核对代码交付 A1—A6；明确尚无运行验收，不将缺失测试误报为已通过或作为本轮推进门槛。
- V1/V2 再集中补测、运行软件/真实环境验收并处理缺陷。后续分别报告功能实现进度和运行验收进度，不因延后测试上调当前完成率。
- 后续 Build→Verify→Archive 只执行 Runtime continuation；按用户确认开始实现，测试与联调延后。
- 本机 Runtime 0.4.1 不支持从 await-user 直接重新准备 Shape；本轮已修订正式正文，未手改状态。其已有确认快照仍来自上轮。实施前必须通过 Runtime 的需求漂移处理重新准备并保存本轮 A1—A6，不得直接按旧的测试门槛实施；用户对修订完整方案的明确确认可沿用，不因内部快照刷新要求重复确认同一范围。
