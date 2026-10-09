# 树莓派边缘端开发规划

## 1. 目标与开发边界

以 Raspberry Pi 4B、Raspberry Pi OS 64-bit 和 Python 为目标，先在开发电脑实现可重复的模拟边缘程序，再搬到树莓派，最后逐个替换真实驱动。硬件型号尚未确定，不提前固定引脚、供电方案或具体驱动。

边缘端负责四类传感器（距离、摄像头、陀螺仪、温度）、逻辑车速源、OpenCV 车道检测、四类风险规则、蜂鸣器/LED、本地持久化、MQTT 上下行。Go 平台负责设备登记、查询、存储、命令生命周期和 WebSocket。规则判断留在边缘端，平台断网时本地仍工作。

范围沿用课设的桌面/模型演示，不包含车辆刹车、转向和油门控制。第一版视觉任务是车道线与偏离，不增加目标识别模型。

成功标准分两级：

1. 软件闭环：六个确定性场景、四类本地预警、四种已支持的远程动作、遥测/状态/告警进入现有 Go 后端，断网和程序重启后可恢复缓存与命令去重。
2. 完整课设：增加实际图像处理、四类真实传感器、蜂鸣器/LED、树莓派服务运行；配置同步和图像平台闭环在对应后端能力完成后验收。

## 2. 阅读依据与后端事实

已阅读课程设计、网络规范、三项 Go capability 的规格、归档 brief/验收报告，并核对后端入口、协议、服务、仓储、MQTT、模拟器和相关测试。

代码核验基线是本地 `go后端开发` 的 `0e65a536a3fe01ae4a866bbe91b695ea25f3623b`。以下代码路径均属于该分支；本边缘分支不复制这些文件。后端后续改变时，应重新核对契约基线。

| 能力 | 实际状态 | 边缘端的安排 |
| --- | --- | --- |
| MQTT telemetry/status | 已实现身份、版本、采样元数据校验 | 第一轮直接接入，使用已登记的 car-001 |
| 最新状态、历史遥测、设备查询 | 已实现；最新按 timestamp/seq 排序，历史每设备最多 1Hz | 2Hz 上报，不用历史条数判断丢包 |
| WebSocket | 已实现 telemetry、device_status、command_update、event | 用测试 WS 客户端验收，不等待 Vue 开发 |
| 动作命令/Ack | 已实现 buzzer.test、indicator.test、scenario.start、scenario.stop | 第一版真实实现这四种动作 |
| events | 已实现校验、全局 event_id 幂等、查询、推送 | 边缘规则生成事件并持久缓存 |
| 配置查询/更新/确认 | GET/PUT 返回 501，未订阅 config-acks | 第一版使用本地 YAML；远程同步留作配套阶段 |
| 图片上传/最新帧/图片内容 | 返回 501；camera.snapshot 被命令校验拒绝 | 先本地处理和保存，平台闭环待后端补齐 |
| Go 模拟器 | 固定遥测、演示事件、命令解析和模拟 Ack | 作为协议参考；不替代 Python 场景、规则或执行器 |
| 真实部署与鉴权 | 开发 Broker 匿名、persistence false；HTTP 无鉴权 | 隔离局域网联调，后续单独完善凭据/ACL/保留策略 |

关键代码证据：

- `go-backend/internal/protocol/telemetry.go:110`：ParseTelemetry；`:144`：ParseStatus；`:215`：View/TTL。
- `go-backend/internal/service/monitor.go:96`：timestamp/seq 顺序；`:124`：历史限频。
- `go-backend/internal/protocol/command.go:80`：四种动作参数；`:141`：ParseCommandAck。
- `go-backend/internal/service/command.go:100`：10 秒截止；`:167`：以平台接收时间处理 Ack。
- `go-backend/internal/simulator/command.go:28`：仅模拟执行，去重缓存不持久化。
- `go-backend/internal/protocol/event.go:79`：ParseEvent；`internal/repo/event.go:34`：唯一键插入。
- `go-backend/internal/mqtt/subscriber.go:80`：事件接入没有应用层入库回执。
- `go-backend/internal/service/config.go:28`、`internal/service/frame.go:28`、`internal/repo/snapshot.go:26`：配置/图片占位。
- `go-backend/config.yaml:15`：默认 HTTP 127.0.0.1:8080；`deploy/mosquitto.conf:1`：开发 Broker 配置。

后端 README 第一段仍称告警未实现，但最新事件代码和 2026-10-09 归档报告已证明软件路径完成，以上表格以代码为准。历史报告记载 Go test/race/vet 通过；本次规划不把历史验收当成本机新运行或真实硬件的验收证据。命令和事件最新归档未执行真实 Broker/Docker/硬件联调。

## 3. Git 与目录隔离

新分支为 `codex/raspberry-pi-edge`，基点是 `main` 的 `e975d4b51d0dd5d42b9c1f1bb1e8ae0c1f0a6113`。选择 main 而非 Go 分支作为父分支，使本分支无需携带后端代码和其开发提交。

职责边界：边缘源码/配置/测试/设备部署全部放在 `edge/`；永久规划放 `docs/PLAN.md`；进度和下一步只放根目录 `HANDOFF.md`。共享课设文档保持独立。需要修改 Go 能力时切到后端分支完成，再由统一集成分支合并两端；不在边缘分支改 Go。

联调使用另一独立后端 checkout，或已运行的后端服务，通过 MQTT/HTTP 通信。当前 checkout 用于边缘开发。不要为联调将 go-backend 拷入 edge，不使用对方的数据库文件，不对同一 device_id 同时运行 Go 模拟器与 Python 边缘端，否则状态和遗嘱会互相覆盖。

```text
PiGuard/
├── README.md
├── HANDOFF.md
├── docs/
│   ├── PLAN.md
│   ├── raspberry_pi_adas_course_design.md
│   └── adas_network_api_spec.md
└── edge/
    ├── README.md
    ├── pyproject.toml
    ├── .gitignore
    ├── config/default.example.yaml
    ├── src/piguard_edge/
    │   ├── config/       # 加载、校验、配置快照
    │   ├── models/       # Sample、Frame、LaneResult、内部状态
    │   ├── protocol/     # v1 JSON 编解码、Topic 与契约校验
    │   ├── drivers/
    │   │   ├── distance/
    │   │   ├── camera/
    │   │   ├── gyro/
    │   │   ├── temperature/
    │   │   └── speed/
    │   ├── simulation/  # 统一场景时钟与六个场景
    │   ├── processing/  # 距离滤波映射、陀螺仪、车道处理
    │   ├── rules/       # 四类状态机及风险汇总
    │   ├── actuators/   # Mock/真实输出与优先级仲裁
    │   ├── commands/    # 参数/有效期校验、调度、幂等与 Ack
    │   ├── transport/   # MQTT、重连、HTTP 图像/事件确认
    │   ├── storage/     # 独立 SQLite、事件与命令记录
    │   └── runtime/     # Workers、LatestState、启动和关闭
    ├── tests/
    │   ├── unit/
    │   ├── contract/
    │   ├── integration/
    │   └── hardware/
    ├── assets/          # 本地道路素材；大视频不进 Git
    ├── scripts/         # 后续联调与设备诊断入口
    ├── deploy/systemd/
    └── data/            # edge.db/缓存/日志，运行数据不进 Git
```

目录已用包标记和说明文件建立；下面提到的具体业务文件是后续实施目标，不表示已经实现。

## 4. 核心模型、职责与并发

内部 `Sample[T]` 保留 value（可为空）、UTC 采样时间、monotonic_ns、source、status。统一单位为 m、km/h、C、deg/s。驱动返回实际距离 raw_value，processing 只映射一次得到 mapped_value，保留比例；场景的道路距离先换算成模拟原始距离，避免重复放大。

用 DistanceSensor、CameraSource、GyroscopeSensor、TemperatureSensor、SpeedProvider 接口隔离型号。硬件库只由真实驱动加载，mock 模式在无 GPIO/Picamera2 的开发电脑也能启动。SpeedProvider 是逻辑数据源，不把陀螺仪当车速传感器。

从线程与有界队列开始：每类采集 worker 独立更新 LatestState，规则 worker 读取快照，MQTT 网络循环独立，图像编码/HTTP 上传独立。摄像头阻塞或网络超时不能阻塞采样和输出。只有出现实测 CPU 瓶颈才把视觉处理移到独立进程。

| 工作过程 | 初始频率 | 限制 |
| --- | --- | --- |
| 距离/车速 | 10Hz | 读取超时有界 |
| 陀螺仪 | 50Hz | 有校准和轴向映射 |
| 温度 | 1Hz | 按实际模块能力调整 |
| 图像采集/车道分析 | 10FPS / 5—10FPS | 保留最新帧，丢弃积压旧帧 |
| RuleEngine | 10Hz | 单调时钟计时，不调用网络 |
| telemetry/status | 2Hz / 状态变化加10s心跳 | 遥测仅发送最新数据 |
| 图片预览（后续） | 1—2FPS | 有界、可丢弃，与事件队列分开 |

第一版不缓存并补传高频遥测和预览。事件、配置和命令结果要持久化，避免网络离线造成无限内存堆积。关闭时停止受理动作、取消测试输出、关闭摄像头/GPIO/网络并释放数据库；网络连接失败仍启动本地规则。

## 5. 与已完成 Go 后端对齐的契约

### 遥测和在线状态

- Topic 使用 `car/{device_id}/...`；初始设备 `car-001`，主题与 payload 的 device_id 完全一致，schema_version=1。
- telemetry 带 message_id、UTC timestamp、非负递增 seq，以及 speed/distance/temperature/gyro/lane/risk 全部嵌套对象。不得把整个故障传感器对象省略。
- 距离字段使用 `raw_value`、`mapped_value`、`scale`，不使用课设早期示例的 raw/mapped。各采样带 source/status/unit/sample_at；gyro 明确包含 yaw_rate；lane 包含 valid/offset_ratio/direction/confidence/sample_at。
- status 枚举只有 ok/timeout/unavailable/error。stale 是根据时间计算的新鲜度，不新增为线上 status 枚举。读取失败数值设为 null；无车道时 valid=false、direction=unknown，不能假造偏移 0。
- speed/distance/gyro/lane TTL 为 1 秒，温度为 5 秒；内部规则同样检查样本年龄，重复上报不能刷新旧样本的 sample_at。
- telemetry QoS0、retain=false；status QoS1、retain=true；连接前设置无 timestamp 的 offline 遗嘱。连接/重连后重新订阅自己的 commands 并发布完整 online status；正常退出主动发布 offline。
- online=true 的 status 必须带 timestamp、software_version、config_version、mode、四个 sensors 状态和 mqtt.connected。mode 仅 mock/hardware/replay；即便混用驱动也不发 hybrid。初始 config_version=0，不因本地 YAML 或平台种子 desired_version=1 冒充配置已同步。
- UTC 用于线上时间；规则持续时间用 monotonic。Go 按 timestamp 优先、同时间 seq 比较，时钟回拨不能靠 seq 重置解决；树莓派上配置时间同步，记录偏差，时钟异常时继续本地工作并对外暴露诊断。

### 命令与执行器

| 命令 | params | 执行完成的定义 |
| --- | --- | --- |
| buzzer.test | 整数 duration_ms，100—5000 | 测试输出完成，或明确返回忙/故障 |
| indicator.test | color=green/yellow/red，duration_ms 同上 | 测试输出完成，或明确返回忙/故障 |
| scenario.start | 六个场景之一，speed=0.5/1.0/2.0 | 场景切换完成；speed 指播放倍率，不是车辆 km/h |
| scenario.stop | 严格空对象 {} | mock 模式恢复 normal_drive，持续采样和预警 |

平台下发有效期统一 10 秒。设备校验 schema/身份/类型/参数/issued_at/expires_at，并在真正开始动作前重新检查剩余时间。动作无法在剩余窗口内完成时返回明确失败，不盲目开始。

命令状态与完整 Ack 存 SQLite。先持久记录已受理 ID 再触发输出；已完成 ID 只重发相同结果，执行时间也不改变。相同 ID 的不同内容按冲突拒绝，禁止重做动作。启动发现执行中记录时关闭残留测试输出并标记结果不确定，不自动重做；业务结果与物理动作无法通过 SQLite 原子提交，不能声称任意断电位置都恰好执行一次。

Ack 必须有 schema_version、command_id、device_id、status=success/failed、UTC executed_at；成功带对象 result，失败带 error.code/message，不能同时带两者。QoS1、retain=false。平台是否超时依据收到 Ack 的时间，不依据设备报告的 executed_at。

自动危险/警告输出优先于人工测试。测试不会压低或覆盖正在发生的预警；冲突时返回 DEVICE_BUSY，或停止测试并给出明确结果。采集和规则不因最长 5 秒测试而暂停。硬件模式中的场景动作返回 DEVICE_NOT_SUPPORTED，避免远程切换模拟源覆盖真实感知。

### 告警及可靠性

事件 type 固定为 obstacle_warning/lane_departure/sharp_turn/high_temperature/sensor_failure；action 为 started/level_changed/recovered，level 只为 warning/danger。recovered 保留恢复前等级。每次状态转换生成新的 UUID event_id，重复发送时 ID、时间和内容保持不变，不添加未定义的顶层字段或 incident_id。

先写 edge.db，再异步发送 QoS1、retain=false。data 是 JSON 对象；截图字段暂不发送，不能伪造 snapshot_id。telemetry.active_events 表达本地当前激活的规则类型，不依赖它在后端生成 events。

现有后端没有 events-acks，PUBACK 只确认 Broker 接收，不等于后端 SQLite 入库。兼容第一版采用两步确认：先记 broker_confirmed，再通过已有 events HTTP 查询按 type 和事件 timestamp 窗口查找相同 event_id，核对持久字段后记 platform_confirmed。查不到、HTTP 失败、窗口超过最大1000条或内容冲突时保持未确认；重试有退避且不改原始事件。确认查询失败不会阻塞本地预警。

没有平台确认的事件不得自动删除；已确认事件可按保留策略回收。设置缓存容量诊断和导出/人工清理入口，明确磁盘满或数据库故障的持久化失败，不能把数据静默丢弃伪装成成功。后续可在 Go 分支单独增加应用层事件确认以替代 HTTP 查询，第一版不擅自发明确认 Topic。

## 6. 分阶段实施与验收

按单人开发估算，模拟软件主线约 10—15 个工作日；具体硬件接入和平台未完成能力另计，以实际型号和环境为准。每阶段先完成可演示结果再扩大范围。

| 阶段 | 工作与主要目标文件 | 预计 | 完成门槛 |
| --- | --- | --- | --- |
| P0 工程骨架 | pyproject、目录、配置示例、规划和开发入口 | 0.5天 | 分支仅有共享文档与 edge；包导入、配置/结构静态检查通过 |
| P1 本地数据链 | models/sample.py、drivers/base.py、simulation/engine.py/scenarios.py、runtime/state.py/workers.py、config/loader.py | 2天 | 无硬件运行六个可重复场景，所有源共享一个场景时钟；错误不变成0；停止场景恢复正常采样 |
| P2 上行最小闭环 | protocol/telemetry.py/status.py、transport/mqtt.py、runtime/app.py、CLI入口 | 1—2天 | Python→真实Broker→Go→HTTP/WS；2Hz遥测、在线/遗嘱、故障/null、TTL、seq重启和重连通过；规则未完成时risk=unknown |
| P3 本地预警与事件 | processing/distance.py/gyro.py、rules/engine.py及四规则、actuators/manager.py/mock.py、storage/events.py、protocol/event.py | 2—3天 | 四种危险及恢复可复现，只在状态改变时发事件；断网时规则/模拟输出不断，重连同ID补传且平台只存一条 |
| P4 下行真实软件行为 | commands/handler.py、storage/commands.py、protocol/command.py、执行器仲裁 | 2天 | 四类命令改变真实模拟状态/输出，HTTP查询与双WS可见Ack；重复、过期、非法、重启及执行中崩溃场景通过 |
| P5 视觉边缘计算 | drivers/camera/synthetic.py/video.py、processing/lane.py、Frame/LaneResult | 2—3天 | 合成道路和本地录像实际经过ROI/灰度/模糊/Canny/Hough；左右线缺失为invalid；偏移方向、持续阈值、视频结束与释放资源通过 |
| P6 真实驱动 | USB/CSI摄像头、实际距离/陀螺仪/温度驱动、GPIO输出 | 型号确定后估算 | 逐个真实源与mock其余源联调；确认电压/电平/接线、距离比例、陀螺轴向/零偏、超时、拔出和恢复 |
| P7 设备部署与完整联调 | deploy/systemd/piguard-edge.service、设备安装/诊断脚本、树莓派配置 | 1—2天，硬件可用后 | 开机启动、崩溃重启、断网启动、正常停止释放输出；Pi4B实测频率、温度/内存、30分钟运行和缓存恢复 |

P1/P3 的车道结果先由场景提供模拟 LaneResult；P5 再让合成图像与录像实际经过 OpenCV，不能将前面的模拟结果计为图像算法通过。

P2/P3/P4 的软件验证先使用 fake clock、fake transport 和临时 SQLite；真实 Broker 检查在联调环境可用时单独执行并保存结果。Go 模拟器只用于对照 payload，同一设备联调时关闭它。

### 规则首版参数与验收样例

| 规则 | 首版判定 | 必须检查 |
| --- | --- | --- |
| 障碍物 | 距离≤15m警告、≤8m危险；速度有效且>0时计算 headway=distance/(speed/3.6)，<1.5s警告、<0.8s危险，取两者较高等级 | 距离比例仅一次；静止不除零；距离故障不作正常；速度无效时仍可用有效距离规则 |
| 车道偏离 | abs(offset_ratio)>0.35持续≥1000ms | 左右符号、一侧/无车道不判正常、无效/过期样本打断连续计时 |
| 急转弯 | abs(yaw_rate)>30deg/s且speed>20km/h，持续≥300ms | 不把陀螺当车速；低速不触发；抖动与缺失中断计时 |
| 高温 | ≥35C持续≥5000ms；≤33C持续≥5000ms解除 | 触发/恢复滞回，阈值附近不反复生成事件 |

规则共用 NORMAL/PENDING/WARNING或DANGER/RECOVERING 状态机。无效样本暂停风险推断并触发一次 sensor_failure；恢复有效采样后重新累计持续时间，不用上一个危险值永久报警，也不把故障当正常解除。综合 risk 取有效告警最高等级；故障由 sensor_failure 与采样状态表达，无有效判断依据时为 unknown。

## 7. 后续平台配套阶段

这些是完整课设的明确依赖，不阻塞 P1—P5，也不在边缘分支修改 Go：

1. 配置同步：Go 分支完成配置 GET/PUT、参数完整性/并发版本校验、retained config 与 config-acks；确定全量或补丁语义及 Broker 重启重发。边缘再实现版本拒旧、完整校验、持久化、原子替换规则快照、Ack 和 reported version。无效配置保持上一版。
2. 图片闭环：Go 分支完成 JPEG 上传与读取、frame_id 去重、事件/命令关联及 camera.snapshot 支持；边缘再实现1—2FPS预览和单次截图。截图Ack只能在上传成功获得真实snapshot_id后返回。当前只保存本地调试图。
3. 部署完善：确定 HTTP/MQTT 凭据、每设备 ACL、TLS需求、Broker持久化、平台/边缘数据保留和启动就绪语义。凭据放环境或本地配置，不入Git。

## 8. 验证组织与下一次开发入口

- unit：fake clock 驱动阈值、持续时间、恢复、过滤、映射、场景一致性和优先级；不依赖硬件或网络。
- contract：逐条对齐此基线的 Go 解析器，覆盖完整/故障telemetry、在线/遗嘱status、四类command/Ack、事件恢复与非法字段；以 Go 实际接受/拒绝结果建立黄金样例，避免两端自测互相误解。
- integration：Python+实际Broker+Go独立服务+HTTP/双WS；覆盖断网、平台/Broker/设备重启、同ID补传、命令去重及磁盘/数据库异常。
- hardware：按实购型号在Pi执行，记录量程、采样误差、轴向、驱动超时、GPIO释放和资源负载，不把mock成功记成硬件通过。

第一笔业务开发从 P1 开始：Sample/Frame/LaneResult → 驱动接口与六个场景 → 配置校验与LatestState/worker → fake-clock验证；完成后才接P2网络。Python运行时依赖在相应阶段核对官方/Context7文档并锁定，当前骨架没有安装或接入这些库。
