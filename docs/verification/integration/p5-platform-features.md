# 软件联调修复验证（2026-10-15）

## 范围

最初使用真实 Raspberry Pi 4B（Debian 13 / aarch64 / Python 3.13.5）运行 mock 边缘端，电脑运行认证 Mosquitto、Go 后端和 Vue 前端。完成六场景、命令/Ack、遥测历史、告警、双 WebSocket、设备正常退出/重连及去重检查。设备侧 82 项测试通过，1 项因未安装 Go 而跳过；电脑上对应 83 项全部通过。没有接传感器、相机、GPIO 或执行器。

随后用户拔下树莓派。本次代码修复的新增验收全部在电脑运行：Ubuntu 24.04、Python 3.12.3（临时 venv）、Go 1.25.5、Node 22.14.0、Mosquitto 2.0.18、OpenCV 4.11.0。没有重新连接或部署树莓派。新增集成测试使用认证真实 Broker 和独立 Go 进程，边缘端为本机 mock，端口/凭据/SQLite/图片均独立。

## 修复

- 前端：开启开发代理 WebSocket；无预览图片作为空状态；销毁页面清理轮询（含异步初始化后的清理）；在线状态采用遥测和设备列表；车道检测状态明确显示，缺少来源显示 `--`；历史保持接口的升序；配置保存提示“等待确认”并轮询同步状态。
- MQTT：后台初次连接及重连；本地采样不等待网络；事件、命令 Ack、配置 Ack 的本地已发送标记等待发布确认，发送失败保留待发记录；每批最多 64 条。
- 摄像头：capture_fps 与 process_fps 分开，缓存检测结果不刷新检测时间；关闭时释放录像资源；SIGTERM 与 SIGINT 都触发正常关闭。
- 配置：完整规则校验、SQLite 记录首个结果、重复回原 Ack、冲突保留首条、旧版本不回退、重启恢复成功版本、保留活跃告警的恢复路径。沿用平台四组规则，平台未定义的时距与高温恢复持续时间保留本地值。
- 图片：显式启用后，从当前有效软件帧上传 preview/snapshot；独立后台 HTTP；拍照成功只使用真实上传返回的 snapshot_id；同一上传重试复用身份、时间、字节；最多三次，认证/参数拒绝不重试；故障/过期/取消均明确失败，命令重放不重复拍照。
- 事件：每 5 秒分批轮转查询持久入库确认，确认后按原保留策略回收；失败保留缓存。新增设备专用确认接口只读、限 64 条/256 KiB、严格比较不可变事件内容；设备只能确认自己报文，不能访问平台事件列表。

## 自动验证

- `GOCACHE=/tmp/piguard-go-cache python -m pytest tests/unit tests/contract -q --tb=short`：边缘端 105 项单元和契约回归通过（3.52 秒），包含发送拒绝/延迟 PUBACK、初次连接恢复、配置校验/持久化/幂等、图片重试/故障/后台采样、事件确认失败保留和 SIGTERM。
- `go test -count=1 ./...`：通过。
- `go test -race -count=1 ./...`：通过。
- `go vet ./...`：通过。
- `npm run build`：类型检查、构建通过；保留已有 Element Plus/ECharts 资源包超过 500 kB 的提示。
- `python -m pytest tests/integration/test_platform_features.py -q --tb=short`：真实认证 Broker/Go 联调 1 项通过（7.07 秒）。

新增集成测试覆盖：Broker 未启动时先启动边缘端且采样继续；Broker 上线后恢复；配置 v1 自动确认；preview JPEG 可读取；camera.snapshot 上传和成功 Ack；重复拍照命令 executed_at 不变；蜂鸣器 mock 命令；配置 v2 修改阈值并实际触发 danger；边缘 SQLite 事件被平台确认；Broker 重启后恢复且事件 ID 不增加；进程重启恢复配置，CLI 收到 SIGTERM 正常退出并离线。测试结束只清理自己启动的进程。

复现入口和设备 local.yaml 示例见 edge/README.md；确认接口契约见 go-backend/README.md。执行网络测试需要允许监听本机端口；受限沙箱禁止套接字时不能把环境失败算作程序失败。

## 限制

不包含修复代码在 ARM/树莓派上的复测、真实摄像头拍摄或传感器/GPIO 验证。mock 拍照是软件图像。没有做长时间浸泡或硬断电恢复验收。远程配置与图片默认关闭，启用方式已写入运行文档；HTTP 与 MQTT 凭据独立，设备上不用平台账号。
