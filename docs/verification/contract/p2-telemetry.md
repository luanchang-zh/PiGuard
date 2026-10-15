# P2 契约测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/contract/test_go_baseline.py`。解析器来自本地分支 `go后端开发` 的 `go-backend/internal/protocol/telemetry.go`，测试时由 `git show` 读出，不复制进 `edge` 分支。Go 版本是 go1.26.1。

## 命令与结果

与单元测试同一次运行：

```text
python -m pytest tests/unit tests/contract -v --tb=short
```

`test_go_parser_accepts_encoder_output_and_rejects_bad_payloads` 通过。整次运行 43 项通过，耗时 34.40 秒。

## 解析器接受的样例

| 样例 | 结果 |
| --- | --- |
| 正常遥测：速度、距离、温度、陀螺仪、车道和 `risk.level=unknown` | 接受 |
| 距离 timeout，`raw_value` 与 `mapped_value` 为 null，对象仍在 | 接受 |
| online status：带 timestamp、软件版本、config_version 0、mode、四个传感器和 mqtt.connected | 接受 |
| 遗嘱：`online=false`，`reason=connection_lost`，没有 timestamp | 接受 |

## 解析器拒绝的样例

| 样例 | 结果 |
| --- | --- |
| 遥测缺少 seq | 拒绝 |
| 主题设备与 payload 的 device_id 不一致 | 拒绝 |
| schema_version 为 2 | 拒绝 |
| 传感器状态写成 stale | 拒绝 |
| online status 缺少 timestamp | 拒绝 |

## 本报告未覆盖

命令、Ack 和事件样例还没有送进 Go 解析器。也没有通过 MQTT 把这些报文交给正在运行的后端。
