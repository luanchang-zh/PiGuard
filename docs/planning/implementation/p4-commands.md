# P4 下行命令实施计划

状态：软件路径已完成。真实 Broker、HTTP 查询和双 WebSocket 见 [联调报告](../../verification/integration/p5-live-stack.md)。

契约以 [总规范](../PLAN.md) 第 5 节为准。本文件只写这一阶段要做的事。

## 1. 本阶段产出

- `protocol/command.py`：校验入站命令，编码成功或失败 Ack。
- `storage/commands.py`：命令 ID、内容指纹、状态和 Ack 原文。
- `commands/handler.py`：受理、去重、有效期和四种动作。
- `actuators/mock.py`：自动告警优先于人工测试。
- `runtime/app.py`：断网时仍处理命令；连接后发布未发出的 Ack。

## 2. 行为

- 只执行 `buzzer.test`、`indicator.test`、`scenario.start`、`scenario.stop`。`camera.snapshot` 和其他类型返回 `UNSUPPORTED_COMMAND`。
- 先把 `command_id` 写入 SQLite，再改变场景或模拟输出。已完成的 ID 只重发同一份 Ack，`executed_at` 不变。相同 ID、不同内容时不执行，已保存的 Ack 保持不变。
- 启动时若记录仍是 `running`，关闭残留测试输出，Ack 为 `RESULT_UNCERTAIN`，不自动重做。断电窗口里不能声称恰好执行一次。
- 已经处于 warning 或 danger 时，测试命令返回 `DEVICE_BUSY`，输出保持自动告警。测试进行中若告警开始，立即停止测试并返回 `DEVICE_BUSY`。另一个测试还在进行时也返回 `DEVICE_BUSY`。
- 测试最长 5 秒。采样和规则在测试期间继续。`scenario.start` / `scenario.stop` 可以在告警期间切换场景，因为它们不占用灯和蜂鸣器。
- 命令已经过期，或持续时间长于剩余有效期时，返回 `COMMAND_EXPIRED`，不启动输出。持续时间刚好等于剩余时间时可以开始。
- `mode` 不是 mock 时，场景命令返回 `DEVICE_NOT_SUPPORTED`，灯和蜂鸣器测试返回 `ACTUATOR_UNAVAILABLE`。
- 正常关闭时，正在进行的测试返回 `CANCELLED`。
- Ack 发到 `car/{device_id}/command-acks`，QoS 1，retain false。成功只有 `result`，失败只有 `error`。SQLite 写入失败时不启动输出，下一次步进再试。

## 3. 验证

用 fake clock、fake transport 和临时 SQLite 覆盖四种动作、重复、冲突、过期、非法参数、硬件模式、断网后补发、关闭取消，以及启动时的不确定结果。Go 解析器接受这些 Ack，并拒绝成功里夹带 error、失败里夹带 result、缺少 `executed_at` 的报文。不连接 Broker，不启动 Go 服务，不做 HTTP 查询。

## 4. 本阶段不做

真实 Broker、HTTP 命令查询、双 WebSocket、`camera.snapshot`、远程配置、GPIO 引脚和真实灯/蜂鸣器驱动。
