# 树莓派边缘端接续状态

## 目标与当前状态

P1 到 P4 的软件路径已实现。四种下行命令、命令去重、Ack 和模拟执行器仲裁已有 fake clock、临时 SQLite 和 Go 命令/Ack 解析器测试。HTTP 入库确认、HTTP 命令查询、真实 Broker 联调和硬件驱动仍未做。步骤见 `docs/planning/implementation/p4-commands.md`。

当前分支 `edge` 从 `main@e975d4b51d0dd5d42b9c1f1bb1e8ae0c1f0a6113` 创建。后端分支仍为 `go后端开发@0e65a536a3fe01ae4a866bbe91b695ea25f3623b`。

## 重要文件与已确定事项

- `docs/planning/PLAN.md`：总规范。后端事实、契约差异、已定硬件、P0—P7 和验收门槛。
- `docs/planning/implementation/pc-edge-software.md`：开发电脑上的实施计划。P1 已完成。P2 软件路径见同目录 `p2-uplink.md`。
- `docs/planning/implementation/p2-uplink.md`：P2 上行实施计划。软件路径已完成。
- `docs/planning/implementation/p3-local-warnings.md`：P3 本地预警实施计划。软件路径已完成。
- `docs/planning/implementation/p4-commands.md`：P4 下行命令实施计划。软件路径已完成。
- `docs/verification/unit/p3-local-warnings.md`、`docs/verification/contract/p3-events.md`：P3 单元和契约报告。联调与硬件报告尚未产生。
- `docs/verification/unit/p4-commands.md`、`docs/verification/contract/p4-acks.md`：P4 单元和契约报告。联调与硬件报告尚未产生。
- `docs/constraints/raspberry_pi_adas_course_design.md`：课设约束。传感器型号写在概述、技术栈和第 8、11–14 节。网络规范未改，湿度、气压、加速度不上报文。
- `edge/README.md`：模块边界和入口。`edge/config/default.example.yaml` 可由 `config/loader.py` 加载；同目录 `local.yaml` 深合并覆盖，且不进 Git。
- 后端已完成telemetry/status、四类动作/Ack、events；配置/图片仍不可用。
- 命令有效期10秒，Ack执行时间为UTC；事件恢复level仍为warning/danger，三种action各用新event_id。
- PUBACK不证明事件入库；规划使用已有HTTP事件查询确认，未确认缓存不自动删除。
- 已定型号：HC-SR04 加 8 路电平转换模块、OV5647IMXZ19、GY-521 MPU6050、AHT20（同模块 BMP280 只做识别）。默认接线在 PLAN 第 1.1 节。蜂鸣器与 LED 型号未知，引脚未分配。
- 分支从main而非后端创建，本目录不含go-backend；联调使用独立后端checkout/服务。main共享规范可能较旧，实际兼容基线见PLAN。

## 下一步

下一步做 P5 车道算法：合成道路和本地录像要实际经过 ROI、灰度、模糊、Canny、Hough。真实 Broker 联调留到 Broker 与 Go 服务都可到达时再单独记录。同一 `car-001` 不同时运行 Go 模拟器。P6 等树莓派在手后按总规范顺序接入，执行器继续等待型号。

P1 里已经定下、后续不要另起一套的行为：

- 课设给出的正车道偏移记为 `right`，负偏移记为 `left`，0 为 `center`。
- `sensor_failure` 按三帧依次故障：距离 timeout、摄像头 unavailable、陀螺仪 error。温度和车速保持有效。
- 每个关键帧保持 1 秒场景时间，播完停在最后一帧。`playback_speed` 只缩放这个间隔。
- 采样从启动后的第一个周期开始，t=0 不立即采样。
- 模拟源的 `source` 都是 `mock`。配置里的 `scenario` / `synthetic` 只选择驱动，不写进样本。
- 道路距离先除以 `distance_scale` 得到 raw，处理层再乘一次。mock 不读取 `hardware`；`yaw_axis` / `yaw_sign` 由采样器从配置取出。

P3 里已经定下、后续不要另起一套的行为：

- 障碍物按映射距离立即分级，并在速度大于 0 时与时距取更高一级。车道、急转弯、高温才使用持续时间。33 °C 到 35 °C 之间不触发高温，也不累计解除。
- 过期样本中断 pending，事件 `data.reason` 写 `stale`。遥测 status 仍然只有 ok/timeout/unavailable/error。
- 事件先写入 SQLite。HTTP 入库确认还没有做，所以重连按原 payload 再发，未确认的行不删除。

P4 里已经定下、后续不要另起一套的行为：

- 只执行四种命令。`camera.snapshot` 在边缘返回 `UNSUPPORTED_COMMAND`。
- 先保存 `command_id` 再驱动输出。已完成的 ID 重发同一份 Ack，`executed_at` 不变。相同 ID、不同内容不执行，已保存的 Ack 保持不变。
- 启动时遇到 `running` 记录，Ack 为 `RESULT_UNCERTAIN`，不自动重做。
- warning 和 danger 优先于人工测试。测试期间采样和规则继续。
- hardware 模式不执行场景切换，也不启动灯或蜂鸣器测试。
- Ack 使用 QoS 1、retain false。成功只有 `result`，失败只有 `error`。

## 验证

2026-10-12 在 Python 3.13.12、pytest 9.1.1、go1.26.1 下执行 `python -m pytest tests/unit tests/contract -q --tb=line`，78 项通过，耗时 7.72 秒。记录见 `docs/verification/unit/p4-commands.md` 与 `docs/verification/contract/p4-acks.md`。未连接 Broker，未启动 Go 服务，未做 HTTP 事件确认或命令查询，未上电，未使用树莓派、GPIO 或摄像头。
