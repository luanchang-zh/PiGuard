# 基于树莓派 4B 的简易车载驾驶辅助预警系统设计方案

> 课程：物联网与边缘计算  
> 项目类型：课程设计 / 原型系统  
> 核心平台：Raspberry Pi 4B  
> 开发原则：先以统一接口 + 模拟数据跑通全链路，后续确定硬件型号后仅替换传感器驱动层。  
> 目标：满足感知层、网络层、平台层、应用层四层架构，传感器数量大于 3，并支持平台向边缘设备下发控制命令。

---

# 1. 项目概述

本项目设计一个基于 Raspberry Pi 4B 的简易车载驾驶辅助预警系统。

系统包含以下四类传感器：

1. 超声波传感器：测量前方障碍物距离；
2. 摄像头：采集前方道路画面，用于简易车道偏离检测；
3. 陀螺仪：测量车辆转动角速度，用于辅助判断急转弯状态；
4. 温度传感器：测量车内环境温度。

树莓派作为边缘计算节点，完成：

- 传感器数据采集；
- 模拟数据生成；
- 数据格式统一；
- 数据滤波与比例映射；
- OpenCV 图像处理；
- 本地预警判断；
- 蜂鸣器 / LED 本地告警；
- MQTT 数据上报；
- 接收平台下发命令；
- 断网本地继续运行；
- 告警事件本地缓存与恢复后补传。

平台端完成：

- 设备管理；
- 遥测数据接收；
- 告警事件记录；
- 控制命令管理；
- 参数配置；
- 历史数据存储；
- WebSocket 实时数据推送。

应用层采用 Web 页面实现监控、告警查看和设备控制。

---

# 2. 项目功能目标

## 2.1 前方障碍物预警

使用超声波传感器获取前方障碍物距离。

系统根据：

- 前方距离；
- 模拟车速；

判断危险等级。

示例：

```text
距离 > 15 m
NORMAL

8 m < 距离 <= 15 m
WARNING

距离 <= 8 m
DANGER
```

发生危险时：

```text
树莓派本地判断
   ↓
蜂鸣器 / LED 告警
   ↓
MQTT 上传事件
   ↓
Go 后端记录
   ↓
网页实时显示
```

---

## 2.2 车道偏离预警

摄像头采集车辆前方道路画面。

树莓派使用 OpenCV 完成：

```text
图像采集
  ↓
裁剪道路 ROI
  ↓
灰度化
  ↓
Gaussian Blur
  ↓
Canny 边缘检测
  ↓
HoughLinesP
  ↓
左右车道线筛选
  ↓
计算车道中心
  ↓
计算车辆相对车道中心偏移
  ↓
判断是否发生车道偏离
```

例如：

```text
abs(offset_ratio) > 0.35
持续 1 秒
```

触发：

```text
LANE_DEPARTURE_WARNING
```

---

## 2.3 急转弯预警

陀螺仪用于获取车辆绕竖直方向的角速度。

例如：

```text
abs(yaw_rate) > 30°/s
且
speed > 20 km/h
持续超过 300 ms
```

则触发：

```text
SHARP_TURN_WARNING
```

注意：

陀螺仪测量的是角速度，并不能直接可靠测量车辆线速度。

因此本项目额外设计一个软件逻辑接口：

```text
SpeedProvider
```

当前阶段由模拟场景提供车速。

后续可以替换为：

```text
GPS
轮速编码器
OBD
CAN
```

该接口属于逻辑数据源，不计入第五种必选物理传感器。

---

## 2.4 车内高温预警

温度传感器用于测量车内环境温度。

例如：

```text
temperature >= 35°C
持续 5 秒
```

触发：

```text
HIGH_TEMPERATURE_WARNING
```

为了避免阈值附近抖动：

```text
触发阈值：35°C
解除阈值：33°C
```

---

## 2.5 向下控制

平台必须支持向树莓派主动下发控制命令。

第一版支持：

```text
config.update

buzzer.test

indicator.test

camera.snapshot

scenario.start

scenario.stop
```

形成完整闭环：

```text
Vue
 ↓ HTTP
Go Backend
 ↓ MQTT
Raspberry Pi
 ↓
Command Handler
 ↓
执行器 / 配置 / 场景引擎
 ↓
Command Ack
 ↓ MQTT
Go Backend
 ↓ WebSocket
Vue
```

---

# 3. 四层物联网架构

系统按照课程要求分为：

1. 感知层；
2. 网络层；
3. 平台层；
4. 应用层。

整体架构：

```text
┌──────────────────────────────────────────────┐
│                  应用层                       │
│                                              │
│ Vue 3 + TypeScript + Element Plus + ECharts │
│                                              │
│ 实时监控 / 告警记录 / 参数配置 / 设备控制      │
└──────────────────────┬───────────────────────┘
                       │
                HTTP + WebSocket
                       │
┌──────────────────────▼───────────────────────┐
│                  平台层                       │
│                                              │
│ Go + Gin                                     │
│                                              │
│ Device Service                               │
│ Telemetry Service                            │
│ Event Service                                │
│ Command Service                              │
│ Config Service                               │
│                                              │
│ SQLite + MQTT Client                         │
│                                              │
│ Eclipse Mosquitto MQTT Broker                │
└──────────────────────┬───────────────────────┘
                       │
                  MQTT / HTTP
                       │
              Wi-Fi / Ethernet
                       │
┌──────────────────────▼───────────────────────┐
│             感知层 + 边缘计算                 │
│                                              │
│ Raspberry Pi 4B                              │
│                                              │
│ ┌─────────┐ ┌────────┐ ┌────────┐ ┌──────┐ │
│ │超声波   │ │摄像头  │ │陀螺仪  │ │温度  │ │
│ └────┬────┘ └───┬────┘ └───┬────┘ └──┬───┘ │
│      │          │          │         │       │
│      └──────────┴────┬─────┴─────────┘       │
│                     ↓                        │
│              Sensor Adapter                  │
│                     ↓                        │
│              Data Processing                 │
│                     ↓                        │
│                Rule Engine                   │
│              ↙              ↘                │
│      Buzzer / LED          MQTT Upload       │
└──────────────────────────────────────────────┘
```

---

# 4. 四层具体职责

## 4.1 感知层

包含：

```text
超声波传感器
摄像头
陀螺仪
温度传感器
蜂鸣器
LED
Raspberry Pi 4B
```

其中树莓派承担边缘计算功能。

---

## 4.2 网络层

采用：

```text
Wi-Fi / Ethernet
```

应用协议：

```text
MQTT
HTTP
WebSocket
```

其中：

- MQTT：设备遥测、告警、控制命令；
- HTTP：图片上传、前端 API；
- WebSocket：平台向网页实时推送数据。

---

## 4.3 平台层

包含：

```text
Mosquitto

Go Backend

SQLite
```

承担：

```text
设备管理
数据存储
告警管理
控制命令
配置同步
图片管理
```

---

## 4.4 应用层

采用：

```text
Vue Web 页面
```

支持：

```text
PC 浏览器
手机浏览器
```

第一版不额外开发 App 或小程序。

使用响应式 Web 即可满足应用层需求。

---

# 5. 系统部署方案

课设阶段推荐三个角色。

## Raspberry Pi 4B

运行：

```text
Python Edge Program
OpenCV
Sensor Driver
MQTT Client
SQLite Edge Cache
```

---

## 开发电脑

运行：

```text
Mosquitto
Go Backend
SQLite
Vue
```

可进一步使用：

```text
Docker Compose
```

统一启动。

---

## 手机 / PC 浏览器

访问：

```text
http://平台IP:8080
```

即可查看车辆状态并控制设备。

系统可以完全运行在局域网，不依赖公网。

---

# 6. 技术栈

## 6.1 边缘端

| 模块 | 技术 |
|---|---|
| 硬件 | Raspberry Pi 4B |
| 系统 | Raspberry Pi OS 64-bit |
| 语言 | Python 3 |
| 图像处理 | OpenCV |
| CSI 摄像头 | Picamera2 |
| USB 摄像头 | OpenCV VideoCapture |
| MQTT | paho-mqtt |
| GPIO | gpiozero / 对应硬件库 |
| 配置 | YAML |
| 本地缓存 | SQLite |
| 服务管理 | systemd |

---

## 6.2 平台后端

| 模块 | 技术 |
|---|---|
| 语言 | Go |
| Web 框架 | Gin |
| ORM | GORM |
| 数据库 | SQLite |
| MQTT | Eclipse Paho Go |
| 实时推送 | WebSocket |
| 部署 | Docker Compose |

---

## 6.3 前端

| 模块 | 技术 |
|---|---|
| 框架 | Vue 3 |
| 语言 | TypeScript |
| 构建 | Vite |
| UI | Element Plus |
| 图表 | ECharts |
| HTTP | Axios |
| 实时数据 | WebSocket |

---

# 7. 为什么采用 Python + Go

树莓派部分推荐 Python：

```text
硬件驱动方便
OpenCV 生态成熟
Picamera2 官方支持较好
开发速度快
```

平台端采用 Go：

```text
并发处理 MQTT / HTTP 简单
性能足够
部署方便
适合设备平台服务
```

形成：

```text
Python：边缘计算 + 硬件

Go：平台服务

Vue：应用层
```

---

# 8. 传感器接口设计

目前传感器型号未确定。

因此不能写：

```text
业务逻辑直接调用某 GPIO
```

必须设计：

```text
业务逻辑
    ↓
Sensor Interface
    ↓
Driver
```

例如：

```text
DistanceSensor
│
├── MockDistanceSensor
└── HardwareDistanceSensor
```

以后更换型号只修改 Driver。

---

# 9. 通用采样结构

建议：

```python
from dataclasses import dataclass
from typing import Generic, Literal, TypeVar

T = TypeVar("T")

Source = Literal[
    "mock",
    "replay",
    "hardware",
    "estimated"
]

Status = Literal[
    "ok",
    "timeout",
    "unavailable",
    "error"
]

@dataclass
class Sample(Generic[T]):
    value: T | None

    timestamp_ms: int

    monotonic_ns: int

    source: Source

    status: Status
```

---

# 10. 为什么需要 status

不能写：

```text
传感器读取失败
value = 0
```

因为：

```text
0 km/h
```

可以代表车辆停止。

```text
0°/s
```

可以代表没有转动。

因此异常必须使用：

```text
status = timeout

status = unavailable

status = error
```

---

# 11. 超声波接口

```python
class DistanceSensor:

    def read(self) -> Sample[float]:
        ...
```

统一单位：

```text
meter
```

例如：

```json
{
  "value": 0.32,
  "source": "hardware",
  "status": "ok"
}
```

---

# 12. 摄像头接口

```python
class CameraSource:

    def read(self) -> Sample["Frame"]:
        ...
```

Frame：

```python
@dataclass
class Frame:

    image: object

    frame_id: int

    width: int

    height: int
```

实现：

```text
CameraSource

├── SyntheticCamera
├── VideoCamera
├── USBCamera
└── PiCamera
```

---

# 13. 陀螺仪接口

```python
@dataclass
class GyroscopeValue:

    x: float

    y: float

    z: float


class GyroscopeSensor:

    def read(self) -> Sample[GyroscopeValue]:
        ...
```

统一单位：

```text
degree / second
```

---

# 14. 温度接口

```python
class TemperatureSensor:

    def read(self) -> Sample[float]:
        ...
```

单位：

```text
°C
```

---

# 15. SpeedProvider

设计：

```python
class SpeedProvider:

    def read(self) -> Sample[float]:
        ...
```

单位：

```text
km/h
```

当前：

```text
MockSpeedProvider
```

后期：

```text
GPS
编码器
OBD
CAN
```

---

# 16. 模拟数据架构

不要给每个传感器随便生成随机数。

增加统一模块：

```text
ScenarioEngine
```

管理：

```text
车辆速度
前方距离
车辆角速度
车道偏移
车内温度
传感器状态
```

结构：

```text
ScenarioEngine
      │
      ├── MockDistanceSensor
      ├── MockGyroscopeSensor
      ├── MockTemperatureSensor
      ├── MockSpeedProvider
      └── SyntheticCamera
```

这样不同数据之间具有逻辑关系。

---

# 17. 模拟场景

## normal_drive

```text
speed = 30 km/h
distance = 50 m
yaw_rate = 2°/s
temperature = 25°C
lane_offset = 0
```

---

## obstacle_approach

```text
50m
40m
30m
20m
15m
10m
5m
```

---

## lane_departure

```text
offset_ratio:

0.0
0.1
0.2
0.3
0.4
0.5
```

---

## sharp_turn

```text
yaw_rate:

5
10
20
35
45
55 °/s
```

---

## high_temperature

```text
25
28
30
32
34
36
38°C
```

---

## sensor_failure

模拟：

```text
distance timeout

camera unavailable

gyro error
```

---

# 18. 等比放缩

如果桌面实验使用超声波：

实际检测范围可能只有：

```text
0.1 m ~ 1 m
```

为了模拟真实道路，可定义：

```text
distance_scale = 100
```

例如：

```text
0.1m -> 10m

0.25m -> 25m

0.5m -> 50m
```

代码：

```python
mapped_distance = raw_distance * distance_scale
```

注意：

```text
raw_distance
```

必须保留。

---

# 19. 摄像头的具体用途

摄像头只做：

```text
前向道路画面

车道线识别

车道偏离判断
```

第一版不建议做：

```text
YOLO
行人识别
车辆识别
红绿灯识别
疲劳驾驶
车牌识别
```

否则项目会偏向计算机视觉，而不是物联网与边缘计算。

---

# 20. 摄像头三种输入方式

## SyntheticCamera

程序生成简单道路：

```text
      /        \
     /          \
    /            \
```

主要用于单元测试。

---

## VideoCamera

读取本地：

```text
road.mp4
```

代码：

```python
cap = cv2.VideoCapture("road.mp4")
```

这是开发阶段最推荐的模式。

---

## RealCamera

USB Camera：

```python
cap = cv2.VideoCapture(0)
```

CSI Camera：

```text
Picamera2
```

最终统一转换为 OpenCV Frame。

---

# 21. 车道检测流程

```text
Camera Frame
     ↓
Resize
     ↓
ROI
     ↓
Gray
     ↓
Gaussian Blur
     ↓
Canny
     ↓
HoughLinesP
     ↓
Filter Lines
     ↓
Left Lane / Right Lane
     ↓
Lane Center
     ↓
Offset
     ↓
Warning
```

---

# 22. ROI

假设：

```text
640 x 480
```

只分析：

```text
y = 240 ~ 480
```

或者使用梯形 ROI。

目的：

```text
减少天空
车辆
建筑
无关区域
```

提高识别稳定性。

---

# 23. Canny

例如：

```python
edges = cv2.Canny(
    gray,
    50,
    150
)
```

---

# 24. HoughLinesP

使用：

```python
lines = cv2.HoughLinesP(...)
```

得到线段后按：

```text
位置
斜率
长度
```

分成：

```text
Left Lines

Right Lines
```

---

# 25. 计算车道中心

例如：

```text
left_x = 180

right_x = 460
```

则：

```text
lane_center = 320
```

640 像素宽画面：

```text
car_center = 320
```

所以：

```text
offset = 0
```

---

# 26. 归一化偏移

建议：

```text
offset_ratio =
(car_center - lane_center)
/
(lane_width / 2)
```

例如：

```text
abs(offset_ratio) > 0.35
```

持续：

```text
1000 ms
```

触发：

```text
LANE_DEPARTURE
```

---

# 27. LaneResult

```python
@dataclass
class LaneResult:

    valid: bool

    offset_ratio: float

    direction: str

    confidence: float
```

例如：

```json
{
  "valid": true,
  "offset_ratio": 0.42,
  "direction": "right",
  "confidence": 0.86
}
```

---

# 28. 无法识别时的处理

如果：

```text
车道线没有检测到
```

应输出：

```text
valid = false
```

不能输出：

```text
lane = normal
```

因为：

```text
没有检测出来
```

不代表：

```text
车辆没有偏离
```

---

# 29. 摄像头边缘计算

摄像头可能采集：

```text
10 FPS
```

但平台无需接收全部图像。

推荐：

```text
本地处理：
5~10 FPS

网页预览：
1~2 FPS
```

流程：

```text
Camera
 ↓
OpenCV
 ↓
本地 Lane Detection
 ↓
只上传 lane_offset 等结果
```

每 500ms：

```text
额外压缩一张 JPEG
```

用于网页预览。

这就是项目中非常典型的边缘计算设计。

---

# 30. 前方障碍物算法

基础规则：

```text
distance > 15m

NORMAL


8m < distance <= 15m

WARNING


distance <= 8m

DANGER
```

---

# 31. 时距指标

为了体现多传感器融合，可以使用：

```text
distance + speed
```

计算：

```text
headway =
distance / (speed / 3.6)
```

例如：

```text
distance = 10m

speed = 36km/h
```

则：

```text
speed = 10m/s

headway = 1s
```

示例：

```text
headway >= 1.5s
NORMAL

0.8s <= headway < 1.5s
WARNING

headway < 0.8s
DANGER
```

注意：

这是简化的时距指标。

不是严格 TTC。

---

# 32. 急转弯规则

```text
abs(yaw_rate) > 30°/s

AND

speed > 20 km/h

AND

持续 > 300ms
```

则：

```text
SHARP_TURN_WARNING
```

---

# 33. 温度规则

```text
temperature >= 35°C

持续 5 秒
```

触发。

解除：

```text
temperature <= 33°C

持续 5 秒
```

---

# 34. Rule Engine

所有规则统一采用状态机：

```text
NORMAL
  ↓
PENDING
  ↓
WARNING
  ↓
RECOVERING
  ↓
NORMAL
```

避免：

```text
35.0
34.9
35.1
34.8
```

导致不停触发 / 恢复。

---

# 35. Event 事件

只在：

```text
状态变化
```

时生成事件。

例如：

```text
NORMAL -> WARNING

ALARM_STARTED
```

```text
WARNING -> DANGER

ALARM_LEVEL_CHANGED
```

```text
WARNING -> NORMAL

ALARM_RECOVERED
```

---

# 36. 执行器

建议至少准备：

```text
蜂鸣器

LED
```

执行器不算传感器。

作用：

```text
自动本地预警

平台向下控制
```

---

# 37. 告警输出策略

例如：

```text
NORMAL
绿色 LED

WARNING
黄色 LED
蜂鸣器间歇响

DANGER
红色 LED
蜂鸣器快速响
```

---

# 38. 输出优先级

自动告警应高于人工测试。

例如：

```text
DANGER = 100

WARNING = 80

buzzer.test = 20
```

这样网页点击测试不会覆盖真正的危险告警。

---

# 39. MQTT Topic

```text
car/{device_id}/telemetry

car/{device_id}/events

car/{device_id}/status

car/{device_id}/commands

car/{device_id}/command-acks
```

例如：

```text
car/car-001/telemetry
```

---

# 40. Telemetry 示例

```json
{
  "schema_version": 1,

  "device_id": "car-001",

  "timestamp": "2026-09-29T10:00:00Z",

  "seq": 1234,

  "speed": {
    "value": 32.0,
    "unit": "km/h",
    "source": "mock"
  },

  "distance": {
    "raw": 0.18,
    "mapped": 18.0,
    "unit": "m",
    "source": "mock",
    "status": "ok"
  },

  "temperature": {
    "value": 27.4,
    "unit": "C",
    "source": "mock",
    "status": "ok"
  },

  "gyro": {
    "x": 0.8,
    "y": 1.1,
    "z": 18.2,
    "unit": "deg/s",
    "source": "mock",
    "status": "ok"
  },

  "lane": {
    "valid": true,
    "offset_ratio": 0.12,
    "direction": "center"
  },

  "risk": {
    "level": "normal"
  }
}
```

---

# 41. QoS

推荐：

| 数据 | QoS |
|---|---:|
| Telemetry | 0 |
| Status | 1 |
| Event | 1 |
| Command | 1 |
| Command Ack | 1 |

---

# 42. 下行 Command

```json
{
  "schema_version": 1,

  "command_id": "cmd-001",

  "device_id": "car-001",

  "type": "buzzer.test",

  "issued_at": "2026-09-29T10:00:00Z",

  "expires_at": "2026-09-29T10:00:05Z",

  "params": {
    "duration_ms": 1000
  }
}
```

---

# 43. Command 类型

```text
config.update

buzzer.test

indicator.test

camera.snapshot

scenario.start

scenario.stop
```

---

# 44. Command Ack

成功：

```json
{
  "command_id": "cmd-001",

  "device_id": "car-001",

  "status": "success",

  "executed_at": "2026-09-29T10:00:01Z",

  "message": "buzzer test completed"
}
```

失败：

```json
{
  "command_id": "cmd-001",

  "status": "failed",

  "message": "unsupported command"
}
```

---

# 45. 命令去重

MQTT QoS 1 可能重复投递。

因此：

```text
command_id
```

必须唯一。

树莓派缓存已执行命令。

重复收到：

```text
cmd-001
```

不能再次执行。

直接返回之前结果即可。

---

# 46. 命令有效期

动作命令必须有：

```text
expires_at
```

例如设备离线后重新上线，收到旧命令：

```text
buzzer.test
```

如果已经过期：

```text
拒绝执行
```

避免旧命令重新触发执行器。

---

# 47. 配置

示例：

```yaml
rules:

  obstacle:

    warning_distance_m: 15

    danger_distance_m: 8


  temperature:

    trigger_c: 35

    recover_c: 33


  sharp_turn:

    yaw_threshold_deg_s: 30

    min_speed_kmh: 20


  lane_departure:

    offset_threshold: 0.35

    duration_ms: 1000
```

---

# 48. 配置版本

平台保存：

```text
desired_config_version
```

树莓派上报：

```text
reported_config_version
```

例如：

```text
desired = 10

reported = 9
```

网页显示：

```text
配置同步中
```

只有：

```text
desired == reported
```

才显示：

```text
配置已生效
```

---

# 49. 边缘端目录结构

```text
edge/
│
├── main.py
│
├── config.yaml
│
├── requirements.txt
│
├── drivers/
│   ├── base.py
│   ├── distance/
│   │   ├── mock.py
│   │   └── hardware.py
│   ├── camera/
│   │   ├── synthetic.py
│   │   ├── video.py
│   │   ├── usb.py
│   │   └── picamera.py
│   ├── gyro/
│   │   ├── mock.py
│   │   └── hardware.py
│   └── temperature/
│       ├── mock.py
│       └── hardware.py
│
├── simulation/
│   ├── engine.py
│   └── scenarios.py
│
├── processing/
│   ├── distance.py
│   ├── lane.py
│   ├── gyro.py
│   └── temperature.py
│
├── rules/
│   ├── engine.py
│   ├── obstacle.py
│   ├── lane_departure.py
│   ├── sharp_turn.py
│   └── temperature.py
│
├── actuators/
│   ├── buzzer.py
│   ├── indicator.py
│   └── manager.py
│
├── commands/
│   ├── handler.py
│   └── deduplicator.py
│
├── transport/
│   ├── mqtt.py
│   └── image_upload.py
│
├── storage/
│   ├── sqlite.py
│   └── event_store.py
│
└── models/
    ├── sample.py
    ├── telemetry.py
    ├── event.py
    └── command.py
```

---

# 50. 边缘端并发模型

不要：

```text
读超声波
 ↓
读温度
 ↓
读陀螺仪
 ↓
读摄像头
 ↓
上传
 ↓
再次读取
```

推荐：

```text
Distance Worker ─┐

Gyro Worker ─────┤

Temp Worker ─────┤

Camera Worker ───┤

                 ↓

            Latest State

                 ↓

            Rule Engine

                 ↓

        Event / Actuator

                 ↓

         MQTT Publish Queue
```

---

# 51. 初始采样频率

| 数据 | 推荐初始频率 |
|---|---:|
| 超声波 | 10 Hz |
| 陀螺仪 | 50 Hz |
| 温度 | 1 Hz |
| 摄像头采集 | 10 FPS |
| 图像分析 | 5~10 FPS |
| Rule Engine | 10 Hz |
| Telemetry 上报 | 2 Hz |
| Web 图片预览 | 1~2 FPS |

实际硬件接入后根据性能调整。

---

# 52. 断网设计

MQTT Broker 不可用时：

```text
Sensor
继续运行

OpenCV
继续运行

Rule Engine
继续运行

Buzzer
继续运行
```

不能因为平台断网导致本地预警失效。

---

# 53. Edge SQLite

树莓派使用独立 SQLite：

```text
edge.db
```

保存：

```text
未上传 Event

设备配置

Command 去重记录
```

---

# 54. 断网补传

Event：

```text
event_id

uploaded
```

断网时：

```text
uploaded = false
```

恢复网络：

```text
重新发送
```

平台确认：

```text
uploaded = true
```

---

# 55. Go 后端目录

```text
server/
│
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   ├── handler/
│   │   ├── device.go
│   │   ├── telemetry.go
│   │   ├── event.go
│   │   ├── command.go
│   │   └── websocket.go
│   │
│   ├── service/
│   │   ├── device.go
│   │   ├── telemetry.go
│   │   ├── event.go
│   │   ├── command.go
│   │   └── config.go
│   │
│   ├── repo/
│   │   ├── device.go
│   │   ├── telemetry.go
│   │   ├── event.go
│   │   └── command.go
│   │
│   ├── mqtt/
│   │   ├── client.go
│   │   ├── subscriber.go
│   │   └── publisher.go
│   │
│   └── model/
│
├── migrations/
│
└── config.yaml
```

---

# 56. 平台数据库

第一版：

```text
SQLite
```

表：

```text
devices

telemetry

events

commands

device_configs

snapshots
```

---

# 57. devices

字段：

```text
id

device_id

name

online

last_seen_at

config_version

created_at
```

---

# 58. telemetry

字段：

```text
id

device_id

speed

distance

temperature

gyro_x

gyro_y

gyro_z

lane_offset

risk_level

timestamp
```

不建议把：

```text
50Hz
```

陀螺仪数据全部长期保存。

可以：

```text
边缘 50Hz

平台 1Hz 落一次历史快照
```

---

# 59. events

字段：

```text
id

event_id

device_id

type

level

payload

started_at

ended_at

snapshot_path
```

---

# 60. commands

字段：

```text
id

command_id

device_id

type

params

status

issued_at

expires_at

ack_at

error_message
```

状态：

```text
pending

sent

success

failed

timeout
```

---

# 61. Go API

```text
GET /api/devices

GET /api/devices/:id/state

GET /api/devices/:id/telemetry

GET /api/devices/:id/events

GET /api/devices/:id/config

PUT /api/devices/:id/config

POST /api/devices/:id/commands

GET /api/commands/:command_id

POST /api/devices/:id/frames

GET /api/devices/:id/frame

GET /api/ws
```

---

# 62. WebSocket

实时数据：

```text
Raspberry Pi
     ↓ MQTT

Go Backend
     ↓ WebSocket

Vue
```

浏览器不需要直接连接 MQTT Broker。

---

# 63. 前端页面

建议四页。

---

## 63.1 实时监控

显示：

```text
设备在线状态

车速

障碍物距离

温度

Yaw Rate

车道偏移

综合风险等级

传感器状态

摄像头预览
```

---

## 63.2 告警记录

显示：

```text
发生时间

告警类型

告警等级

触发时数据

恢复时间

截图
```

---

## 63.3 设备控制

提供：

```text
修改阈值

蜂鸣器测试

LED 测试

摄像头拍照

查看 Command 状态

查看 Config Version
```

---

## 63.4 模拟场景

按钮：

```text
正常驾驶

接近障碍

车道偏离

急转弯

车内高温

传感器故障

停止场景
```

---

# 64. 模拟场景下行

例如用户点击：

```text
车道偏离
```

流程：

```text
Vue

POST /commands

Go

MQTT:
scenario.start

Raspberry Pi

ScenarioEngine

切换到 lane_departure
```

所以：

```text
模拟器本身也受平台向下控制
```

非常适合作为课程演示。

---

# 65. Camera Snapshot

网页：

```text
拍摄当前画面
```

后端发送：

```text
camera.snapshot
```

树莓派：

```text
读取最新 Frame
↓
保存 JPEG
↓
HTTP 上传
```

Go：

```text
返回图片 ID / URL
```

网页显示：

```text
拍照成功
```

也是一个很直观的下行控制功能。

---

# 66. 推荐 config.yaml

```yaml
device:

  id: car-001


mqtt:

  host: 192.168.1.100

  port: 1883

  telemetry_qos: 0

  event_qos: 1

  command_qos: 1


sensors:

  distance:

    driver: mock

    sample_hz: 10


  camera:

    driver: video

    path: ./assets/road.mp4

    width: 640

    height: 480


  gyro:

    driver: mock

    sample_hz: 50


  temperature:

    driver: mock

    sample_hz: 1


speed:

  driver: scenario


mapping:

  distance_scale: 100


rules:

  obstacle:

    warning_distance_m: 15

    danger_distance_m: 8


  lane_departure:

    offset_threshold: 0.35

    duration_ms: 1000


  sharp_turn:

    yaw_threshold_deg_s: 30

    min_speed_kmh: 20

    duration_ms: 300


  temperature:

    trigger_c: 35

    recover_c: 33

    duration_ms: 5000


transport:

  telemetry_hz: 2

  preview_fps: 2


storage:

  sqlite_path: ./data/edge.db
```

---

# 67. Hardware Driver 切换

开发阶段：

```yaml
distance:
  driver: mock

gyro:
  driver: mock

temperature:
  driver: mock

camera:
  driver: video
```

后续：

```yaml
distance:
  driver: hc_sr04

gyro:
  driver: mpu6050

temperature:
  driver: dht22

camera:
  driver: picamera
```

以上型号只作为示例。

真正采购后根据实际型号编写对应 Driver。

---

# 68. 系统上行数据流

```text
Sensor
 ↓
Driver
 ↓
Sample
 ↓
Processing
 ↓
Latest State
 ↓
Rule Engine
 ↓
Telemetry / Event
 ↓
MQTT
 ↓
Mosquitto
 ↓
Go
 ↓
SQLite
 ↓
WebSocket
 ↓
Vue
```

---

# 69. 系统下行数据流

```text
Vue
 ↓ HTTP
Go
 ↓
生成 command_id
 ↓
SQLite 保存 Command
 ↓
MQTT
 ↓
Raspberry Pi
 ↓
Command Handler
 ↓
Actuator / Config / Scenario
 ↓
Command Ack
 ↓
MQTT
 ↓
Go
 ↓
更新 Command
 ↓
WebSocket
 ↓
Vue
```

---

# 70. 第一阶段开发内容

先实现接口：

```text
Sample

DistanceSensor

CameraSource

GyroscopeSensor

TemperatureSensor

SpeedProvider
```

全部使用 Mock。

---

# 71. 第二阶段

实现：

```text
ScenarioEngine

normal_drive

obstacle_approach

lane_departure

sharp_turn

high_temperature

sensor_failure
```

---

# 72. 第三阶段

实现：

```text
ObstacleRule

SharpTurnRule

TemperatureRule

LaneDepartureRule
```

摄像头暂时可以用模拟 LaneResult。

---

# 73. 第四阶段

完成：

```text
Mosquitto

Python MQTT Client

Go MQTT Client
```

跑通：

```text
Raspberry Pi -> MQTT -> Go
```

---

# 74. 第五阶段

Vue 页面先只实现：

```text
实时仪表盘
```

验证：

```text
树莓派模拟数据

↓

网页实时更新
```

---

# 75. 第六阶段

实现下行：

```text
scenario.start

buzzer.test

config.update
```

必须做：

```text
Command Ack
```

---

# 76. 第七阶段

加入真实摄像头处理链路。

先使用：

```text
road.mp4
```

实现：

```text
OpenCV Lane Detection
```

然后再切真实 Camera。

---

# 77. 第八阶段

增加：

```text
Telemetry 历史

Event 历史

Command 历史

图片
```

---

# 78. 第九阶段

真实传感器采购后逐个替换：

```text
Mock Driver

↓

Hardware Driver
```

不改：

```text
Rule Engine

MQTT

Go

Vue
```

---

# 79. 最小可行版本 MVP

如果课程周期较紧，最低建议完成：

```text
四种 Sensor Interface

四种 Mock Sensor

ScenarioEngine

OpenCV Lane Detection

Obstacle Warning

Sharp Turn Warning

Temperature Warning

Lane Departure Warning

MQTT

Go Backend

SQLite

Vue Dashboard

Command 下行

Command Ack

buzzer.test

scenario.start

config.update
```

这已经可以形成一个完整课程设计。

---

# 80. 最终演示流程

## 演示一：正常行驶

页面：

```text
Speed: 30 km/h

Distance: 50 m

Temperature: 25°C

Yaw Rate: 2°/s

Lane: Normal

Risk: Normal
```

---

## 演示二：接近障碍物

网页点击：

```text
接近障碍物
```

平台下发：

```text
scenario.start
```

树莓派：

```text
50m
↓
30m
↓
15m
↓
8m
↓
5m
```

观察：

```text
网页距离变化

风险等级变化

LED 变化

蜂鸣器响

Event 被记录
```

---

## 演示三：车道偏离

播放道路视频或合成道路。

随着：

```text
lane_offset
```

增加：

```text
0.1
0.2
0.3
0.4
0.5
```

OpenCV 检测结果发生变化。

超过阈值后：

```text
Lane Departure Warning
```

---

## 演示四：急转弯

设置：

```text
speed = 30km/h
```

然后：

```text
yaw_rate
10
20
30
40
50°/s
```

观察急转弯预警。

---

## 演示五：车内高温

温度：

```text
25
30
33
35
36
38°C
```

超过阈值持续 5 秒后产生告警。

---

## 演示六：平台向下控制

网页点击：

```text
蜂鸣器测试
```

流程：

```text
Vue
↓
Go
↓
MQTT
↓
Raspberry Pi
↓
蜂鸣器响
↓
Ack
↓
网页显示执行成功
```

---

## 演示七：修改阈值

网页：

```text
障碍物 WARNING 阈值

15m -> 20m
```

平台：

```text
config.update
```

树莓派应用成功：

```text
config_version + 1
```

网页显示：

```text
配置已同步
```

---

## 演示八：断网

让树莓派：

```text
断开 MQTT
```

此时：

```text
Sensor
Rule Engine
Buzzer
```

依然运行。

产生告警后：

```text
Edge SQLite
```

缓存 Event。

恢复网络后：

```text
自动补传
```

---

# 81. 课设创新点 / 可写进报告的亮点

## 1. 统一传感器抽象

通过：

```text
Sensor Interface
```

解耦：

```text
业务逻辑
```

和：

```text
具体硬件型号
```

支持 Mock / Hardware 无缝切换。

---

## 2. 边缘计算

摄像头画面：

```text
不全部上传服务器
```

而是在树莓派：

```text
OpenCV
```

本地处理。

只上传：

```text
lane_offset

warning

低帧率 preview
```

降低：

```text
网络带宽

平台压力

预警延迟
```

---

## 3. 断网自治

平台失联时：

```text
本地驾驶预警仍然有效
```

体现真正的：

```text
Edge Computing
```

而不是简单物联网数据采集。

---

## 4. 双向控制

系统不仅：

```text
Device -> Cloud
```

同时支持：

```text
Cloud -> Device
```

包括：

```text
阈值修改

蜂鸣器

LED

拍照

模拟场景
```

完整体现下行控制。

---

## 5. 命令可靠性

设计：

```text
command_id

expires_at

ack

deduplication
```

避免重复执行与过期命令。

---

## 6. 多源数据融合

障碍物风险不是只看距离。

可以结合：

```text
distance + speed
```

急转弯：

```text
yaw_rate + speed
```

提高系统设计完整度。

---

# 82. 硬件购买后需要处理的问题

真实硬件型号确定以后，需要确认：

```text
通信接口

GPIO

I2C

SPI

UART

供电电压

逻辑电平

量程

采样频率
```

尤其注意：

树莓派 GPIO 为 3.3V 逻辑电平。

如果某些超声波传感器 Echo 为 5V：

```text
不能直接连接 GPIO
```

需要：

```text
分压 / 电平转换
```

---

# 83. 项目不涉及真实车辆控制

本课设仅用于：

```text
模型 / 桌面 / 教学演示
```

系统可以：

```text
告警

显示

蜂鸣

LED
```

不直接控制：

```text
真实车辆刹车

方向盘

油门
```

这样风险更低，也符合课程设计范围。

---

# 84. 最终方案总结

最终系统采用：

```text
Raspberry Pi 4B
        ↓
四类传感器
        ↓
Python Edge
        ↓
OpenCV + Rule Engine
        ↓
MQTT
        ↓
Mosquitto
        ↓
Go + Gin
        ↓
SQLite
        ↓
WebSocket
        ↓
Vue
```

四种传感器：

```text
超声波

摄像头

陀螺仪

温度
```

主要功能：

```text
前方障碍物预警

车道偏离预警

急转弯预警

车内高温预警
```

平台支持：

```text
实时监控

历史数据

告警记录

参数配置

摄像头预览

设备控制
```

下行控制：

```text
buzzer.test

indicator.test

camera.snapshot

scenario.start

scenario.stop

config.update
```

开发策略：

```text
先 Mock

↓

跑通全链路

↓

再接真实硬件
```

这样可以保证在传感器具体型号尚未确定的情况下，软件开发仍然能够立即开始，并且后续硬件确定后只需要增加 Driver，不需要重构整套系统。

