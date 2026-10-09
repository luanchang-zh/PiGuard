---
generated_from_state_version: 10
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 1
- 迭代: 2
- 验证器尝试次数: 1
- 完成时间: 2026-10-09T13:18:41.503Z
- 摘要: 独立验收当前候选A1—A9全部通过。已完整核对brief/Spec、源码、软件组件测试及同候选Runtime检查/历史日志；复用全量test/race/vet有效回执，无缺失或失效检查需要补跑。未把Builder自报检查或真实环境未执行项目当成正式通过证据。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：已登记设备上传合法 JPEG、frame_id、带时区 captured_at 和 preview/snapshot/event，返回 HTTP 201/code 0、稳定且不暴露数据库 ID 的 snapshot_id/device_id/type/captured_at。元数据归一 UTC，JPEG 落入配置目录，离线设备与 MQTT 断线不额外拒绝图片请求，不改设备在线或配置事实。 | protocol/frame.go、handler/frame.go与service/frame.go核实三种类型、UTC DTO、服务端随机稳定ID及配置目录存储；TestFramesHTTPContentTwoWSAndRestart覆盖201合法上传、原始内容与设备/配置事实不变。服务依赖无MQTT/online限制，未知设备为40401。证据为当前候选Runtime全量test/race。 |
| A2 | passed | brief.md | A2：拒绝缺失/重复/未知字段、多个 file、错误 multipart、空或非法 frame_id、非法时间/type、空文件、伪 JPEG、损坏或截断 JPEG；返回 400/40001 和字段路径。单张 JPEG 最大 2 MiB，总请求最大 2 MiB+64 KiB，单边≤4096、总像素≤16,777,216；超限返回 413/40001，进行有界读取；任何拒绝不新增图片、记录或 WS 通知。 | multipart明确四字段、拒绝缺失/重复/未知/多文件/错误正文；frame_id、带时区时间、type及JPEG尺寸/完整解码校验。MaxBytesReader与ContextReader有界且响应取消。协议和HTTP拒绝测试验证400/413、40001字段错误及零记录/文件/通知；资源边界覆盖2 MiB、总正文及4096×4096。 |
| A3 | passed | brief.md | A3：同设备/frame_id 的同元数据同字节重试返回 HTTP 200 和原 snapshot_id，只有一份文件/记录且不重复推送；同键冲突返回 409/40903，不覆盖首个内容。并发同键也只有一份成功持久记录，其他依相同重试/冲突规则响应；不同设备的相同 frame_id 相互独立。 | SQLite(device_id,frame_id)唯一索引和Accept事务负责并发身份；retry核对归一时间、类型、长度、摘要和原始字节，返回原ID且不推送。HTTP测试覆盖重试、元数据/字节冲突、24路同键及12路冲突、跨设备同frame_id，验证最终文件/记录唯一。 |
| A4 | passed | brief.md | A4：GET frame 返回 HTTP 200/code 0 及当前 preview 的 ID、UTC captured_at、type 和稳定 URL；只上传 snapshot/event 不产生 preview。无预览返回 404/40403，未知设备 404/40401。按 captured_at 选择最新，迟到旧图不覆盖；采集时间相同时按平台成功提交顺序决定。乱序与并发查询的 URL 和当前记录一致。 | FindLatest只查preview，按采集秒/纳秒/提交自增ID降序，从同一行构造ID、UTC时间、type和稳定URL。HTTP/服务测试覆盖无预览40403、未知设备40401、snapshot/event隔离、迟到旧图、同时间提交、并发纳秒排序及全RFC3339年份排序。 |
| A5 | passed | brief.md | A5：GET content 返回 HTTP 200/image/jpeg 和保存的原始字节；未知 ID 或已登记但文件缺失返回 404/40403，其他读取故障 500/50001。不使用上传文件名或 frame_id 构造路径，不接受任意文件路径，不暴露本地目录。 | 按snapshot_id参数查询并校验服务端文件身份，os.OpenRoot及本地单文件路径限制阻止路径逃逸；Content读取原始字节后响应image/jpeg。HTTP测试覆盖原字节、未知/文件缺失40403、目录/根外symlink/非法DB路径/DB故障50001，错误响应不泄露目录。 |
| A6 | passed | brief.md | A6：文件写入/最终落盘失败、数据库提交失败或提交前请求取消不产生可见的不完整记录、不推进最新预览或通知；正常失败回收本请求创建的临时与未提交文件。已成功提交后连接断开不撤销图片，幂等重试可找回原结果。重启保留已提交图片和元数据，临时/未引用文件不被当成有效图片；不自动清理用户文件或历史图片。 | diskFrameStore使用O_EXCL临时文件、Sync、不可覆盖Link及目录Sync，DB提交前文件可读；Save补偿未提交文件，提交成功后保留结果并通知。实际SQLite写入故障、磁盘不可用、发布冲突和取消阶段测试验证无半成品/预览/通知；提交后取消可幂等恢复。HTTP重开SQLite/目录验证持久身份、preview及内容，未引用文件不可查询且不自动删除用户文件。 |
| A7 | passed | brief.md | A7：每个新接受的 snapshot/event、以及成为当前 preview 的图片，在文件和元数据可读后发送一次 frame_update；timestamp 为平台时间，data 有 device_id/snapshot_id/type/captured_at/url，没有 JPEG 字节。幂等重试与拒绝不重复通知；迟到旧 preview 不发送 frame_update，按采集时间/同时间提交顺序保证通知不使预览回退。双客户端收到一致信息，慢客户端不阻塞上传与既有业务。 | 短提交门串行Accept与Hub.Publish，只有新snapshot/event或成为latest的preview在提交后发布frame_update；DTO含规定字段、平台timestamp，无JPEG。双WS测试验证一致信息和重试/冲突/旧preview无重复；并发排序验证通知采集时间不回退，服务与Hub测试验证慢队列关闭且不阻塞快客户端/上传。 |
| A8 | passed | brief.md | A8：软件组件测试真实串联 HTTP multipart 上传→文件/SQLite→最新预览 URL→content 原始 JPEG→双 WS，覆盖三种图片类型、并发、重试、乱序、失败与数据库/文件重开。既有监控、命令、事件、配置 DTO/WS 类型与业务语义回归保持通过。 | 独立读取实际HTTP/双WS/SQLite/磁盘夹具和协议、服务故障注入测试，覆盖三类型、读取、并发、重试、冲突、乱序与重开。当前Runtime全量test/race通过，监控、命令、事件、配置及Hub回归保持启用；event_test只同步图片占位的400/404预期，既有告警数量及DTO/WS业务断言保留。 |
| A9 | passed | brief.md | A9：在 go-backend 执行 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 均通过；README 和网络规范同步实际契约与新增错误码，写清 camera.snapshot 仍在下一轮。真实 Broker/Docker/硬件明确未执行，不混淆软件证据。 | 当前candidateId及fingerprint gate、projectRoot/cwd与Runtime证据一致；3433a993日志中go test -count=1 ./...、go test -race -count=1 ./...及go vet ./...均exit0，复用有效回执未重跑全量。README/网络规范同步实际JPEG、幂等、preview、content、WS、错误码及camera.snapshot后续边界。历史3043efdf失败核实为旧event_test的501占位预期，当前已修复并通过。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go 全量测试 | GOCACHE=/tmp/piguard-go-build-cache go test -count=1 ./... | go-backend | passed | 0 | 6648 ms |
| Go 全量竞态检查 | GOCACHE=/tmp/piguard-go-build-cache go test -race -count=1 ./... | go-backend | passed | 0 | 14278 ms |
| Go vet | GOCACHE=/tmp/piguard-go-build-cache go vet ./... | go-backend | passed | 0 | 319 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- 图片协议与服务定向检查: passed — GOCACHE=/tmp/piguard-go-build-cache go test ./internal/service ./internal/protocol -run TestFrame -count=1
- HTTP图片链路定向检查: passed — GOCACHE=/tmp/piguard-go-build-cache go test ./internal/handler -run TestFrames -count=1；真实HTTP/双WS/SQLite/临时文件
- 图片定向竞态检查: passed — 最新实现执行 GOCACHE=/tmp/piguard-go-build-cache go test -race ./internal/protocol ./internal/service ./internal/handler -run TestFrame -count=1；包括2MiB与4096×4096边界
- 补丁与格式: passed — 新增及修改Go文件已gofmt，git diff --check通过
- 告警与图片联合定向竞态回归: passed — GOCACHE=/tmp/piguard-go-build-cache go test -race ./internal/handler -run TestAlarmEventsHTTPAndWebSocket|TestFrames -count=1；只更新已实现图片接口的旧占位预期，不削减告警业务断言。
- 已知限制: 真实Broker、Docker、摄像头和树莓派未执行；本轮证据为实际软件组件。
- 已知限制: camera.snapshot/完整告警引用、前端、鉴权、TTL/容量清理等按已确认非目标留后续。
- 已知限制: 通知复用当前进程Hub；本轮没有跨进程WS协调或硬中断后的通知补发。

## 阻塞项

_无。_

## 风险与跳过的工作

- 真实Broker、Docker、摄像头和树莓派图片联调未执行；通过结论限定已确认的软件组件验收范围。
- 持久恢复证据为SQLite/文件目录关闭后重开，未执行真实断电或硬中断实验；硬中断可留临时/未引用文件，按规格不会暴露为有效图片且不做自动清理。
- 通知使用单进程Hub，无跨进程WS协调或硬中断后的通知补发；camera.snapshot、完整告警图片引用、前端、鉴权、TTL/容量清理仍为已确认非目标。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 0 | recovery | — | Builder handoff Runtime checks failed: go-tests, go-race | 2026-10-09T13:02:15.119Z |
| 1 | 2 | 1 | pass | — | 独立验收当前候选A1—A9全部通过。已完整核对brief/Spec、源码、软件组件测试及同候选Runtime检查/历史日志；复用全量test/race/vet有效回执，无缺失或失效检查需要补跑。未把Builder自报检查或真实环境未执行项目当成正式通过证据。 | 2026-10-09T13:18:41.503Z |



## 结论

独立验收当前候选A1—A9全部通过。已完整核对brief/Spec、源码、软件组件测试及同候选Runtime检查/历史日志；复用全量test/race/vet有效回执，无缺失或失效检查需要补跑。未把Builder自报检查或真实环境未执行项目当成正式通过证据。
