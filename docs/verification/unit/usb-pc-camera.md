# USB 电脑摄像头软件验证（2026-10-20）

## 实现

- Ubuntu V4L2 摄像头服务：`edge/src/piguard_edge/pc_camera.py`。
- 树莓派异步取图驱动：`edge/src/piguard_edge/drivers/camera/remote.py`。
- 演示配置切换为 remote，原合成道路配置继续可用。
- 两端安装、4B USB-C 网卡配置和目标硬件验收见 [部署文档](../../deployment/usb-pc-camera.md)。

## 运行结果

Windows 开发机、Python 3.12.4，在仓库内隔离的 `edge/.venv` 安装项目运行依赖；测试不调用本机摄像头。

```text
cd edge
.venv/Scripts/python -m pytest tests/unit tests/contract -q -p no:cacheprovider
130 passed in 10.85s
```

新增的 25 个测试覆盖：真实 loopback HTTP 鉴权与健康检查、JPEG 取图与车道检测、断流恢复与资源关闭、旧图片过期、慢请求不阻塞其他传感器、现有抓拍命令上传与幂等、摄像头读取失败后的释放重开、缺失 token、配置校验，以及非 JPEG、无效帧年龄、超长/截断响应、重定向等异常响应。原有单元与 Go 协议契约测试一并通过。

`python -m piguard_edge.pc_camera --help` 正常退出，启动脚本通过 `sh -n`，差异通过 `git diff --check`。

## 验证边界

摄像头测试像素由测试程序生成，V4L2 采集对象用测试替身验证生命周期；没有连接目标 Ubuntu 24.04 电脑或 Raspberry Pi 4B。真实镜头、USB 网卡枚举、供电和端到端吞吐尚未硬件验收，本报告不将软件结果视为实机通过。
