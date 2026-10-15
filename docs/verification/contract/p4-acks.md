# P4 契约测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/contract/test_go_baseline.py` 里新增的命令和 Ack 样例。解析器来自本地分支 `go后端开发` 的 `telemetry.go`、`event.go` 和 `command.go`。`frame.go` 依赖后端模块，测试只放进它使用的 `frameIdentity` 正则，不复制该文件。Go 版本是 go1.26.1。

## 命令与结果

与单元测试同一次运行：

```text
python -m pytest tests/unit tests/contract -q --tb=line
```

`test_go_parser_accepts_encoder_output_and_rejects_bad_payloads` 通过。整次运行 78 项通过，耗时 7.72 秒。P2 的遥测和状态样例、P3 的事件样例仍在同一次运行里。

## 解析器接受的样例

| 样例 | 结果 |
| --- | --- |
| `buzzer.test`，`duration_ms` 为 1000 | 接受 |
| `indicator.test`，黄色，500 ms | 接受 |
| `scenario.start`，`obstacle_approach`，速度 2 | 接受 |
| `scenario.stop`，空 params | 接受 |
| 成功 Ack，`result` 为蜂鸣器完成说明 | 接受 |
| 成功 Ack，`result` 含指示灯颜色 | 接受 |
| 失败 Ack，只有 `COMMAND_EXPIRED` | 接受 |

边缘编码器发出的 Ack 就是上表里的成功和失败样例。

## 解析器拒绝的样例

| 样例 | 结果 |
| --- | --- |
| `duration_ms` 为 50 | 拒绝 |
| `scenario.stop` 多了字段 | 拒绝 |
| `schema_version` 为 2 | 拒绝 |
| 成功 Ack 同时带了 `error` | 拒绝 |
| 失败 Ack 同时带了 `result` | 拒绝 |
| 成功 Ack 缺少 `executed_at` | 拒绝 |

边缘解析器对上表的命令样例同样拒绝。Go 不检查 Ack 错误码的枚举；`RESULT_UNCERTAIN` 和 `CANCELLED` 因此可以出现在失败 Ack 里。

## 本报告未覆盖

没有通过 MQTT 把命令交给正在运行的后端，因此还没有核对 HTTP 查询和双 WebSocket 上的 Ack。`camera.snapshot` 能通过 Go 的命令解析，边缘在本阶段仍回复 `UNSUPPORTED_COMMAND`。
