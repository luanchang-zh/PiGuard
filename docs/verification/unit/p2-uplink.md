# P2 单元测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/unit/test_uplink.py` 与 [P2 实施计划](../../planning/implementation/p2-uplink.md)。同一命令也重跑了 P1 的单元测试。不连接 Broker，不打开摄像头，不访问 GPIO。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows，win32 |
| Python | 3.13.12 |
| pytest | 9.1.1 |
| paho-mqtt | 2.1.0，测试用替身客户端，未连接 |
| 工作目录 | `edge/` |

## 命令与结果

```text
python -m pytest tests/unit tests/contract -v --tb=short
```

43 项通过，0 项失败，耗时 34.40 秒。其中单元 42 项，契约 1 项。契约的耗时来自调用 Go 解析器，见 [P2 契约报告](../contract/p2-telemetry.md)。

## P2 行为

| 门槛 | 用例 | 结果 |
| --- | --- | --- |
| 连接前设置无 timestamp 的 offline 遗嘱，连上后发布完整 online status | `test_startup_sets_will_without_timestamp_and_publishes_online` | 通过 |
| 2 Hz 遥测，seq 递增，风险为 unknown | `test_two_hertz_telemetry_uses_latest_sample_and_keeps_sample_at` | 通过 |
| 重复上报不改写旧 sample_at | `test_republish_does_not_refresh_an_old_sample_at` | 通过 |
| 故障对象保留，数值为 null | `test_fault_object_keeps_null_values`、`test_timeout_zero_is_encoded_as_null` | 通过 |
| 重连不重置 seq，并重新订阅 commands | `test_reconnect_keeps_seq_and_resubscribes` | 通过 |
| 新进程 seq 从 0 重新开始 | `test_new_process_restarts_seq` | 通过 |
| Broker 不可达时本地采样继续 | `test_broker_outage_still_samples` | 通过 |
| 正常退出发布 offline | `test_clean_shutdown_publishes_offline` | 通过 |
| 传感器状态不变时按 10 秒心跳 | `test_status_heartbeat_while_sensors_stay_unchanged` | 通过 |
| 进程循环可以停下并发布 shutdown | `test_run_loop_stops_cleanly` | 通过 |
| paho 包装在连接前设置遗嘱 | `test_paho_wrapper_sets_will_before_connect` | 通过 |
| CLI 必须提供配置文件 | `test_cli_requires_a_config` | 通过 |

1 秒内的遥测是 2 条，QoS 0、retain false。状态与遗嘱是 QoS 1、retain true。

## 本报告未覆盖

没有连接真实 Broker，没有把报文送进正在运行的 Go 服务，也没有看 HTTP 或 WebSocket。命令到达后尚未执行。规则、事件和图片都不在这次运行里。
