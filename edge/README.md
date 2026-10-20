# PiGuard 树莓派边缘端

Python边缘工程，开发分支为 `edge`。总规范见[开发规划](../docs/planning/PLAN.md)，当前实施步骤见[开发电脑上的实施计划](../docs/planning/implementation/pc-edge-software.md)，接续状态见[HANDOFF](../HANDOFF.md)。课设与网络规范在 `docs/constraints/`。

P1 已实现本地模拟数据链。P2 已实现遥测/状态编码、MQTT 会话和 `python -m piguard_edge`。P3 已实现四条规则、模拟执行器和事件缓存。P4 已实现四种下行命令、Ack 和模拟执行器仲裁。P5 已实现合成道路和本地录像的车道检测；当前也支持 USB 网络连接的 Ubuntu 电脑摄像头。其他传感器和执行器的硬件驱动尚未实现。测试报告见 [P1](../docs/verification/unit/p1-local-data-chain.md)、[P2](../docs/verification/unit/p2-uplink.md)、[P3](../docs/verification/unit/p3-local-warnings.md)、[P4](../docs/verification/unit/p4-commands.md)、[P5](../docs/verification/unit/p5-lane.md) 与 [联调](../docs/verification/integration/p5-live-stack.md)。

## 模块职责

| 目录 | 职责 |
| --- | --- |
| src/piguard_edge/config | 配置加载、校验和不可变快照 |
| models | 内部Sample、Frame、LaneResult和状态，保留采样质量与时钟 |
| protocol | MQTT v1消息编解码，与内部模型分开 |
| drivers | 按数据源分目录；mock/video/hardware通过相同接口 |
| simulation | 六个一致、可重复的场景及播放倍率 |
| processing | 滤波、比例映射、角速度与OpenCV车道结果 |
| rules | 本地规则状态机、事件生成和综合风险 |
| actuators | 模拟/真实蜂鸣器与LED、自动预警优先级 |
| commands | 命令校验、执行调度、幂等与完整Ack |
| transport | MQTT连接/重连/遗嘱、HTTP确认和后续图片上传 |
| storage | 独立edge.db、命令结果、事件缓存和配置记录 |
| runtime | 采集worker、LatestState、启动与关闭资源 |
| tests | unit/contract/integration/hardware分层证据 |
| assets、data | 本地素材、运行数据库/图片/日志 |
| deploy/systemd、scripts | 后续设备服务、联调与诊断入口 |

## 工程约定

- 实机型号为 HC-SR04（配 8 路电平转换模块）、CSI 传感器 OV5647IMXZ19、GY-521 MPU6050、AHT20+BMP280 模块中的 AHT20。默认配置仍是 mock。接线、地址和上板顺序见开发规划第 1.1 节与 P6。
- Python目标版本≥3.11。pyproject仅描述可安装包，运行时依赖将在对应阶段锁定；Picamera2/GPIO等硬件依赖不能阻止mock启动。
- 配置文件相对路径按配置文件目录解析。示例的 `../data/edge.db` 指向edge/data。
- 本地覆盖使用config/local.yaml或环境变量，真实凭据、数据库、大视频和截图不进Git。
- 线上schema_version=1，时间为UTC；持续时间用单调时钟；读取故障保留状态和null，不能补0。
- edge与Go使用不同SQLite文件。后端运行在另一checkout/主机，通过MQTT和HTTP联调。
- 同一device_id只能有一个活跃边缘实例。新实例上线前关闭该设备的Go模拟器。

## 树莓派演示（USB 电脑摄像头）

`config/raspberrypi-demo.yaml` 是纳入 Git 的课设测试配置，默认设备 `car-001`，使用 mock 传感器、USB 连接的 Ubuntu 电脑摄像头、远程配置和图片上传。摄像头已替换损坏的 CSI 模块，两端设置和 token 加载见 [USB 摄像头部署](../docs/deployment/usb-pc-camera.md)。MQTT / HTTP 设备密码分别为 `piguard-device-mqtt-demo`、`piguard-device-http-demo`，与 [平台演示配置](../go-backend/.env.demo) 一致。

在 Raspberry Pi OS / Debian 13 的仓库根目录安装：

```sh
sudo apt-get update
sudo apt-get install -y git python3-venv python3-pip
python3 -m venv --system-site-packages "$HOME/.venvs/piguard-edge"
"$HOME/.venvs/piguard-edge/bin/python" -m pip install -e './edge[dev]'
```

USB 电脑摄像头路径不依赖 Picamera2、libcamera 或 GPIO 库。已有硬件库可继续通过 `--system-site-packages` 使用。

本机平台运行 [演示启动脚本](../go-backend/scripts/run-demo.sh)，树莓派运行：

```sh
set -a
. "$HOME/.config/piguard/camera.env"
set +a
./edge/scripts/run-demo.sh
```

配置中的电脑地址为 `192.168.43.121`；网络变化后同步修改 `mqtt.host` 和 `http.base_url`。默认环境目录可通过 `PIGUARD_EDGE_PYTHON` 指定另一个 Python。相邻的 `config/local.yaml` 仍按加载器规则覆盖配置，运行这套演示时应检查是否有旧覆盖。

摄像头电脑地址独立为 `192.168.7.1:8765`。需要纯软件合成道路时，使用 `python -m piguard_edge --config edge/config/default.example.yaml`；该默认示例不要求摄像头 token。

目录骨架可在edge工作目录执行导入检查（并非服务启动）：

```sh
PYTHONPATH=src python -c 'import piguard_edge; print(piguard_edge.__version__)'
```

接入后端时使用电脑的局域网IP，而不是树莓派自身的127.0.0.1。Go直接运行的默认HTTP只监听127.0.0.1，需要后端配置为可达监听地址；该配置在后端分支/运行环境处理。

## 2026-10-15 软件联调修复

已补齐设备 HTTP 事件确认、远程规则配置、软件帧拍照/预览上传、启动连接重试和 SIGTERM 停止。实机传感器和 GPIO 驱动仍未实现；mock 图片来自合成道路或本地录像。以上更新替代本文早期阶段对“配置、图片、HTTP 确认未实现”的描述。

在被 Git 忽略的 `config/local.yaml` 中配置：

```yaml
mqtt:
  host: 电脑局域网地址
  username: car-001
  password: 设备MQTT密码
http:
  base_url: http://电脑局域网地址:8080/api/v1
  username: car-001
  password: 设备HTTP密码
features:
  remote_config: true
  frame_upload: true
```

HTTP 密码与 MQTT 密码独立，设备上只使用设备身份。HTTP 身份启用后台事件确认；图片和配置默认仍关闭，避免旧部署未经配置自动启用。启动：

```sh
python -m piguard_edge --config config/default.example.yaml
```

- QoS 1 发布确认后才标记事件/命令 Ack 已发送。网络回调只入队；等待确认不会阻塞采样。首次 Broker 不可达也继续采样，并由后台网络线程重试。
- 事件确认每 5 秒检查一批，最多 64 条，轮转处理积压。后端仅确认设备自己的、与持久记录内容完全相同的事件；未确认记录保留。默认保留已确认记录 7 天，不等于确认后立即删除。
- 配置完整校验并持久化后更新规则和报告版本，再发送 config-acks。相同版本相同规则重发原 Ack，冲突保留首个结果，旧版本不回退。重启恢复最后成功规则；更新阈值保留活跃告警，使恢复事件仍可正常产生。平台尚未定义的时距和高温恢复持续时间继续使用本地值。
- `camera.snapshot` 从当前有效帧编码 JPEG，在后台上传，成功才用平台返回的 snapshot_id 回复。重试复用 frame_id、时间和图片字节，最多 3 次；认证/参数错误不重试。同一命令不重复拍照，未完成命令重启后返回 RESULT_UNCERTAIN。无有效帧明确失败。启用图片后每 2 秒上传一张 preview；mock 不冒充真实摄像头。
- 采集仍按 capture_fps，检测按 process_fps；复用检测结果时保留其原采样时间。SIGINT/SIGTERM 退出会停止输出、发送离线并释放录像资源。

无需树莓派的真实 Broker/Go 集成回归：先构建后端，并指定本机 Mosquitto（同安装布局包含 mosquitto_passwd）。测试使用随机 loopback 端口、临时凭据/数据库，只清理自身启动的进程。

```sh
(cd ../go-backend && go build -o /tmp/piguard-backend ./cmd/server)
PIGUARD_TEST_BACKEND_BIN=/tmp/piguard-backend \
PIGUARD_TEST_MOSQUITTO_BIN=/usr/sbin/mosquitto \
python -m pytest tests/integration/test_platform_features.py -q
```

验证范围与限制见 [本次记录](../docs/verification/integration/p5-platform-features.md)。
