# PiGuard

基于 Raspberry Pi 4B 的车载驾驶辅助预警课程原型：Python 负责采集、本地预警与执行器，Go 负责平台服务。

仓库包含 Python 边缘端 `edge/`、Go 平台 `go-backend/` 和 Vue 前端 `frontend/`。树莓派 CSI 摄像头损坏后，演示配置改用 USB 连接的 Ubuntu 24.04 电脑摄像头。

前后端现已部署到腾讯云 Ubuntu 服务器：[打开 PiGuard](http://106.54.188.216/)。前端、API、实时 WebSocket 和设备 MQTT 共用 80 端口；部署更新、登录凭据位置及树莓派接入见 [云服务器部署](docs/deployment/cloud-server.md)。本地演示启动方式继续保留。

- [树莓派端开发规划](docs/planning/PLAN.md)：总规范。后端能力基线、模块职责、阶段门槛与验收标准。
- [开发电脑上的实施计划](docs/planning/implementation/pc-edge-software.md)：当前可执行的 P1—P5，以及 P1 的实施步骤。
- [测试报告](docs/verification/README.md)：按单元、契约、联调、硬件分层。P1 到 P4 的软件结果已记录。
- [边缘端目录与开发入口](edge/README.md)。
- [Ubuntu 摄像头经 USB 接入树莓派 4B](docs/deployment/usb-pc-camera.md)：替换损坏的 CSI 摄像头，包含两端安装、USB 网络和验收步骤。
- [接续开发状态](HANDOFF.md)。
- [课程设计方案](docs/constraints/raspberry_pi_adas_course_design.md)：课设整体约束。
- [网络交互规范](docs/constraints/adas_network_api_spec.md)：网络交互整体约束。

当前已实现模拟数据链、遥测/状态上行、本地预警、下行命令、图片上传和远程配置，并补齐 HC-SR04、MPU 系列、AHT20 和 GPIO 蜂鸣器驱动。电脑摄像头通过 USB 网络供树莓派检测与抓拍；真实传感器的组合配置与固定 USB 网络启动方法见 [边缘端 README](edge/README.md#真实传感器与电脑摄像头2026-10-22)。最新部署状态见接续开发记录。
