# 树莓派边缘端接续状态

## 目标与当前状态

P1 到 P5 的软件路径已实现，并已与真实 Mosquitto、独立 Go 服务、HTTP 和两个 WebSocket 客户端联调。边缘端没有 HTTP 入库确认客户端。OV5647、GPIO 和蜂鸣器/LED 引脚仍未做。P5 步骤见 `docs/planning/implementation/p5-lane.md`。

当前分支 `edge` 从 `main@e975d4b51d0dd5d42b9c1f1bb1e8ae0c1f0a6113` 创建。联调使用独立工作副本 `E:\StudyDir\PiGuard-go`，后端提交 `go后端开发@7b39493177fca1586279707a357b6f906694053e`。不要把 go-backend 拷进 `edge`。

## 重要文件与已确定事项

- `docs/planning/PLAN.md`：总规范。后端事实、契约差异、已定硬件、P0—P7 和验收门槛。
- `docs/planning/implementation/pc-edge-software.md`：开发电脑上的实施计划。P1 已完成。P2 软件路径见同目录 `p2-uplink.md`。
- `docs/planning/implementation/p2-uplink.md`：P2 上行实施计划。软件路径已完成。
- `docs/planning/implementation/p3-local-warnings.md`：P3 本地预警实施计划。软件路径已完成。
- `docs/planning/implementation/p4-commands.md`：P4 下行命令实施计划。软件路径已完成。
- `docs/planning/implementation/p5-lane.md`：P5 车道检测实施计划。合成道路和本地录像已完成。
- `docs/verification/unit/p5-lane.md`：P5 单元报告。
- `docs/verification/integration/p5-live-stack.md`：真实 Broker、HTTP 和双 WebSocket 报告。硬件报告尚未产生。
- `docs/constraints/raspberry_pi_adas_course_design.md`：课设约束。传感器型号写在概述、技术栈和第 8、11–14 节。网络规范未改，湿度、气压、加速度不上报文。
- `edge/README.md`：模块边界和入口。`edge/config/default.example.yaml` 可由 `config/loader.py` 加载；同目录 `local.yaml` 深合并覆盖，且不进 Git。
- 后端已完成telemetry/status、四类动作/Ack、events；配置/图片仍不可用。
- 命令有效期10秒，Ack执行时间为UTC；事件恢复level仍为warning/danger，三种action各用新event_id。
- PUBACK不证明事件入库；规划使用已有HTTP事件查询确认，未确认缓存不自动删除。
- 已定型号：HC-SR04 加 8 路电平转换模块、OV5647IMXZ19、GY-521 MPU6050、AHT20（同模块 BMP280 只做识别）。默认接线在 PLAN 第 1.1 节。蜂鸣器与 LED 型号未知，引脚未分配。
- 分支从main而非后端创建，本目录不含go-backend；联调使用独立后端checkout/服务。main共享规范可能较旧，实际兼容基线见PLAN。

## 下一步

下一步等树莓派在手后做 P6。按总规范第 6 节一次换一路：先确认电平，再 AHT20、MPU6050、HC-SR04，最后 OV5647。实拍进入已经写好的 `inspect_lane`，不另写车道算法。蜂鸣器和 LED 继续等型号，不分配引脚。同一 `car-001` 不同时运行 Go 模拟器。

重跑联调前先启动 Compose 项目 `piguard-edge-it`。这台 Windows 上的 Docker Desktop 会把宿主机文件挂载变成空目录，Broker 配置要先放进命名卷。后端镜像构建时 `proxy.golang.org` 会 unexpected EOF，需要 `GOPROXY=https://goproxy.cn,direct`。测试用系统时钟，不要用 FakeClock 对真实命令有效期。默认 pytest 不收集 `edge/tests/integration`。

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

P5 里已经定下、后续不要另起一套的行为：

- 场景只决定白线画在哪里。`LaneResult` 来自 `inspect_lane`，不抄 `scene.lane_offset`。
- 左右线缺一则无效。偏移绝对值小于 0.01 记为 0。正值向右，负值向左。
- 摄像头故障返回空图像，不调用检测。录像结束后释放采集。
- `capture_fps` 仍是 10，每次读取都做检测。配置里的 `process_fps: 5` 还没有单独节流。

## 验证

2026-10-12 在 Python 3.13.12、pytest 9.1.1、OpenCV 4.11.0 下执行 `python -m pytest tests/unit tests/contract -q --tb=line`，83 项通过，耗时 11.08 秒。记录见 `docs/verification/unit/p5-lane.md`。

同一天对 Compose 项目 `piguard-edge-it` 执行 `python -m pytest tests/integration/test_live_stack.py -q --tb=short`，1 项通过，耗时 15.87 秒。设备上线，居中车道进入平台状态，障碍物从 warning 升到 danger，重复命令被平台记为 duplicate，Broker 重启后事件不新增，正常停止后离线。记录见 `docs/verification/integration/p5-live-stack.md`。栈已停止。未上电，未使用树莓派、GPIO 或 OV5647。
