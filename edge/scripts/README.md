# 工具入口

| 脚本 | 运行位置 | 用途 |
| --- | --- | --- |
| `run-demo.sh` | 树莓派 | 启动边缘端；`PIGUARD_EDGE_CONFIG` 选择 mock 或真实传感器配置 |
| `configure-usb-pi.sh` | 树莓派、sudo | 固定 usb0 为 10.12.194.1/28，停用 ICS 角色轮换并保留 MAC |
| `configure-usb-host.sh USB_INTERFACE` | Linux 电脑、sudo | 固定 USB 网卡为 10.12.194.2/28，并处理已存在的 Mihomo TUN 直连集合 |
| `serve-pc-camera.py` | Linux 电脑 | 用 FFmpeg 采集 V4L2 摄像头，以带 token 的 HTTP JPEG 接口提供给树莓派 |

完整启动顺序、接线和依赖见 [边缘端 README](../README.md#真实传感器与电脑摄像头2026-10-22)。
