# Ubuntu 24.04 摄像头经 USB 接入 Raspberry Pi 4B

## 方案与边界

树莓派 CSI 摄像头损坏后，改用另一台 Ubuntu 24.04 电脑的摄像头。这台开发用 Windows 电脑不参与采集，也不需要开放摄像头权限。

```text
Ubuntu 的 /dev/video0
  → piguard_edge.pc_camera（640×480、JPEG、Bearer Token）
  → USB Ethernet（Ubuntu 192.168.7.1 ↔ Pi 192.168.7.2）
  → Pi 的 remote 摄像头驱动
  → 原有车道检测 / preview / camera.snapshot → 现有 Go 平台 → 前端
```

1. Ubuntu 负责连续采集和 JPEG 编码，只保留最新帧；摄像头拔下后会释放设备并每秒重试打开。
2. 树莓派后台拉取 `/frame.jpg`，采样主循环不等待 HTTP。取图失败即清空图片；超过 `remote_max_age_ms` 的图片不能用于检测、预览或抓拍。两端用单调时钟和帧年龄判定新鲜度，不依赖电脑与 Pi 的 UTC 完全同步。
3. 沿用平台的抓拍命令、上传鉴权、snapshot_id 和命令幂等逻辑。抓拍使用最近一张有效画面；没有有效画面时返回 `CAMERA_UNAVAILABLE`。
4. 演示配置 `edge/config/raspberrypi-demo.yaml` 已切换到 `remote`。其他传感器与执行器继续使用现有 mock 驱动，整机状态仍是 mock；收到的实际摄像头帧标记为 hardware。未实现的传感器硬件驱动不在这次修改范围内。
5. `default.example.yaml` 继续提供纯软件合成道路测试。USB 只承担网络传输，电脑摄像头不需要伪装为 UVC 外设。平台可以运行在另一主机，MQTT/HTTP 平台地址与摄像头地址分别配置。

## 1. 连接 USB 网络

4B 要用 **USB-C 口**接 Ubuntu 的 USB 数据口，数据线必须支持传输。4B 的 USB-A 口不能用普通 A-A 线连接另一台电脑作为本方案链路。依据 [树莓派官方 OTG 白皮书](https://pip-assets.raspberrypi.com/categories/685-app-notes-guides-whitepapers/documents/RP-009276-WP-1-Using%20OTG%20mode%20on%20Raspberry%20Pi%20SBCs)，4B 可在 USB-C 口启用 peripheral 模式。

4B 官方供电规格为 5V/3A，普通电脑 USB 口可能供电不足。部署前确认供电方案；如需外部供电，按硬件说明使用支持数据连接的供电方案或 PoE HAT，避免将两路 5V 电源直接并联。参见 [4B 产品规格](https://datasheets.raspberrypi.com/rpi4/raspberry-pi-4-product-brief.pdf)。

以下是 Raspberry Pi OS Bookworm/Trixie 的手工 `g_ether` 配置。不要同时启用另一套 ConfigFS / rpi-usb-gadget 服务，否则会争用 USB 控制器；已能 USB 联网时直接使用现有接口和地址，跳过本节并修改后文 URL。

先通过原来的网络或本地终端在 Pi 上操作，备份启动文件：

```sh
sudo cp -an /boot/firmware/config.txt /boot/firmware/config.txt.before-piguard-usb
sudo cp -an /boot/firmware/cmdline.txt /boot/firmware/cmdline.txt.before-piguard-usb
sudo nano /boot/firmware/config.txt
```

在 `[all]` 段配置以下一行；若已有 `dtoverlay=dwc2`，修改原行，避免重复。检查并移除该 USB 控制器的强制 host 配置，例如 `otg_mode=1`：

```ini
[all]
dtoverlay=dwc2,dr_mode=peripheral
```

编辑 `/boot/firmware/cmdline.txt`，在原有内容末尾空一格追加以下参数。**保留原参数，整个文件保持一行**；若已有 `modules-load`，合并其模块列表而不是添加第二个同名参数：

```text
modules-load=dwc2,g_ether
```

重启 Pi，连接数据线，检查接口：

```sh
sudo reboot
# 重连后运行
ip -br link
```

Pi 应出现 `usb0`。Ubuntu 运行 `ip -br link` 找到插线后新增的 `enx...` 或 `usb0`，不要误选 Wi-Fi/有线平台网络。两端以下命令各执行一次，建立独立静态子网且不修改默认路由。若已有同名连接，使用 `nmcli con modify` 修改该连接。

Pi（NetworkManager）：

```sh
sudo nmcli con add type ethernet ifname usb0 con-name piguard-usb \
  ipv4.method manual ipv4.addresses 192.168.7.2/24 ipv4.never-default yes \
  ipv6.method disabled connection.autoconnect yes
sudo nmcli con up piguard-usb
```

Ubuntu 24.04（把 `enx实际接口名` 改成刚查到的名称）：

```sh
PC_USB_IF=enx实际接口名
sudo nmcli con add type ethernet ifname "$PC_USB_IF" con-name piguard-usb \
  ipv4.method manual ipv4.addresses 192.168.7.1/24 ipv4.never-default yes \
  ipv6.method disabled connection.autoconnect yes
sudo nmcli con up piguard-usb
ping -c 3 192.168.7.2
```

Pi 运行 `ping -c 3 192.168.7.1`。若地址与现有网络/VPN 重叠，给两端换一个空闲子网并同步修改 URL、服务监听地址。NetworkManager 命令依据 [官方 nmcli 示例](https://networkmanager.dev/docs/api/latest/nmcli-examples.html)。

## 2. Ubuntu 安装与启动摄像头服务

以下在**有摄像头的 Ubuntu 24.04 电脑**上执行，当前目录为最新仓库根目录：

```sh
sudo apt-get update
sudo apt-get install -y python3-venv python3-pip v4l-utils openssl
python3 -m venv "$HOME/.venvs/piguard-camera"
"$HOME/.venvs/piguard-camera/bin/python" -m pip install -e ./edge
v4l2-ctl --list-devices
v4l2-ctl --device=/dev/video0 --list-formats-ext
```

默认设备 `/dev/video0`，可通过 `--device /dev/video2` 或 `/dev/v4l/by-id/...` 选择其他 V4L2 摄像头。Ubuntu 服务用 [OpenCV VideoCapture](https://docs.opencv.org/4.11.0/d8/dfe/classcv_1_1VideoCapture.html) 读取设备，无需图形窗口。先关闭占用摄像头的视频会议软件。如提示权限不足，确认 `ls -l /dev/video0`，将运行用户加入设备所属的 `video` 组并重新登录：`sudo usermod -aG video "$USER"`。

创建两端共用的随机 token（只在首次部署生成；后续重用此文件）：

```sh
install -d -m 700 "$HOME/.config/piguard"
(umask 077; test -f "$HOME/.config/piguard/camera.env" || \
  printf 'PIGUARD_CAMERA_TOKEN=%s\n' "$(openssl rand -hex 32)" > "$HOME/.config/piguard/camera.env")
set -a
. "$HOME/.config/piguard/camera.env"
set +a
sh edge/scripts/run-pc-camera.sh
```

默认只监听 USB 地址 `192.168.7.1:8765`。可用参数示例：`sh edge/scripts/run-pc-camera.sh --device /dev/video2 --fps 10 --quality 85`。更换地址时设置 `PIGUARD_CAMERA_HOST`。直接运行模块时 `--host` 默认是 `127.0.0.1`，跨机部署必须显式指定 USB IP。

若 Ubuntu 启用了 UFW，允许 USB 接口来自 Pi 的该端口即可：

```sh
sudo ufw allow in on "$PC_USB_IF" from 192.168.7.2 to 192.168.7.1 port 8765 proto tcp
```

服务提供带相同 Bearer 鉴权的 `/healthz`（新鲜画面才返回 200）和 `/frame.jpg`。它用于 USB 私有链路，不要开放到公网。token 从环境读取，不写入 YAML、URL 或 Git。

## 3. Pi 启用电脑摄像头

Pi 拉取包含本次改动的代码，用现有 Python 环境重新安装：

```sh
"$HOME/.venvs/piguard-edge/bin/python" -m pip install -e ./edge
```

把 Ubuntu 的 `~/.config/piguard/camera.env` 安全复制到 Pi 的同一路径，设置权限为 600。若 Pi 已开启 SSH，可在 Ubuntu 执行（替换 `PI_USER`）：

```sh
PI_USER=你的树莓派用户名
ssh "$PI_USER@192.168.7.2" 'install -d -m 700 "$HOME/.config/piguard"'
scp "$HOME/.config/piguard/camera.env" "$PI_USER@192.168.7.2:.config/piguard/camera.env"
ssh "$PI_USER@192.168.7.2" 'chmod 600 "$HOME/.config/piguard/camera.env"'
```

Pi 上加载相同 token，验证 USB 取图，启动边缘端：

```sh
set -a
. "$HOME/.config/piguard/camera.env"
set +a
curl --noproxy '*' --fail --max-time 2 \
  -H "Authorization: Bearer $PIGUARD_CAMERA_TOKEN" http://192.168.7.1:8765/healthz
curl --noproxy '*' --fail --max-time 2 \
  -H "Authorization: Bearer $PIGUARD_CAMERA_TOKEN" http://192.168.7.1:8765/frame.jpg -o /tmp/piguard-camera.jpg
sh edge/scripts/run-demo.sh
```

演示配置中已包含：

```yaml
sensors:
  camera:
    driver: remote
    capture_fps: 10
    process_fps: 5
    width: 640
    height: 480
    video_path: ""
    remote_url: http://192.168.7.1:8765/frame.jpg
    remote_token_env: PIGUARD_CAMERA_TOKEN
    remote_timeout_s: 0.5
    remote_max_age_ms: 1000
features:
  frame_upload: true
```

同目录 `edge/config/local.yaml` 会覆盖演示配置，检查旧文件是否仍有 `driver: synthetic`。平台仍使用配置中的 `mqtt.host`、`http.base_url` 及设备凭据；如果 Go 平台也运行在这台 Ubuntu，改为其可达地址（可用 `192.168.7.1`），并按后端文档启动 MQTT/HTTP 服务。`remote_timeout_s` 限制在 `(0, 5]` 秒。USB 断开时后台每秒重试，不会改用模拟图片冒充实拍。

## 4. Ubuntu 可选自启动

仓库提供用户级服务，使用前面的默认虚拟环境、token 文件和 USB 地址：

```sh
install -d "$HOME/.config/systemd/user"
cp edge/deploy/systemd/piguard-pc-camera.service "$HOME/.config/systemd/user/"
systemctl --user daemon-reload
systemctl --user enable --now piguard-pc-camera.service
journalctl --user -u piguard-pc-camera.service -f
```

先关闭手动运行的服务以释放端口。修改服务中的 `--device` / `--host` 以匹配实机；USB IP 尚未就绪时服务会重试启动。需要退出登录后继续运行，可执行 `sudo loginctl enable-linger "$USER"`。Pi 若由其他 systemd 服务启动，也要在其服务中加载相同的 `camera.env`。

## 5. 验收与排障

- `/healthz` 为 200、下载文件是 JPEG；前端最新预览显示 Ubuntu 镜头画面。
- 平台发一次 `camera.snapshot`，应收到成功 Ack 和可访问图片；相同 command_id 重发不产生第二次抓拍上传。
- 拔掉 USB 线或摄像头，画面停止有效，摄像头状态变为不可用，抓拍明确失败；其他采样及 MQTT 仍运行。重连后自动恢复。
- 401：两端 token 不一致或未加载；503：设备被占用、没有摄像头或画面已过期；连接失败：先检查 `ip -br addr`、供电、数据线、监听地址和 UFW。
- 已有采样帧过期时，驱动不重复延长其有效期；拒绝无帧年龄、非 JPEG、超 2 MiB、损坏图片和 HTTP 重定向。
- 本次本地自动测试使用 loopback HTTP 和生成的测试像素，不访问本机摄像头。真实 Ubuntu 摄像头、4B USB 枚举、供电和吞吐需要按本节在目标硬件验收，不能由这台 Windows 开发机的软件测试代替。
