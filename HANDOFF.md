# 树莓派边缘端接续状态

## 目标与当前状态

用户要求阅读项目与已完成Go后端，制定树莓派开发规划、设计目录并创建隔离分支。硬件尚未确定，已确认按Pi4B和模拟驱动规划。

当前分支 `edge` 从 `main@e975d4b51d0dd5d42b9c1f1bb1e8ae0c1f0a6113` 创建。后端分支仍为 `go后端开发@0e65a536a3fe01ae4a866bbe91b695ea25f3623b`。已完成代码/文档阅读、规划和包目录骨架；业务采集、规则、MQTT和硬件驱动均尚未实现。

## 重要文件与已确定事项

- `docs/planning/PLAN.md`：后端事实、契约差异、P0—P7和验收门槛。
- `docs/constraints/`：课设方案与网络交互规范，作为整体约束，不随本分支实施计划改写。
- `edge/README.md`：模块边界和入口；`edge/config/default.example.yaml` 是设计示例，尚无加载器。
- 后端已完成telemetry/status、四类动作/Ack、events；配置/图片仍不可用。
- 命令有效期10秒，Ack执行时间为UTC；事件恢复level仍为warning/danger，三种action各用新event_id。
- PUBACK不证明事件入库；规划使用已有HTTP事件查询确认，未确认缓存不自动删除。
- 分支从main而非后端创建，本目录不含go-backend；联调使用独立后端checkout/服务。main共享规范可能较旧，实际兼容基线见PLAN。

## 下一步

按PLAN的P1实施Sample/Frame/LaneResult、五类数据接口、统一ScenarioEngine六个场景、YAML加载校验和LatestState/workers；先用fake clock验证。之后P2接真实Broker与现有Go服务，不同时运行相同car-001的Go模拟器。

用户本轮只要求规划、目录和分支，后续业务实现范围以新的明确任务为准。型号确定后再填具体接线与驱动。

## 验证

已在Python 3.13.12导入全部18个骨架包；FastCtx读取的TOML内容经tomllib解析，版本/src布局一致；YAML示例已人工核对。已确认新分支基点为main、当前目录无go-backend、后端分支仍指向0e65a536。git diff --cached --check已通过，待提交的33个任务文件已复核，范围仅为规划/说明/edge骨架。历史Go验收报告只作为已有能力证据；本轮未重跑Go test/race/vet，未运行Broker、Docker、树莓派或硬件。
