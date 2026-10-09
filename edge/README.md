# PiGuard 树莓派边缘端

Python边缘工程，开发分支为 `edge`。完整开发顺序和实际Go契约见[开发规划](../docs/PLAN.md)，接续状态见[HANDOFF](../HANDOFF.md)。

当前只有包目录骨架和配置设计示例；没有可运行的采集/预警服务、CLI、MQTT或硬件驱动。后续P1先实现本地模拟数据链，P2再接后端。

## 模块职责

| 目录 | 职责 |
| --- | --- |
| src/piguard_edge/config | 配置加载、校验和不可变快照 |
| models | 内部Sample、Frame、LaneResult和状态，保留采样质量与时钟 |
| protocol | MQTT v1消息编解码，与内部模型分开 |
| drivers | 按数据源分目录；mock/video/hardware通过相同接口 |
| simulation | 六个一致、可重复的场景及播放倍率 |
| processing | 滤波、比例映射、角速度与OpenCV车道结果 |
| rules | 本地规则状态机、事件生成和综合风险 |
| actuators | 模拟/真实蜂鸣器与LED、自动预警优先级 |
| commands | 命令校验、执行调度、幂等与完整Ack |
| transport | MQTT连接/重连/遗嘱、HTTP确认和后续图片上传 |
| storage | 独立edge.db、命令结果、事件缓存和配置记录 |
| runtime | 采集worker、LatestState、启动与关闭资源 |
| tests | unit/contract/integration/hardware分层证据 |
| assets、data | 本地素材、运行数据库/图片/日志 |
| deploy/systemd、scripts | 后续设备服务、联调与诊断入口 |

## 工程约定

- Python目标版本≥3.11。pyproject仅描述可安装包，运行时依赖将在对应阶段锁定；Picamera2/GPIO等硬件依赖不能阻止mock启动。
- 配置文件相对路径按配置文件目录解析。示例的 `../data/edge.db` 指向edge/data。
- 本地覆盖使用config/local.yaml或环境变量，真实凭据、数据库、大视频和截图不进Git。
- 线上schema_version=1，时间为UTC；持续时间用单调时钟；读取故障保留状态和null，不能补0。
- edge与Go使用不同SQLite文件。后端运行在另一checkout/主机，通过MQTT和HTTP联调。
- 同一device_id只能有一个活跃边缘实例。新实例上线前关闭该设备的Go模拟器。

目录骨架可在edge工作目录执行导入检查（并非服务启动）：

```sh
PYTHONPATH=src python -c 'import piguard_edge; print(piguard_edge.__version__)'
```

接入后端时使用电脑的局域网IP，而不是树莓派自身的127.0.0.1。Go直接运行的默认HTTP只监听127.0.0.1，需要后端配置为可达监听地址；该配置在后端分支/运行环境处理。
