# PiGuard

基于 Raspberry Pi 4B 的车载驾驶辅助预警课程原型：Python 负责采集、本地预警与执行器，Go 负责平台服务。

本分支 `codex/raspberry-pi-edge` 从 `main` 创建，专门承载树莓派端。Go 后端保留在 `go后端开发` 分支，联调时独立运行。

- [树莓派端开发规划](docs/PLAN.md)：后端能力基线、模块职责、实施顺序与验收标准。
- [边缘端目录与开发入口](edge/README.md)。
- [接续开发状态](HANDOFF.md)。
- [课程设计方案](docs/raspberry_pi_adas_course_design.md)。
- [网络交互规范](docs/adas_network_api_spec.md)。

当前交付为规划和目录骨架，采集、规则、MQTT 和硬件驱动将在后续阶段实现。共享文档继承自 `main`；与已完成 Go 后端的契约差异见开发规划。
