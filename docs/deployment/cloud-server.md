# 腾讯云前后端部署

2026-10-27，PiGuard 前端和 Go 后端已部署到腾讯云 Ubuntu 22.04，访问入口为 <http://106.54.188.216/>。树莓派暂未连接，控制台初始显示设备离线。

## 登录与设备配置

- 平台账号：`platform`。本机 `deploy/cloud/.secrets/access.txt` 保存实际随机密码和入口。
- 平台 HTTP、设备 HTTP、平台 MQTT、设备 MQTT、Broker 健康检查分别使用独立密码。
- 私密配置保存在本机 `deploy/cloud/.secrets/` 与 `edge/config/cloud/`，均已加入 Git 忽略；服务器环境文件为 `/etc/piguard/backend.env`，权限 `0600`。
- `.env.demo` 中的固定演示账号用于本地演示；云端登录页隐藏测试账号填充按钮。

当前入口使用 HTTP/WS，连接中的账号和设备数据以明文传输。后续配置域名与 HTTPS 时，应同时更新设备的 HTTP 地址及 MQTT TLS 配置。

## 服务与网络

| 入口 | 用途 |
| --- | --- |
| `http://106.54.188.216/` | Vue 生产静态页面，支持页面刷新和直接打开子路由 |
| `/api/v1/` | 后端 API、Cookie 会话、CSRF 校验、JPEG 图片 |
| `/api/v1/ws` | 管理页面的实时 WebSocket |
| `/mqtt` | 设备 MQTT over WebSocket，端口 80，沿用用户名密码与主题 ACL |
| `/health` | 后端进程与 SQLite 健康状态 |

Nginx 新增独立的 `piguard` 站点，只匹配该服务器 IP，保留原有域名配置。后端监听 `127.0.0.1:18080`，Broker 的 TCP / WebSocket 分别监听 `127.0.0.1:1883` / `127.0.0.1:1884`。公网设备使用 `transport: websockets`、`port: 80`、`websocket_path: /mqtt`，全部通过 Nginx 入口连接。

| 服务器路径 | 内容 |
| --- | --- |
| `/opt/piguard/releases/` | 各次部署的二进制、静态页面和部署配置 |
| `/opt/piguard/current` | 当前版本软链接 |
| `/etc/piguard/backend.env` | 认证与运行环境变量 |
| `/etc/piguard/mqtt-auth/` | MQTT 哈希密码与角色 ACL |
| `/var/lib/piguard/piguard.db` | SQLite 业务数据库 |
| `/var/lib/piguard/snapshots/` | JPEG 图片 |
| `/var/lib/piguard-mqtt/` | Broker 持久数据 |

`piguard-backend.service` 使用专用低权限系统账号，`piguard-mqtt.service` 使用 `mosquitto` 账号。两者启用开机自启与失败重启；服务数据位于版本目录之外，更新前后端时继续保留。

## 更新部署

本机安装 Go 1.25+、Node.js/npm、Python 3 和 SSH。服务器需要 Nginx 和可用的 sudo 权限；脚本在缺少 Mosquitto 时安装对应 Ubuntu 软件包。

在仓库根目录执行：

```sh
./deploy/cloud/deploy.sh
```

脚本默认目标为 `ubuntu@106.54.188.216`，本机构建 Linux amd64 后端和 Vue 静态页面，上传独立版本，安装服务，检测后端健康并重新加载 Nginx。SSH 使用本机已有身份或交互式密码，不保存 SSH 密码。已有 SSH 控制连接时可传入 `PIGUARD_SSH_CONTROL_PATH`；`PIGUARD_CLOUD_HOST` 和 `PIGUARD_SSH_TARGET` 可指定其他目标。

`prepare.py` 首次生成凭据，后续部署复用。请保留本机 `.secrets` 目录：服务器与本地凭据不一致时，脚本会停止更新，避免改变现有账号。版本切换后的健康检查失败时，脚本恢复上一版本的软链接和 Nginx 站点。

## 树莓派以后接入

先把本次代码更新到树莓派，再把本机私密文件 `edge/config/cloud/raspberrypi.yaml` 复制到树莓派项目的同一相对位置。该文件已经填写云端地址及设备密码，并保留 HC-SR04、MPU6500、AHT20、BCM17 蜂鸣器和 USB 电脑摄像头配置。

树莓派有可访问云服务器的网络后，从项目根目录启动：

```sh
./edge/scripts/run-cloud.sh
```

默认 Python 为 `$HOME/.venvs/piguard-edge/bin/python`，可通过 `PIGUARD_EDGE_PYTHON` 指定。摄像头电脑继续运行原有 JPEG 服务，地址仍为 USB 局域网 `10.12.194.2:8090`。

原来的 `run-demo.sh`、`raspberrypi-demo.yaml`、`raspberrypi-connected-demo.yaml` 继续用于局域网演示。`run-cloud.sh` 显式选择云端配置，避免更换平台时改动摄像头地址。

## 检查与维护

在具备 `paho-mqtt==2.1.0` 和 `websockets==15.0.1` 的本地 Python 环境执行：

```sh
python deploy/cloud/verify.py
```

检查公网静态页面、子路由刷新、健康接口、登录 Cookie、Origin/CSRF 拒绝、管理 WebSocket、设备 MQTT WebSocket 认证订阅及退出失效。脚本只建立临时会话和订阅，不发布设备状态或命令。

服务器维护命令：

```sh
sudo systemctl status piguard-backend piguard-mqtt nginx
sudo journalctl -u piguard-backend -u piguard-mqtt -n 100 --no-pager
sudo systemctl restart piguard-backend
sudo nginx -t
```

本次验证通过前端生产构建、Go `test ./...` / `vet ./...`、Python 178 项单元测试及上述公网检查。浏览器已实际登录控制台，页面实时通道显示已连通。

另以临时软件设备完成 MQTT WebSocket 遥测入库、默认配置确认、拍照命令→设备 HTTP JPEG 上传→成功回执、模拟蜂鸣器回执、实时遥测/命令/图片推送和正常离线；重启后数据库与 JPEG 仍可读取。软件设备已停止，测试数据库和图片归档到服务器 `/var/backups/piguard/smoke-results-*/`，线上恢复测试前的初始状态。
