# P3 契约测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/contract/test_go_baseline.py` 里新增的事件样例。解析器来自本地分支 `go后端开发` 的 `telemetry.go`、`event.go` 和 `command.go`。`frame.go` 依赖后端模块，测试只放进它使用的 `frameIdentity` 正则，不复制该文件。Go 版本是 go1.26.1。

## 命令与结果

与单元测试同一次运行：

```text
python -m pytest tests/unit tests/contract -q --tb=line
```

`test_go_parser_accepts_encoder_output_and_rejects_bad_payloads` 通过。整次运行 63 项通过，耗时 9.38 秒。P2 的遥测和状态样例仍在同一次运行里。

## 解析器接受的样例

| 样例 | 结果 |
| --- | --- |
| `obstacle_warning` / `started` / `danger`，data 含距离、车速和时距 | 接受 |
| `high_temperature` / `recovered` / `warning` | 接受 |
| `sensor_failure` / `started` / `warning`，`reason=stale`，没有 snapshot 字段 | 接受 |

## 解析器拒绝的样例

| 样例 | 结果 |
| --- | --- |
| `data` 是数组 | 拒绝 |
| `recovered` 的 level 写成 normal | 拒绝 |
| 顶层多了 `incident_id` | 拒绝 |
| 主题设备与 payload 的 device_id 不一致 | 拒绝 |

## 本报告未覆盖

命令和 Ack 还没有送进 Go 解析器。也没有通过 MQTT 把事件交给正在运行的后端，因此还没有核对“平台只存一条”。
