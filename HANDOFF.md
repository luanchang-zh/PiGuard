# 树莓派边缘端接续状态

## 目标与当前状态

用户已确定四类传感器型号。课设规范与 `docs/planning/PLAN.md` 已写入型号、电气约束和实机顺序。业务采集、规则、MQTT 和硬件驱动仍未实现。

当前分支 `edge` 从 `main@e975d4b51d0dd5d42b9c1f1bb1e8ae0c1f0a6113` 创建。后端分支仍为 `go后端开发@0e65a536a3fe01ae4a866bbe91b695ea25f3623b`。

## 重要文件与已确定事项

- `docs/planning/PLAN.md`：后端事实、契约差异、已定硬件、P0—P7 和验收门槛。
- `docs/constraints/raspberry_pi_adas_course_design.md`：课设约束。传感器型号写在概述、技术栈和第 8、11–14 节。网络规范未改，湿度、气压、加速度不上报文。
- `edge/README.md`：模块边界和入口；`edge/config/default.example.yaml` 是设计示例，尚无加载器。
- 后端已完成telemetry/status、四类动作/Ack、events；配置/图片仍不可用。
- 命令有效期10秒，Ack执行时间为UTC；事件恢复level仍为warning/danger，三种action各用新event_id。
- PUBACK不证明事件入库；规划使用已有HTTP事件查询确认，未确认缓存不自动删除。
- 已定型号：HC-SR04 加 8 路电平转换模块、OV5647IMXZ19、GY-521 MPU6050、AHT20（同模块 BMP280 只做识别）。默认接线在 PLAN 第 1.1 节。蜂鸣器与 LED 型号未知，引脚未分配。
- 分支从main而非后端创建，本目录不含go-backend；联调使用独立后端checkout/服务。main共享规范可能较旧，实际兼容基线见PLAN。

## 下一步

按PLAN的P1实施Sample/Frame/LaneResult、五类数据接口、统一ScenarioEngine六个场景、YAML加载校验和LatestState/workers；先用fake clock验证。加载器要接受 `hardware` 段，mock 不使用它。之后P2接真实Broker与现有Go服务，不同时运行相同car-001的Go模拟器。P6 等树莓派在手后按规划顺序接入，执行器继续等待型号。

## 验证

此前已在 Python 3.13.12 导入全部 18 个骨架包，并核对分支基点与后端提交。本轮只更新规范和规划，未重新导入包，未上电，未运行 Broker、Go 测试或树莓派。
