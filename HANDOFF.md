# 树莓派边缘端接续状态

## 目标与当前状态

P1 本地数据链已实现：模型、五类 mock 源、六个场景、YAML 加载和 LatestState。规则、MQTT、CLI 和硬件驱动仍未实现。实施步骤见 `docs/planning/implementation/pc-edge-software.md`。

当前分支 `edge` 从 `main@e975d4b51d0dd5d42b9c1f1bb1e8ae0c1f0a6113` 创建。后端分支仍为 `go后端开发@0e65a536a3fe01ae4a866bbe91b695ea25f3623b`。

## 重要文件与已确定事项

- `docs/planning/PLAN.md`：总规范。后端事实、契约差异、已定硬件、P0—P7 和验收门槛。
- `docs/planning/implementation/pc-edge-software.md`：开发电脑上的实施计划。P1 已完成。P2 起另写同目录文件。
- `docs/verification/unit/p1-local-data-chain.md`：P1 单元测试报告。契约、联调和硬件报告尚未产生。
- `docs/constraints/raspberry_pi_adas_course_design.md`：课设约束。传感器型号写在概述、技术栈和第 8、11–14 节。网络规范未改，湿度、气压、加速度不上报文。
- `edge/README.md`：模块边界和入口。`edge/config/default.example.yaml` 可由 `config/loader.py` 加载；同目录 `local.yaml` 深合并覆盖，且不进 Git。
- 后端已完成telemetry/status、四类动作/Ack、events；配置/图片仍不可用。
- 命令有效期10秒，Ack执行时间为UTC；事件恢复level仍为warning/danger，三种action各用新event_id。
- PUBACK不证明事件入库；规划使用已有HTTP事件查询确认，未确认缓存不自动删除。
- 已定型号：HC-SR04 加 8 路电平转换模块、OV5647IMXZ19、GY-521 MPU6050、AHT20（同模块 BMP280 只做识别）。默认接线在 PLAN 第 1.1 节。蜂鸣器与 LED 型号未知，引脚未分配。
- 分支从main而非后端创建，本目录不含go-backend；联调使用独立后端checkout/服务。main共享规范可能较旧，实际兼容基线见PLAN。

## 下一步

另写 P2 实施文件，再实现遥测/状态编解码、MQTT 和进程入口。规则未完成时 `risk=unknown`。同一 `car-001` 不同时运行 Go 模拟器。P6 等树莓派在手后按总规范顺序接入，执行器继续等待型号。

P1 里已经定下、后续不要另起一套的行为：

- 课设给出的正车道偏移记为 `right`，负偏移记为 `left`，0 为 `center`。
- `sensor_failure` 按三帧依次故障：距离 timeout、摄像头 unavailable、陀螺仪 error。温度和车速保持有效。
- 每个关键帧保持 1 秒场景时间，播完停在最后一帧。`playback_speed` 只缩放这个间隔。
- 采样从启动后的第一个周期开始，t=0 不立即采样。
- 模拟源的 `source` 都是 `mock`。配置里的 `scenario` / `synthetic` 只选择驱动，不写进样本。
- 道路距离先除以 `distance_scale` 得到 raw，处理层再乘一次。mock 不读取 `hardware`；`yaw_axis` / `yaw_sign` 由采样器从配置取出。

## 验证

2026-10-12 在 Python 3.13.12、pytest 9.1.1 下执行 `python -m pytest tests/unit -v`，29 项通过；`PYTHONPATH=src` 导入 `piguard_edge` 得到 `0.0.0`。记录见 `docs/verification/unit/p1-local-data-chain.md`。未连接 Broker，未运行 Go 测试，未上电，未使用树莓派、GPIO 或摄像头。
