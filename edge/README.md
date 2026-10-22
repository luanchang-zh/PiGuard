# PiGuard 树莓派边缘端

Python边缘工程，开发分支为 `edge`。总规范见[开发规划](../docs/planning/PLAN.md)，当前实施步骤见[开发电脑上的实施计划](../docs/planning/implementation/pc-edge-software.md)，接续状态见[HANDOFF](../HANDOFF.md)。课设与网络规范在 `docs/constraints/`。

P1–P5 的模拟数据、遥测、规则、命令和车道检测已实现。现已增加 HC-SR04、MPU6050/6500/9250、AHT20、GPIO 蜂鸣器和电脑摄像头 HTTP JPEG 驱动。当前实机配置与启动方法见下方“真实传感器与电脑摄像头”；较早的软件测试报告见 [P1](../docs/verification/unit/p1-local-data-chain.md)、[P2](../docs/verification/unit/p2-uplink.md)、[P3](../docs/verification/unit/p3-local-warnings.md)、[P4](../docs/verification/unit/p4-commands.md)、[P5](../docs/verification/unit/p5-lane.md) 与 [联调](../docs/verification/integration/p5-live-stack.md)。

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

- 实机输入为 HC-SR04（配电平转换模块）、已识别为 MPU6500 的陀螺仪、AHT20。当前摄像头方案为电脑摄像头经 USB 以太网传图；原 OV5647 的硬件排查见 HANDOFF。默认配置仍是 mock。
- Python目标版本≥3.11。pyproject仅描述可安装包，运行时依赖将在对应阶段锁定；Picamera2/GPIO等硬件依赖不能阻止mock启动。
- 配置文件相对路径按配置文件目录解析。示例的 `../data/edge.db` 指向edge/data。
- 本地覆盖使用config/local.yaml或环境变量。课设固定示例密码按用户要求随配置入 Git；数据库、大视频和运行截图不进 Git。
- 线上schema_version=1，时间为UTC；持续时间用单调时钟；读取故障保留状态和null，不能补0。
- edge与Go使用不同SQLite文件。后端运行在另一checkout/主机，通过MQTT和HTTP联调。
- 同一device_id只能有一个活跃边缘实例。新实例上线前关闭该设备的Go模拟器。

## 树莓派演示（USB 电脑摄像头）

`config/raspberrypi-demo.yaml` 是纳入 Git 的课设测试配置，默认设备 `car-001`，使用 mock 传感器、USB 连接的 Ubuntu 电脑摄像头、远程配置和图片上传。摄像头已替换损坏的 CSI 模块，两端设置和 token 加载见 [USB 摄像头部署](../docs/deployment/usb-pc-camera.md)。MQTT / HTTP 设备密码分别为 `piguard-device-mqtt-demo`、`piguard-device-http-demo`，与 [平台演示配置](../go-backend/.env.demo) 一致。

在 Raspberry Pi OS / Debian 13 的仓库根目录安装：

```sh
sudo apt-get update
sudo apt-get install -y git python3-venv python3-pip python3-smbus2 python3-lgpio i2c-tools
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

已补齐设备 HTTP 事件确认、远程规则配置、软件帧拍照/预览上传、启动连接重试和 SIGTERM 停止。此节记录较早的软件联调；最新硬件驱动与配置见下节。

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

## 真实传感器与电脑摄像头（2026-10-22）

实机配置为已纳入 Git 的 `config/raspberrypi-connected-demo.yaml`。它与平台 `.env.demo` 使用同一套固定示例密码，直接使用 USB 对端 `10.12.194.2`。

仓库同时保留 Ubuntu 摄像头部署入口，按所选配置使用配套服务：

| 边缘配置 | 摄像头驱动与服务 | 默认地址/认证 |
| --- | --- | --- |
| `raspberrypi-connected-demo.yaml` | `network` + `scripts/serve-pc-camera.py`（FFmpeg） | `10.12.194.2:8090`，YAML 固定 `stream_token`；真实传感器 |
| `raspberrypi-demo.yaml` | `remote` + `python -m piguard_edge.pc_camera`（OpenCV） | `192.168.7.1:8765`，两端 `PIGUARD_CAMERA_TOKEN`；mock 传感器 |
| `default.example.yaml` | `synthetic` | 合成道路；纯软件演示 |

两种网络摄像头使用各自的采样时间头，配置需匹配对应服务。Ubuntu/systemd 方案详见 [部署说明](../docs/deployment/usb-pc-camera.md)，下文描述 `connected` 配置。

| 输入/输出 | 驱动 | 当前配置 |
| --- | --- | --- |
| HC-SR04 | `hcsr04` | BCM23 触发、BCM24 回波，经电平转换；10 Hz |
| 陀螺仪 | `mpu` | I²C1、0x68、500 dps、50 Hz；支持 WHO_AM_I 0x68/0x70/0x71 |
| 温度 | `aht20` | I²C1、0x38、1 Hz；检查校准状态、忙标志和 CRC |
| 电脑摄像头 | `network` | HTTP JPEG 10 fps，车道检测 5 fps，640×480 |
| 蜂鸣器 | `gpio` | 物理 Pin11 = BCM17，500 Hz 软件方波 |
| 速度 | `scenario` | 场景模拟，遥测明确标记 `source=mock` |

温度、距离、三轴角速度和摄像头使用 `source=hardware`。AHT20 同时解出湿度并保存在驱动的 `humidity_rh`，当前 v1 页面/遥测只接温度。BMP280 地址保留为 0x77，当前不采集气压。LED 未接，真实模式下灯光测试返回 `ACTUATOR_UNAVAILABLE`。

桌面演示沿用 `distance_scale: 100`：真实测距 0.20 m 对应规则和页面里的演示车距 20 m。遥测分别保存 `raw_value=0.20`、`mapped_value=20` 和 `scale=100`；直接使用物理米数时将比例改为 1，并配套调整预警距离和时距参数。

采样线程保留原始采样时刻；I²C 转换、超声波等回波和网络取帧在后台运行，主循环继续处理规则、命令和遥测。读取失败上报 `timeout/error/unavailable` 和空值。网络断流或帧超过 1 秒后停用图像和车道结果，拍照走已有上传/Ack 流程。相同帧 ID 不刷新时间，HTTP 客户端直连对端并拒绝重定向。

### USB 以太网

USB-C 线需要支持数据传输。先通过系统的 `rpi-usb-gadget` 启用以太网 gadget；树莓派有 `usb0` 后，从仍可用的 SSH 会话在仓库根目录执行：

```sh
sudo ./edge/scripts/configure-usb-pi.sh
# 首次保存 g_ether MAC 后重启一次，再配置电脑端接口：
sudo reboot
```

脚本关闭官方 ICS 服务的 DHCP/共享角色轮换，建立 `10.12.194.1/28` 固定地址，不改变 Wi-Fi 默认路由；原连接概况和 ICS 状态备份到 `/var/backups/piguard-usb/`。使用 `g_ether` 时，将两端 MAC 保存到 `/etc/modprobe.d/piguard-usb-addresses.conf`；模块参数为空时使用固定示例 MAC，重启后生效，使电脑接口名稳定。

Linux 电脑安装 NetworkManager 后，用 `nmcli device` 找到 USB 网卡名，再在仓库根目录执行（替换实际接口名）：

```sh
sudo ./edge/scripts/configure-usb-host.sh enx061eb8028872
```

电脑固定为 `10.12.194.2/28`，不抢占默认路由。脚本检测到 Mihomo 的 `inet4_local_address_set` 时加入 USB 网段直连例外；代理重启重建规则后可重跑本脚本。其他代理需将 `10.12.194.0/28` 配为直连。

先前超时有两层原因：USB ECM 初始化曾卡住；恢复后，ICS 每约 14 秒尝试 DHCP 又短暂撤掉 USB 地址。固定角色后，实机连续 600 次 HTTP 请求全部成功，最大耗时约 21.4 ms。若冷启动出现 ECM TX watchdog、USB 两端互不通，可通过 Wi-Fi SSH 在停止边缘程序后重新加载 `g_ether` 并激活上述连接；此操作会断开 USB 链路。系统问题参考 [官方 gadget 工程](https://github.com/raspberrypi/rpi-usb-gadget) 的 [ECM 端点锁死修复讨论](https://github.com/raspberrypi/rpi-usb-gadget/pull/31)。

### 启动顺序

以下命令均在仓库根目录执行。先在电脑启动后端/MQTT，再启动摄像头服务；Linux 电脑需安装 FFmpeg，摄像头设备默认为 `/dev/video0`：

```sh
./go-backend/scripts/run-demo.sh
# 另一个终端：
python3 edge/scripts/serve-pc-camera.py --bind 10.12.194.2 --port 8090
# 前端终端：
cd frontend && npm run dev -- --host 0.0.0.0
```

摄像头接口 `/frame.jpg` 和 `/health` 使用 `Authorization: Bearer piguard-camera-demo`。可用 `--device`、`--width`、`--height`、`--fps`、`--token` 修改；改 token 后同步修改实机 YAML 的 `stream_token`。摄像头服务只在启动期间采集，Ctrl+C 会停止 FFmpeg。

树莓派安装前文的系统库和 Python 环境后，执行 `sudo raspi-config nonint do_i2c 0` 开启 I²C，并重启使 `/dev/i2c-1` 可用；然后在仓库根目录启动：

```sh
PIGUARD_EDGE_CONFIG=config/raspberrypi-connected-demo.yaml ./edge/scripts/run-demo.sh
```

`PIGUARD_EDGE_CONFIG` 的相对路径按 `edge/` 目录解释，也可传绝对路径。启动前停止相同 `car-001` 的旧边缘进程/Go 模拟器；检查相邻 `local.yaml` 是否有旧覆盖。蜂鸣器保留自动预警优先级，已有 warning/danger 时手工测试会返回 `DEVICE_BUSY`。

本轮在拔下树莓派后完成传感器和网络视频接入，使用假 I²C/GPIO、生成 JPEG 和本机临时 HTTP 服务做自动化测试；新组合的上板联调待设备重新连接。拔下之前，网页蜂鸣器测试已由用户确认听到声音，USB 的 600 请求测试已完成。
