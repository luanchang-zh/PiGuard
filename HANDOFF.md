# 树莓派边缘端接续状态

## 2026-10-27 前后端迁移腾讯云

前端和 Go 后端已部署到 `ubuntu@106.54.188.216`（Ubuntu 22.04），网页入口为 `http://106.54.188.216/`。Nginx 按 IP 单独分流，后端监听 `127.0.0.1:18080`；专用 Mosquitto 的 TCP / WebSocket 监听为 `127.0.0.1:1883` / `127.0.0.1:1884`，设备通过公网 `ws://106.54.188.216/mqtt` 接入。直接公网 1883 的验证请求未到达服务器，因此使用已连通的 80 端口。原有域名站点和 Docker 服务保留。

服务为 `piguard-backend.service`、`piguard-mqtt.service`，均开启自启和失败重启。数据库与图片在 `/var/lib/piguard/`，Broker 数据在 `/var/lib/piguard-mqtt/`，私密环境变量在 `/etc/piguard/backend.env`。可重复部署入口为 `deploy/cloud/deploy.sh`，公网检查为 `deploy/cloud/verify.py`。

平台、设备 HTTP、设备 MQTT 等身份均生成独立随机密码，保存在被 Git 忽略的 `deploy/cloud/.secrets/`。登录信息为该目录中的 `access.txt`；已生成的树莓派配置为被忽略的 `edge/config/cloud/raspberrypi.yaml`，使用 `edge/scripts/run-cloud.sh` 启动。摄像头仍连接 USB 局域网 `10.12.194.2:8090`。本次用户明确树莓派未连接；云端部署和软件验证独立完成，真实设备等待以后接入。旧局域网演示配置保持原用途。

当前前端构建、Go test/vet、Python 178 项单元测试，以及公网登录/Cookie/CSRF/Origin/WebSocket/MQTT WebSocket 检查通过。具体访问和维护步骤见 [云服务器部署](docs/deployment/cloud-server.md)。

临时软件设备进一步通过云端遥测、默认配置确认、拍照上传与 Ack、模拟蜂鸣器 Ack、WS 业务推送、正常离线及后端重启持久化检查。软件设备已停止，验证产生的数据与图片归档在 `/var/backups/piguard/smoke-results-*/`，线上恢复验证前的干净初始数据库。

## 2026-10-22 真实输入与 USB 链路开发

最新代码已接入 HC-SR04（BCM23/24）、MPU 系列陀螺仪（当前识别为 MPU6500，0x68）、AHT20（0x38）、物理 Pin11 / BCM17 蜂鸣器，以及电脑摄像头 HTTP JPEG 输入。选择 `edge/config/raspberrypi-connected-demo.yaml`，设备模式为 hardware；速度保留场景模拟并标记 mock，距离保留桌面 ×100 映射。湿度在 AHT20 驱动中解码，当前页面/遥测仍按课设 v1 使用温度。LED 未接。

USB 电脑地址 `10.12.194.2/28`，树莓派 `10.12.194.1/28`。已定位官方 ICS 服务每约 14 秒切到 DHCP、临时撤掉地址导致 HTTP 超时；停用角色轮换后，实机 600 次 HTTP 请求全成功，最大约 21.4 ms。此前还处理过 ECM 初始化卡住及 Mihomo TUN 拦截。配置步骤已做成 `configure-usb-pi.sh` / `configure-usb-host.sh`，详见 edge/README.md。电脑摄像头由 `serve-pc-camera.py` 提供，示例 token 和平台测试账号一并纳入 Git。

用户已经确认网页蜂鸣器测试听到声音。原 CSI 摄像头排查显示模块接入时 SCL 被拉低，空排线对照恢复，当前采用电脑摄像头方案。用户现已拔下树莓派并要求只完成开发，因此新增传感器/视频组合使用离线替身验证，未继续连接设备。电脑摄像头采集测试进程也已停止。设备上的旧配置与进程状态以重新连接后为准；最新接入代码仍在本机工作区。

下方均为历史阶段记录；旧的“没有硬件驱动”“硬件模式不支持蜂鸣器”“本目录不含后端”描述由本节和最新 README 替代。

## 2026-10-20 摄像头改为 USB 连接的 Ubuntu 电脑

用户确认树莓派为 4B，另一台摄像头电脑为 Ubuntu 24.04。最新远程 main 已快进整合至本地 `9bdc88e`。新增 Ubuntu `python -m piguard_edge.pc_camera` 服务与 Pi `remote` 摄像头驱动，使用 USB Ethernet 的 JPEG/Token 接口；`raspberrypi-demo.yaml` 默认取 `192.168.7.1:8765`，原有默认示例仍用合成道路。两端需要相同的 `PIGUARD_CAMERA_TOKEN`。部署规划、供电、USB-C 网卡配置、安装和硬件验收见 [USB 摄像头部署](docs/deployment/usb-pc-camera.md)。当前未连接目标 Ubuntu 或 Pi，尚未执行目标硬件部署。

本地边缘端单元与 Go 协议契约回归 130 项通过，新增摄像头测试 25 项，未调用这台电脑的摄像头。验证范围见 [软件测试记录](docs/verification/unit/usb-pc-camera.md)。

## 2026-10-19 重刷后的运行环境

树莓派 `zhangzhuhao@raspberrypi.local` 已重新配置：Debian 13 arm64、Python 3.13.5，项目位于 `/home/zhangzhuhao/PiGuard`，虚拟环境位于 `/home/zhangzhuhao/.venvs/piguard-edge`。项目依赖和 Picamera2、libcamera、SMBus、GPIO 库均可导入；I²C 已启用，`/dev/i2c-1` 可由当前用户访问。启动配置备份和验证日志在树莓派的 `~/piguard-setup-20261019/`。

按用户要求，固定测试账号写入版本控制：`go-backend/.env.demo` 与 `edge/config/raspberrypi-demo.yaml`。本机执行 `./go-backend/scripts/run-demo.sh`，树莓派执行 `./edge/scripts/run-demo.sh` 或 `~/bin/piguard-edge`。前端继续在本机运行。电脑当前地址为 `192.168.43.121`。

本轮硬件未连接，使用 mock 传感器和合成道路。树莓派测试 104 项通过、1 项因未安装 Go 跳过；实际 MQTT/HTTP 遥测、历史、预览 JPEG、软件拍照 Ack、模拟蜂鸣器 Ack、远程配置成功确认和正常离线均通过。测试实例已停止，后续按上述脚本启动。

## 2026-10-15 联调修复更新

当前 checkout 包含前端、Go 后端和 Python 边缘端。本次补齐远程规则配置、设备 HTTP 事件确认、软件图片拍照/预览、MQTT 发送确认、首次连接重试、摄像头检测节流和 SIGTERM 退出，并修复前端 WS 代理、空预览弹错、轮询清理和在线显示。配置通过被忽略的 local.yaml 显式启用，见 edge/README.md。真实硬件驱动仍未实现。

树莓派已由用户拔下；修复后以本机 mock 边缘端和真实认证 Broker/Go 验证，没有把新代码部署到树莓派。新结果见 docs/verification/integration/p5-platform-features.md。以下内容保留为较早阶段记录；其中“HTTP 确认未做、拍照不支持、配置/图片不可用、process_fps 未节流”已被本次更新替代。

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
