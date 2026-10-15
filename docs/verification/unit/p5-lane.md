# P5 单元测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/unit/test_lane.py` 与 [P5 实施计划](../../planning/implementation/p5-lane.md)。同一命令也重跑了 P1 到 P4 的单元测试和契约测试。这次命令不连接 Broker，不访问 GPIO。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows，win32 |
| Python | 3.13.12 |
| pytest | 9.1.1 |
| OpenCV | opencv-python-headless 4.11.0 |
| 工作目录 | `edge/` |

## 命令与结果

```text
python -m pytest tests/unit tests/contract -q --tb=line
```

83 项通过，0 项失败，耗时 11.08 秒。其中单元 82 项，契约 1 项。P5 新增 5 项单元测试。P4 当时的 78 项记录仍留在 [P4 单元报告](p4-commands.md)，这次运行包含那些测试。

## P5 行为

| 门槛 | 结果 |
| --- | --- |
| 居中道路偏移约为 0、方向 center；0.1 到 0.5 和 -0.4 保持符号，误差在 0.02 内 | 通过 |
| ROI 为 240×640，灰度、模糊、Canny 和至少两条 Hough 线段都实际生成 | 通过 |
| 缺一条线或空白画面时车道无效，偏移和置信度为 null | 通过 |
| 合成摄像头使用检测结果；场景写着 0 时，替换后的检测仍返回 -0.2 / left | 通过 |
| 本地 MJPG 录像先读到居中，再读到约 0.4 / right；结束后状态 unavailable，释放可以重复调用 | 通过 |

## 本报告未覆盖

OV5647 实拍不在这次运行里。真实 Broker 上的车道报文见 [联调报告](../integration/p5-live-stack.md)。
