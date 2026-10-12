# 树莓派边缘端开发规划

## 1. 目标与开发边界

以 Raspberry Pi 4B、Raspberry Pi OS 64-bit 和 Python 为目标，先在开发电脑实现可重复的模拟边缘程序，再搬到树莓派，最后按已定型号替换真实驱动。引脚采用第 1.1 节的规划默认值，上板前对照实物确认。蜂鸣器和 LED 的型号尚未给出，不分配它们的引脚，也不写驱动。

边缘端负责四类传感器（距离、摄像头、陀螺仪、温度）、逻辑车速源、OpenCV 车道检测、四类风险规则、蜂鸣器/LED、本地持久化、MQTT 上下行。Go 平台负责设备登记、查询、存储、命令生命周期和 WebSocket。规则判断留在边缘端，平台断网时本地仍工作。

范围沿用课设的桌面/模型演示，不包含车辆刹车、转向和油门控制。第一版视觉任务是车道线与偏离，不增加目标识别模型。

成功标准分两级：

1. 软件闭环：六个确定性场景、四类本地预警、四种已支持的远程动作、遥测/状态/告警进入现有 Go 后端，断网和程序重启后可恢复缓存与命令去重。
2. 完整课设：增加实际图像处理、四类真实传感器、蜂鸣器/LED、树莓派服务运行；配置同步和图像平台闭环在对应后端能力完成后验收。蜂鸣器/LED 仍要等执行器型号。

## 1.1 已定硬件

| 感知 | 实物 | 驱动落点 | 总线 |
| --- | --- | --- | --- |
| 距离 | HC-SR04，配套 8 路电压转换模块 | `drivers/distance/hcsr04.py`，gpiozero | GPIO 脉冲。Trig/Echo 经电平转换 |
| 画面 | CSI 模块，传感器 OV5647IMXZ19 | `drivers/camera/ov5647.py`，Picamera2 | CSI。按 Camera v1 / libcamera `ov5647` |
| 角速度 | GY-521，MPU6050，6 轴 | `drivers/gyro/mpu6050.py`，smbus2 | 硬件 I2C1 |
| 车内温度 | AHT20+BMP280 模块中的 AHT20 | `drivers/temperature/aht20.py`，smbus2 | 硬件 I2C1，与陀螺仪共用 |

8 路电压转换模块按双向电平转换使用：高压侧 5V，低压侧 3.3V，与树莓派、HC-SR04 共地。只接 Trig 和 Echo。其余六路空置，I2C 不经过它。HC-SR04 的 VCC 接树莓派 5V；Echo 在确认转换模块高低压侧之后才接到模块上，不直接进 GPIO。

GY-521 与 AHT20+BMP280 优先用 3.3V 供电。上电前确认两块模块的 SDA/SCL 上拉不是 5V；若模块只有把上拉接到 VCC、而 VCC 又接了 5V，这组 I2C 不能接到树莓派。

两个 I2C 器件共用 `drivers/i2c_bus.py` 里的一把锁。AHT20 测量约需 80ms，等待期间释放锁，避免挡住 50Hz 的 MPU6050 读取。启动时探测地址：MPU6050 默认 `0x68`（十进制 104，AD0 拉高则为 `0x69`），AHT20 为 `0x38`（56），BMP280 先试 `0x76`（118），没有再试 `0x77`（119）。MPU6050 以 WHO_AM_I 等于 `0x68` 作为器件确认。BMP280 只用来确认模块在线上，它的温度、气压以及 AHT20 的湿度都留在本地诊断，不写入 `Sample`，也不增加 MQTT 字段。现有 telemetry 的 `temperature` 仍是一个摄氏温度，`gyro` 仍是 x/y/z 加 `yaw_rate`。

急转弯只用绕竖直轴的角速度。静止时用加速度判断哪一轴接近 1g，该轴就是竖直轴；若与配置的 `yaw_axis` 不一致，硬件检查失败，规则里不做静默对调。默认模块平放、`yaw_axis=z`、`yaw_sign=1`，量程 ±500 °/s，避免手持演示时 ±250 °/s 饱和。静止采集零偏后再输出。加速度不参与车速，角速度不积分成角度。

OV5647 使用 15 针 CSI 排线，分辨率 640×480，采集 10FPS，处理 5FPS。Picamera2 的采集配置使用 `BGR888`，`Frame.image` 与 OpenCV 录像同为 BGR。开发电脑继续用合成图和本地录像；`VideoCapture(0)` 只作为没有 CSI 时的备用输入。

HC-SR04 的 `raw_value` 是测得的米。桌面演示保持 `distance_scale=100`；传感器直接测量车距时改为 1。处理层只乘一次。量程按 0.02–4.00m，回波超时 30ms，超时则 `status=timeout`、值为空。10Hz 周期是 100ms，超时放在距离线程内。gpiozero 的 `DistanceSensor` 默认只测到 1m，驱动创建时必须传入配置中的 4.0。

规划默认接线如下。这些编号写进配置，驱动从配置读取。实物若已另接，改配置，不改规则。

| 信号 | BCM | 物理针 | 连接 |
| --- | --- | --- | --- |
| 5V | — | 2 | HC-SR04 VCC，电平转换模块 HV |
| 3.3V | — | 1 | 电平转换模块 LV；两个 I2C 模块的 VCC（在确认上拉电压之后） |
| GND | — | 6 | 传感器、转换模块、两个 I2C 模块共地 |
| SDA | 2 | 3 | MPU6050 与 AHT20/BMP280 |
| SCL | 3 | 5 | 同上 |
| Trig | 23 | 16 | 树莓派 → 转换模块 → HC-SR04 Trig |
| Echo | 24 | 18 | HC-SR04 Echo → 转换模块 → 树莓派 |
| CSI | — | CSI 座 | OV5647 排线，触点朝向按座子标记 |

树莓派配置启用 `i2c_arm` 和摄像头自动识别（或 `dtoverlay=ov5647`）。`gpiozero`、`smbus2` 放在可选依赖 `hardware`，Picamera2 用系统的 apt 包。mock 模式不导入这三项，开发电脑仍能启动。蜂鸣器与 LED 的 GPIO 保持未分配。

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

新分支为 `edge`，基点是 `main` 的 `e975d4b51d0dd5d42b9c1f1bb1e8ae0c1f0a6113`。选择 main 而非 Go 分支作为父分支，使本分支无需携带后端代码和其开发提交。

职责边界：边缘源码/配置/测试/设备部署全部放在 `edge/`。`docs/planning/PLAN.md` 是总规范，保存契约、硬件、阶段门槛和验收。按阶段可执行的实施计划放在 `docs/planning/implementation/`，当前一份是 `pc-edge-software.md`；后续阶段另写 `pN-短名.md`，不改写本文件的阶段定义。测试代码放在 `edge/tests/`，对应运行报告放在 `docs/verification/`，按 unit、contract、integration、hardware 分层，文件名 `pN-短名.md`。进度和下一步只放根目录 `HANDOFF.md`。课设方案与网络规范作为整体约束，放在 `docs/constraints/`。需要修改 Go 能力时切到后端分支完成，再由统一集成分支合并两端；不在边缘分支改 Go。

联调使用另一独立后端 checkout，或已运行的后端服务，通过 MQTT/HTTP 通信。当前 checkout 用于边缘开发。不要为联调将 go-backend 拷入 edge，不使用对方的数据库文件，不对同一 device_id 同时运行 Go 模拟器与 Python 边缘端，否则状态和遗嘱会互相覆盖。

```text
PiGuard/
├── README.md
├── HANDOFF.md
├── docs/
│   ├── constraints/          # 课设与网络交互的整体约束
│   │   ├── raspberry_pi_adas_course_design.md
│   │   └── adas_network_api_spec.md
│   ├── planning/
│   │   ├── PLAN.md           # 总规范：契约、硬件、阶段门槛、验收
│   │   └── implementation/   # 按阶段的可执行实施计划
│   │       ├── pc-edge-software.md
│   │       ├── p2-uplink.md
│   │       └── p3-local-warnings.md
│   └── verification/         # 测试运行报告，代码仍在 edge/tests
│       ├── README.md
│       ├── unit/
│       │   ├── p1-local-data-chain.md
│       │   ├── p2-uplink.md
│       │   └── p3-local-warnings.md
│       └── contract/
│           ├── p2-telemetry.md
│           └── p3-events.md
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

目录骨架在 P0 建立。P1 已实现模型、mock 驱动、六个场景、配置加载和 LatestState。P2 已实现遥测/状态编码、MQTT 会话和进程入口。P3 已实现四条规则、模拟执行器和事件缓存。P4 及之后点名的文件仍未实现。

## 4. 核心模型、职责与并发

内部 `Sample[T]` 保留 value（可为空）、UTC 采样时间、monotonic_ns、source、status。统一单位为 m、km/h、C、deg/s。驱动返回实际距离 raw_value，processing 只映射一次得到 mapped_value，保留比例；场景的道路距离先换算成模拟原始距离，避免重复放大。

用 DistanceSensor、CameraSource、GyroscopeSensor、TemperatureSensor、SpeedProvider 接口隔离型号。硬件库只由 HC-SR04、OV5647、MPU6050、AHT20 四个真实驱动加载，mock 模式在无 GPIO/Picamera2 的开发电脑也能启动。SpeedProvider 是逻辑数据源。陀螺仪驱动输出三轴角速度，处理层按配置取出 `yaw_rate`。

从线程与有界队列开始：每类采集 worker 独立更新 LatestState，规则 worker 读取快照，MQTT 网络循环独立，图像编码/HTTP 上传独立。摄像头阻塞或网络超时不能阻塞采样和输出。只有出现实测 CPU 瓶颈才把视觉处理移到独立进程。

| 工作过程 | 初始频率 | 限制 |
| --- | --- | --- |
| 距离/车速 | 10Hz | 读取超时有界 |
| 陀螺仪 MPU6050 | 50Hz | I2C 锁；静止零偏；`yaw_axis` 映射 |
| 温度 AHT20 | 1Hz | 测量等待约 80ms，等待时不占 I2C 锁 |
| 图像采集/车道分析 | 10FPS / 5FPS | OV5647 640×480；保留最新帧；驱动输出 BGR |
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

按单人开发估算，模拟软件主线约 10—15 个工作日。四类已定传感器的实机接入另计约 3 个工作日，蜂鸣器/LED 和平台未完成能力不含在这 3 天内。每阶段先完成可演示结果再扩大范围。

| 阶段 | 工作与主要目标文件 | 预计 | 完成门槛 |
| --- | --- | --- | --- |
| P0 工程骨架 | pyproject、目录、配置示例、规划和开发入口 | 0.5天 | 分支仅有共享文档与 edge；包导入、配置/结构静态检查通过 |
| P1 本地数据链 | models/sample.py、drivers/base.py、simulation/engine.py/scenarios.py、runtime/state.py/workers.py、config/loader.py | 2天 | 无硬件运行六个可重复场景，所有源共享一个场景时钟；错误不变成0；停止场景恢复正常采样。加载器接受 `hardware` 配置段，mock 驱动不读取它 |
| P2 上行最小闭环 | protocol/telemetry.py/status.py、transport/mqtt.py、runtime/app.py、CLI入口 | 1—2天 | Python→真实Broker→Go→HTTP/WS；2Hz遥测、在线/遗嘱、故障/null、TTL、seq重启和重连通过；规则未完成时risk=unknown |
| P3 本地预警与事件 | processing/distance.py/gyro.py、rules/engine.py及四规则、actuators/manager.py/mock.py、storage/events.py、protocol/event.py | 2—3天 | 四种危险及恢复可复现，只在状态改变时发事件；断网时规则/模拟输出不断，重连同ID补传且平台只存一条 |
| P4 下行真实软件行为 | commands/handler.py、storage/commands.py、protocol/command.py、执行器仲裁 | 2天 | 四类命令改变真实模拟状态/输出，HTTP查询与双WS可见Ack；重复、过期、非法、重启及执行中崩溃场景通过 |
| P5 视觉边缘计算 | drivers/camera/synthetic.py/video.py、processing/lane.py、Frame/LaneResult | 2—3天 | 合成道路和本地录像实际经过ROI/灰度/模糊/Canny/Hough；左右线缺失为invalid；偏移方向、持续阈值、视频结束与释放资源通过 |
| P6 真实驱动 | `drivers/i2c_bus.py`、`hcsr04.py`、`ov5647.py`、`mpu6050.py`、`aht20.py`；配置中的地址和 BCM | 约 3 天，树莓派在手之后 | 一次只换一路，其余保持 mock。顺序和通过条件见下文 |
| P7 设备部署与完整联调 | deploy/systemd/piguard-edge.service、设备安装/诊断脚本、树莓派配置 | 1—2天，硬件可用后 | 开机启动、崩溃重启、断网启动、正常停止释放输出；Pi4B实测频率、温度/内存、30分钟运行和缓存恢复 |

P6 在树莓派上按下面的顺序做。每一路通过后才接下一路线，失败的那一路回到 mock。蜂鸣器和 LED 不在这个顺序里。

1. 未接 Echo 之前，先确认电平转换模块的 HV 为 5V、LV 为 3.3V，并确认两块 I2C 模块的时钟数据线不是 5V。然后 `i2cdetect -y 1` 应看到 `0x38`、`0x68`，以及 `0x76` 或 `0x77`。
2. AHT20：1Hz 读出摄氏温度；手温能使读数上升。报文里的温度只来自这一路。BMP280 在线即可，它的温度不进入 `Sample`。
3. MPU6050：静止零偏后 `yaw_rate` 接近 0。绕竖直轴转动时，配置指定的轴变号，量程内不饱和。静止时约 1g 的加速度轴必须与 `yaw_axis` 一致，否则停止这一路并改配置，不在规则里对调。
4. HC-SR04：Echo 经转换模块进入 BCM24。正对平面目标时，0.20m 与 1.00m 的 `raw_value` 误差在 ±0.03m 内；遮住探头或超出 4m 为 timeout，值为空。桌面演示的 `mapped_value` 仍按 scale 100。
5. OV5647：Picamera2 按 `BGR888` 输出 640×480。排线触点方向错误会表现为设备不存在，先断电再重插。这一路通过后再把画面送进 P5 的车道处理。

拔出任一 I2C 模块或断开 Echo 时，对应 `Sample.status` 变为 `unavailable` 或 `timeout`，其他传感器继续采样。进程退出时释放 GPIO、关闭摄像头并放开 I2C。

P1/P3 的车道结果先由场景提供模拟 LaneResult；P5 再让合成图像与录像实际经过 OpenCV，不能将前面的模拟结果计为图像算法通过。OV5647 实拍进入同一条处理函数，不另写一套车道算法。

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

- unit：fake clock 驱动阈值、持续时间、恢复、过滤、映射、场景一致性和优先级；不依赖硬件或网络。报告放 `docs/verification/unit/`。
- contract：逐条对齐此基线的 Go 解析器，覆盖完整/故障telemetry、在线/遗嘱status、四类command/Ack、事件恢复与非法字段；以 Go 实际接受/拒绝结果建立黄金样例，避免两端自测互相误解。报告放 `docs/verification/contract/`。
- integration：Python+实际Broker+Go独立服务+HTTP/双WS；覆盖断网、平台/Broker/设备重启、同ID补传、命令去重及磁盘/数据库异常。报告放 `docs/verification/integration/`。
- hardware：在树莓派上按 P6 的五步执行，记录 I2C 地址、零偏、竖直轴、HC-SR04 误差、Echo 超时、OV5647 分辨率与 BGR、GPIO/摄像头释放。mock 通过不算硬件通过。执行器型号未定，不测蜂鸣器和 LED。报告放 `docs/verification/hardware/`。

同一阶段、同一层只保留最新一份报告。P1 单元结果见 `docs/verification/unit/p1-local-data-chain.md`。P2 软件结果见 `docs/verification/unit/p2-uplink.md` 与 `docs/verification/contract/p2-telemetry.md`。P3 软件结果见 `docs/verification/unit/p3-local-warnings.md` 与 `docs/verification/contract/p3-events.md`。运行时依赖已锁定 PyYAML==6.0.3 与 paho-mqtt==2.1.0，开发依赖锁定 pytest==9.1.1。真实 Broker 联调尚未执行。
