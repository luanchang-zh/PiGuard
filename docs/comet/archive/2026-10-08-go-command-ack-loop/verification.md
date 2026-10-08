---
generated_from_state_version: 14
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 2
- 迭代: 2
- 验证器尝试次数: 1
- 完成时间: 2026-10-08T12:22:58.974Z
- 摘要: 独立只读核查完整brief A1—A10、完整Spec、当前实现与测试，随后核对候选绑定Runtime结果和日志，最后读取Builder handoff。A1—A10全部通过；前轮A5/A6诊断不足和A10竞态检查缺证已解决。无需补检，真实环境延期边界保留。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：四类合法请求通过校验。在线且 Broker 可用的设备收到请求后，HTTP 202/code 0 返回 command_id 和 pending；数据库已存在唯一记录。MQTT 下发含 schema_version=1、同一 ID/设备、类型、参数及 UTC 签发/到期时间，QoS 1、retained=false。 | protocol严格校验四类参数；Create生成唯一cmd ID和UTC时间、expires_at=issued_at+10秒，先插入pending再入异步队列。handler固定返回202/code0/pending；命令发送QoS1、retained=false。TestCommandsHTTPAndTwoWebsockets覆盖四类请求及重复有效POST生成新ID，Runtime test/race通过。 |
| A2 | passed | brief.md | A2：非法 JSON、缺失/越界参数、未知或本轮不支持的类型返回 HTTP 400/40001；未知设备返回 404/40401；未知 command_id 返回 404/40402。上述无效创建请求不入库、不发布。 | 单对象解码拒绝非法JSON、额外字段、尾随JSON、null/缺失/错误类型/越界参数及未支持类型。HTTP错误映射为400/40001、404/40401、404/40402。协议和HTTP拒绝测试通过，并断言无效创建请求的commands记录数为0；拒绝路径不调用发布。 |
| A3 | passed | brief.md | A3：已知离线设备返回 HTTP 503/50301；设备在线但 Broker 不可用返回 503/50302。两种前置拒绝均不创建命令或发布。受理后发布明确失败写 failed，发布结果不确定时保留待决并最终超时；不得宣称设备执行成功。 | 创建前检查登记、在线事实、工作过程及发布可用性；离线和发布不可用映射50301/50302且不入库。明确未提交错误仅将仍pending记录改failed/MQTT_UNAVAILABLE；token错误、等待超时或取消保持交付不确定，不伪造执行失败。HTTP拒绝与发送结果/截止测试通过。 |
| A4 | passed | brief.md | A4：单独收到 Broker publish 成功，GET 与 WS 显示 sent；合法 success Ack 才写 success 和 result，合法 failed Ack 写 failed 和设备 error，两类都保留执行时间和平台接收时间。 | publish成功仅条件转换pending→sent；success/设备failed由合法Ack转换并分别保存结构化result/error、平台ack_at和设备executed_at。状态转换持久成功后发布统一WS更新。服务发送结果、快速Ack及HTTP模拟失败/成功测试通过。 |
| A5 | passed | brief.md | A5：Ack 早于 publish 完成时仍可从 pending 进入终态；随后 publish 完成不能覆盖回 sent。重复相同 Ack、冲突 Ack 不重复改变终态/广播；非法版本、主题身份不符、未知命令或跨设备 Ack 不更新记录。 | Ack可直接结束pending，发送完成重新读取当前记录且不能覆盖终态；仓储转换包含当前状态条件。非法版本/主题身份/未知命令/跨设备Ack拒绝且不广播。快速Ack、并发冲突及八个诊断场景通过；相同Ack分类duplicate，结果/错误/时间/状态差异分类conflict，均保留首个终态且不重复广播。 |
| A6 | passed | brief.md | A6：在签发后 10 秒截止前未收到有效 Ack，pending 和 sent 均变为 timeout；迟到 Ack 不改终态，只留诊断。timeout 表示未及时确认，不宣称设备一定没有执行。边界时间采用平台接收有效 Ack 的时间，不信任设备时钟决定超时。 | 统一10秒截止；Ack接收与截止处理在同一临界区排序，received_at取平台时间，等于或晚于expires_at不能写Ack终态。100ms扫描和GET读取前持久超时覆盖pending/sent。截止边界与两个迟到路径测试通过，日志保留Ack状态、executed_at、received_at和expires_at；迟到Ack不改变终态或结果。 |
| A7 | passed | brief.md | A7：平台重启后保留已结束命令；从数据库恢复 pending/sent，已过截止时间变 timeout，未到期继续等待 Ack；不自动重新发布动作。Broker 重连后可继续接收新命令的 Ack。 | Start从数据库扫描过期pending/sent并转timeout；未到期记录继续等待Ack，既有终态保留，恢复不向发送队列重放。TestCommandRecoveryNeverReplays覆盖manager重建及无发布。Subscriber首次订阅失败可见，OnConnect恢复三个主题；软件客户端重连订阅测试通过。实际Broker/平台进程生命周期仍属明确延期边界。 |
| A8 | passed | brief.md | A8：GET 返回 command_id/device_id/type/params/status/issued_at/expires_at/ack_at/executed_at/result/error；没有 Ack 时 ack_at/executed_at 为 null。WS command_update 使用统一 type/timestamp/data 信封，状态来自同一持久记录；两个客户端可见更新，慢或断开连接不阻塞命令或遥测链路。 | GET使用明确CommandView包含全部规定字段，无Ack及无结果/错误输出null；WS通过统一type/timestamp/data信封发送同一持久记录DTO。HTTP双WS测试比较同ID、类型、参数和结果；共享Hub的有界队列、慢客户端驱逐及WS关闭回归通过，命令发布等待位于Monitor锁外。 |
| A9 | passed | brief.md | A9：模拟器订阅 commands，提供成功、设备失败、不回执三种模式，按 command_id 缓存结果；重复命令只重发同一 Ack，不再次执行，已过 expires_at 的新动作不执行。软件组件联调核对 HTTP/模拟发布器/模拟设备执行器/GET/双 WS 的同一 ID 及成功、失败、超时和持久恢复行为。 | 模拟器commands订阅调用实际CommandExecutor，提供success/failed/none模式，验证身份、类型参数及截止；按ID缓存Ack或无Ack结果，重复投递不再执行，过期新命令返回COMMAND_EXPIRED且执行计数不增。软件组件测试串联HTTP→模拟发布器→实际执行器→Ack→SQLite/GET/双WS，覆盖四类成功及失败/无Ack超时；服务层补充持久恢复和异常Ack/竞争边界，均由Runtime执行通过。 |
| A10 | passed | brief.md | A10：必要的 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 通过；既有单元与组件层的监控排序、TTL、1Hz 历史和双 WS 行为保持通过。按用户最新约束，本轮不启动 Docker、不做硬件测试，真实 Mosquitto 和实际 Broker/平台进程生命周期检查留待后续且明确标为未执行，不算通过。 | 当前候选绑定的Runtime go-tests、go-race-supported-toolchain、go-vet均passed且exit_code=0；已读取对应日志核实相关包通过。正式命令包含-count=1和全包范围，race使用兼容Windows便携工具链。监控排序、TTL、每设备1Hz历史、并发、双WS及慢客户端回归包含在本次通过的软件检查中；integration标签的真实环境测试未执行。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go全部单元与软件组件测试 | test -count=1 ./... | go-backend | passed | 0 | 33788 ms |
| Windows便携MinGW-w64 14工具链的Go全部竞态检查 | CGO_ENABLED=1 CC=C:/Users/23156/AppData/Local/Temp/piguard-race-toolchain/w64devkit/bin/gcc.exe CXX=C:/Users/23156/AppData/Local/Temp/piguard-race-toolchain/w64devkit/bin/g++.exe C:/Users/23156/sdk/go1.25.1/bin/go.exe test -race -count=1 ./... | go-backend | passed | 0 | 47688 ms |
| Go全部静态检查 | vet ./... | go-backend | passed | 0 | 5814 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- Ack诊断组件测试: passed — go test -count=1 ./internal/service -run TestCommandIgnoredAckDiagnostics：重复成功/失败、结果/错误/执行时间/状态冲突、已timeout迟到与截止扫描前迟到，共8个场景通过。
- 代码格式与差异检查: passed — gofmt已执行；git diff --check通过。
- 全量Go test/race/vet: not-run — 上一候选的Runtime test/vet通过但race环境启动失败；此候选源代码已改，交给Runtime正式运行全部三项，不把上一候选证据当新候选通过。
- 已知限制: 用户明确本轮不启动Docker、不做硬件测试；真实Mosquitto、实际MQTT网络重连和后端进程生命周期联调未执行，按最新A9/A10延期。
- 已知限制: 没有HTTP幂等键，每次有效POST创建新命令；重启不自动重发动作。模拟器去重缓存仅限当前进程，不代表Python或硬件执行器。
- 已知限制: 兼容便携工具链仅在C:/Users/23156/AppData/Local/Temp/piguard-race-toolchain，正式竞态证据须由Runtime生成。

## 阻塞项

_无。_

## 风险与跳过的工作

- 按用户最新约束，本轮未启动Docker、未做硬件测试；真实Mosquitto、实际MQTT网络重连和后端进程生命周期联调均未执行，不认定这些真实环境检查通过。
- 重连证据来自软件Paho客户端替身，持久恢复证据来自SQLite记录和CommandManager重建；实际Broker与后端进程重启仍需后续联调。
- 模拟器去重缓存仅在当前进程内，模拟执行不代表Python执行器或真实蜂鸣器/指示灯行为。
- Windows竞态检查依赖临时目录中的兼容便携工具链；当前候选正式race证据已通过，后续环境需保持兼容编译器。
- 本轮没有HTTP幂等键；每次有效POST生成新命令，平台恢复不自动重发动作。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 0 | recovery | — | Native confirmed acceptance criteria changed | 2026-10-08T11:45:14.074Z |
| 2 | 1 | 1 | fail | A5, A6, A10 | 独立读取完整brief A1—A10、Spec、实际实现与测试，再核对Runtime正式检查，最后核对Builder交接。七项通过；A5/A6因完整规范要求的Ack诊断分类和迟到执行事实缺失未通过；A10因race测试进程启动失败暂时无法验证。应返回Build补齐诊断，并在后续候选由Runtime补齐兼容工具链的正式race证据。 | 2026-10-08T12:09:23.167Z |
| 2 | 2 | 1 | pass | — | 独立只读核查完整brief A1—A10、完整Spec、当前实现与测试，随后核对候选绑定Runtime结果和日志，最后读取Builder handoff。A1—A10全部通过；前轮A5/A6诊断不足和A10竞态检查缺证已解决。无需补检，真实环境延期边界保留。 | 2026-10-08T12:22:58.974Z |



## 结论

独立只读核查完整brief A1—A10、完整Spec、当前实现与测试，随后核对候选绑定Runtime结果和日志，最后读取Builder handoff。A1—A10全部通过；前轮A5/A6诊断不足和A10竞态检查缺证已解决。无需补检，真实环境延期边界保留。
