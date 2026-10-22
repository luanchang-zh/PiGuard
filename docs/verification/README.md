# 测试报告

这里只记录一次运行的结果。阶段门槛在 [总规范](../planning/PLAN.md)，实施步骤在 [实施计划](../planning/implementation/)。测试代码在 `edge/tests/`，不把用例正文复制到这里。

四层目录与代码目录同名：

```text
docs/verification/
├── unit/           # edge/tests/unit
├── contract/       # edge/tests/contract
├── integration/    # edge/tests/integration
└── hardware/       # edge/tests/hardware
```

文件名是 `pN-短名.md`。同一阶段、同一层只保留最新一份，下次运行替换结论，不另存历史副本。某一层还没有运行时，不预建空报告。

当前记录：

- [P1 单元测试](unit/p1-local-data-chain.md)
- [P2 单元测试](unit/p2-uplink.md)
- [P2 契约测试](contract/p2-telemetry.md)
- [P3 单元测试](unit/p3-local-warnings.md)
- [P3 契约测试](contract/p3-events.md)
- [P4 单元测试](unit/p4-commands.md)
- [P4 契约测试](contract/p4-acks.md)
- [P5 单元测试](unit/p5-lane.md)
- [真实 Broker 联调](integration/p5-live-stack.md)

- [软件联调修复：配置、图片、事件确认与恢复](integration/p5-platform-features.md)

## 2026-10-22 真实输入开发离线回归

用户拔下树莓派后，本轮仅用假 I²C/GPIO、生成的道路 JPEG 和本机临时 HTTP 服务验证新增代码。没有继续访问树莓派或真实摄像头。

| 检查 | 结果 |
| --- | --- |
| `python -m pytest edge/tests/unit edge/tests/contract -q` | 175 passed；整合远端 Ubuntu 摄像头后重跑，包括历史基线与当前工作区 Go 解析器 |
| Go 后端 `go test ./...` | 全部通过 |
| Go 模块目录 `gopls check internal/protocol/telemetry.go internal/protocol/telemetry_test.go` | 通过，无诊断 |
| 前端 `npm run build` | 类型检查与构建通过；保留原有分包体积/路由导入提示 |
| shell 语法、Python 编译、`git diff --check` | 通过 |

新增覆盖：MPU 三种芯片 ID、符号与量程换算；AHT20 校准/温湿度/CRC/忙超时；HC-SR04 边沿时间、回波丢失/卡高/超量程；后台采样不阻塞读取和采样时间保留；摄像头鉴权、重复帧、过期、畸形 JPEG、时区、重定向拒绝、断流；平台 HTTP 绕过环境代理；GPIO 蜂鸣器截止时间、自动预警优先级、故障 Ack；启动失败和关闭异常时资源释放。

电脑测试环境使用 Python 3.12、OpenCV 4.11.0.86；系统 Python 加载临时虚拟环境依赖，避免本机 Nix Python 的动态库搜索差异。测试未修改正式依赖版本。

拔下之前的实机证据另记：网页触发物理 Pin11/BCM17 蜂鸣器后用户确认听到；USB 停用 ICS 地址轮换后 600/600 HTTP 请求成功，最大约 21.4 ms。新增真实传感器与电脑视频的组合尚待重新上板联调，启动方法见 [边缘端 README](../../edge/README.md#真实传感器与电脑摄像头2026-10-22)。
