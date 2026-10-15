# P4 单元测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/unit/test_commands.py` 与 [P4 实施计划](../../planning/implementation/p4-commands.md)。同一命令也重跑了 P1、P2、P3 的单元测试和契约测试。不连接 Broker，不打开摄像头，不访问 GPIO。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows，win32 |
| Python | 3.13.12 |
| pytest | 9.1.1 |
| SQLite | Python 标准库 `sqlite3`，测试使用 `:memory:` 或临时文件 |
| 工作目录 | `edge/` |

## 命令与结果

```text
python -m pytest tests/unit tests/contract -q --tb=line
```

78 项通过，0 项失败，耗时 7.72 秒。其中单元 77 项，契约 1 项。P4 新增 15 项单元测试。契约见 [P4 契约报告](../contract/p4-acks.md)。

## P4 行为

| 门槛 | 结果 |
| --- | --- |
| 蜂鸣器测试按 `duration_ms` 结束后返回成功，期间遥测继续 | 通过 |
| 另一个测试还在进行时返回 `DEVICE_BUSY`，不替换当前测试 | 通过 |
| 指示灯测试使用请求的颜色，结束后回到 normal 的绿灯 | 通过 |
| 已经是 danger 时不启动测试，模拟输出保持红灯和急促声 | 通过 |
| 测试进行中切入 `obstacle_approach`，15 m 告警抢占测试并返回 `DEVICE_BUSY` | 通过 |
| `scenario.start` 从下一次采样生效；2 倍速把 1 秒单调时间变成 2 秒场景时间 | 通过 |
| `scenario.stop` 把下一次采样恢复为 `normal_drive`，播放速度为 1 | 通过 |
| 已过期，或持续时间长于剩余有效期时不启动；等于剩余时间时可以开始 | 通过 |
| 相同命令只执行一次，再次送达时重发同一份 Ack | 通过 |
| 相同 ID、不同内容不切换场景，已保存的 Ack 不变 | 通过 |
| 非法参数和 `camera.snapshot` 失败且不产生输出；空 ID 或设备不一致时不写 Ack | 通过 |
| 启动时发现 `running` 记录，标记 `RESULT_UNCERTAIN`，再次送达也不重做 | 通过 |
| hardware 模式下场景命令为 `DEVICE_NOT_SUPPORTED`，执行器测试为 `ACTUATOR_UNAVAILABLE` | 通过 |
| SQLite 插入失败时不启动输出，下一次处理再写入并开始 | 通过 |
| 断网时 Ack 先落库，连接后按原报文发布，QoS 1、retain false | 通过 |
| 正常关闭取消正在进行的测试，Ack 为 `CANCELLED` | 通过 |
| `from_config` 在同一个 SQLite 文件里建立 events 和 commands 两张表 | 通过 |

## 本报告未覆盖

没有连接真实 Broker，没有把 Ack 送进正在运行的 Go 服务，也没有做 HTTP 查询或双 WebSocket。OpenCV 和真实执行器都不在这次运行里。
