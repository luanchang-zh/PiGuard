# PiGuard

基于 Raspberry Pi 4B 的车载驾驶辅助预警课程原型：Python 负责采集、本地预警与执行器，Go 负责平台服务。

本分支 `edge` 从 `main` 创建，专门承载树莓派端。Go 后端保留在 `go后端开发` 分支，联调时独立运行。

- [树莓派端开发规划](docs/planning/PLAN.md)：总规范。后端能力基线、模块职责、阶段门槛与验收标准。
- [开发电脑上的实施计划](docs/planning/implementation/pc-edge-software.md)：当前可执行的 P1—P5，以及 P1 的实施步骤。
- [测试报告](docs/verification/README.md)：按单元、契约、联调、硬件分层。P1 到 P4 的软件结果已记录。
- [边缘端目录与开发入口](edge/README.md)。
- [接续开发状态](HANDOFF.md)。
- [课程设计方案](docs/constraints/raspberry_pi_adas_course_design.md)：课设整体约束。
- [网络交互规范](docs/constraints/adas_network_api_spec.md)：网络交互整体约束。

当前已完成开发电脑上的模拟数据链、遥测/状态上行、四条本地预警和四种下行命令。四类传感器型号已写入课设规范和开发规划：HC-SR04、OV5647、MPU6050、AHT20。真实 Broker 联调和硬件驱动仍在后续阶段。共享文档继承自 `main`；与已完成 Go 后端的契约差异见开发规划。
