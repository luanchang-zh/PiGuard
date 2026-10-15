# P3 本地预警实施计划

状态：软件路径已完成。真实 Broker 与 Go 服务联调见 [联调报告](../../verification/integration/p5-live-stack.md)。边缘端仍不把 HTTP 查询写回 `platform_confirmed`。

契约以 [总规范](../PLAN.md) 第 5、6 节为准。本文件只写这一阶段要做的事。

## 1. 本阶段产出

- `rules/`：障碍物、车道偏离、急转弯、高温，以及四个传感器的故障状态机。
- `actuators/mock.py`：按综合风险切换模拟灯和蜂鸣器。不分配 GPIO。
- `storage/events.py`：事件先写入 SQLite，再按原报文发布。
- `protocol/event.py`：v1 事件 JSON。不带 `snapshot_id`。
- `runtime/app.py`：规则 10 Hz。断网时继续判断和模拟输出；重连后用同一个 `event_id` 再发。

## 2. 行为

- 障碍物看映射后的米，不再乘 `distance_scale`。速度大于 0 才算时距；速度为 0 不除零。距离和时距取更高一级。没有单独的持续时间。
- 车道 `abs(offset_ratio) > 0.35` 连续达到 1000 ms 才是 warning。急转弯要同时满足角速度和车速，连续达到 300 ms。高温在 35 °C 连续达到 5000 ms 后触发，降到 33 °C 连续达到 5000 ms 才解除。33 °C 和 35 °C 之间不触发，也不累计解除。
- 比较用 `>=` 持续时间。样本年龄超过 TTL 视为过期：中断 pending，不把已经激活的告警当成正常解除。过期写在事件 `data.reason`，遥测 `status` 仍然只有 ok/timeout/unavailable/error。
- 无效样本同样中断 pending，并触发一次 `sensor_failure`。恢复有效后，持续时间重新累计。
- 只在状态变化时生成事件。`recovered` 保留恢复前的 warning 或 danger。同一次重发不改 `event_id`、时间和内容。
- 综合风险取当前激活告警的最高级。四个规则都有新鲜有效依据且没有告警时为 normal。还不能判断时为 unknown。
- 模拟输出：normal 绿灯，warning 黄灯和间歇声，danger 红灯和急促声，unknown 关闭。
- 发布使用事件 QoS 1、retain false。SQLite 失败时把事件留在内存里重试，不把它当成已经发出。已平台确认且超过保留期的行才可以删除。HTTP 入库确认留到真实联调。

## 3. 验证

用 fake clock 推进阈值、持续时间和恢复。用临时 SQLite 和 fake transport 检查断网继续告警、重连后报文不变。Go 解析器接受这些事件，并拒绝非法等级、非对象 `data` 和主题不一致。不连接 Broker，不启动 Go 服务。

## 4. 本阶段不做

命令执行、执行器仲裁里的人工测试、OpenCV、真实硬件、HTTP 事件查询。
