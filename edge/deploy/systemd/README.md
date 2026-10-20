# 树莓派服务部署

`piguard-pc-camera.service` 是另一台 Ubuntu 电脑的用户级摄像头服务。安装和环境文件步骤见 [USB 摄像头部署](../../../docs/deployment/usb-pc-camera.md)。它不部署到树莓派。

P7在CLI和真实驱动可运行后添加piguard-edge.service：固定工作目录/配置路径、独立运行用户、重启策略、日志、可取消的停止流程。Broker不可达仍允许本地采集与预警启动；凭据不写进服务文件或Git。
