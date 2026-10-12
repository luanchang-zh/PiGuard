# P3 单元测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/unit/test_rules.py`、`edge/tests/unit/test_events.py` 与 [P3 实施计划](../../planning/implementation/p3-local-warnings.md)。同一命令也重跑了 P1、P2 的单元测试和契约测试。不连接 Broker，不打开摄像头，不访问 GPIO。

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

63 项通过，0 项失败，耗时 9.38 秒。其中单元 62 项，契约 1 项。P3 新增 20 项单元测试。契约见 [P3 契约报告](../contract/p3-events.md)。

## P3 行为

| 门槛 | 结果 |
| --- | --- |
| 障碍物按映射距离分级，速度为 0 时不算时距；时距可以把等级抬高 | 通过 |
| 距离故障不把已有危险解除，也不把故障值当成 0 | 通过 |
| 车道偏移超过 0.35 并持续 1000 ms 才告警，负偏移记为 left；无效车道不判正常，也不解除 | 通过 |
| 过期样本中断 pending，并只发一次 `sensor_failure`，`reason=stale` | 通过 |
| 急转弯同时看角速度和车速，低速、等于阈值和中途抖动都不告警 | 通过 |
| 高温 35 °C 持续 5 秒触发，34 °C 不解除，33 °C 持续 5 秒才恢复 | 通过 |
| 多个告警同时存在时，风险取 danger | 通过 |
| `obstacle_approach` 只在 warning 和 danger 两次变化时发事件，模拟输出到红灯和急促声 | 通过 |
| `sensor_failure` 按距离、摄像头、陀螺仪依次开始和恢复 | 通过 |
| 断网时规则和模拟输出继续，SQLite 留下两行；重连后同一 payload 再发，行数不变 | 通过 |
| SQLite 写入失败时事件留在内存里，下一次步进再写入并发布 | 通过 |
| 只删除已平台确认且早于保留点的行；启动时同样遵守 | 通过 |

正常行驶在温度样本到达后，遥测 `risk.level` 为 normal。温度尚未采样时仍是 unknown。

## 本报告未覆盖

没有连接真实 Broker，没有把事件送进正在运行的 Go 服务，也没有做 HTTP 入库确认。命令、OpenCV 和真实执行器都不在这次运行里。
