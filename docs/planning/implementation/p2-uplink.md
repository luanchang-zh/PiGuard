# P2 上行实施计划

状态：软件路径已完成。真实 Broker 与 Go 服务联调未做。

契约以 [总规范](../PLAN.md) 第 5 节和 `go后端开发` 的 `ParseTelemetry` / `ParseStatus` 为准。本文件只写这一阶段要做的事。

## 1. 本阶段产出

- `protocol/telemetry.py`、`protocol/status.py`：把 `LatestState` 编成 v1 JSON。规则未完成时 `risk.level=unknown`，`active_events` 为空数组。
- `transport/mqtt.py`：发布、订阅、遗嘱。单元测试用 fake transport。paho-mqtt 2.1.0 只在真实连接时使用，回调 API 用 VERSION2。
- `runtime/app.py` 和 `python -m piguard_edge`：采样继续跑，连接失败也启动。遥测 2 Hz，只发最新样本，不补传。
- 连接前设置无 `timestamp` 的 offline 遗嘱。连上和重连后重新订阅 `commands`，并发布完整 online status。正常退出发布 offline。命令内容留到 P4，本阶段不执行。

## 2. 行为

- 故障对象保留，数值为 null，状态仍是 ok/timeout/unavailable/error。不发明 `stale`。
- `sample_at` 来自采样时刻。再次上报同一条样本不改成发布时间。超过 TTL 也照原样发送，由平台计算新鲜度。
- `seq` 从 0 起按已发布的遥测递增。同一次进程内重连不归零。新进程重新从 0 开始。
- 在线状态在连接时发布，传感器状态变化时再发布，另外每 10 秒心跳一次。
- 遥测 QoS 0、retain false。状态和遗嘱 QoS 1、retain true。

## 3. 验证

先用 fake clock 和 fake transport 跑单元测试。再用本机 Go 解析器接受编码器产出的样例，并拒绝缺字段、错误枚举和主题不一致的样例。不连接真实 Broker，不运行 Go 服务。

## 4. 本阶段不做

规则、事件入库、命令执行、图片、远程配置、真实硬件。
