# 开发电脑上的边缘软件实施计划

状态：P1 与 P2 的软件路径已完成。P2 见 [上行实施计划](p2-uplink.md)。真实 Broker 联调未做。

本文件是可执行的实施计划。型号、电气约束、MQTT 契约、频率、规则参数和阶段门槛以 [总规范](../PLAN.md) 为准。进度和下一步只写在根目录 `HANDOFF.md`。后续阶段开始时，在本目录另写 `pN-短名.md`，不把实施步骤改写进总规范。

## 1. 当前范围

在开发电脑上完成 P1 到 P5。默认 `mode=mock`，不导入 gpiozero、smbus2、Picamera2。树莓派、真实接线和 systemd 留到 P6、P7。

| 阶段 | 本阶段产出 | 前置 | 完成检查 |
| --- | --- | --- | --- |
| P1 本地数据链 | 内部模型、五类接口、六个场景、配置加载、LatestState | P0 | 无硬件、无 Broker。六个场景可重复；故障值保持空；停止场景后恢复 `normal_drive` |
| P2 上行 | 遥测/状态编解码、MQTT、进程入口 | P1，以及可到达的 Broker 和 Go 服务 | 2Hz 遥测、上线/遗嘱、故障 null、重连。规则未完成时 `risk=unknown` |
| P3 本地预警 | 距离与角速度处理、四条规则、模拟执行器、事件缓存 | P1。与平台对账需要 P2 | 四种危险及恢复可复现；断网时本地预警继续；重连后同一 `event_id` 只入库一条 |
| P4 下行命令 | 四种已支持动作、命令去重、Ack | P2、P3 的模拟执行器 | 重复、过期、非法、重启和执行中崩溃按总规范第 5 节通过 |
| P5 车道算法 | 合成道路、本地录像、OpenCV 处理 | P1 的 `LaneResult`。规则持续时间在 P3 已有 | 画面实际经过 ROI、灰度、模糊、Canny、Hough。左右线缺失为 invalid |

P2 到 P5 的契约细节沿用总规范第 5、6 节。每一阶段开工时再写自己的实施文件；本文件把 P1 写到可以动手的粒度。

## 2. 当前不做

- P6、P7：等树莓派在手，并先完成总规范第 1.1 节的电平转换和 I2C 上拉检查。
- 蜂鸣器与 LED 的引脚和驱动：型号未定。P3、P4 只用模拟执行器。
- 远程配置和图片上传：Go 后端这两项仍返回 501。`remote_config` 与 `frame_upload` 保持关闭。
- 在 `edge` 分支修改 Go。配置同步、图片接口回到 `go后端开发` 分支。
- 同一 `car-001` 上同时运行 Go 模拟器与 Python 边缘端。

## 3. P1 步骤

按这个顺序做。前一步的检查通过后再做下一步。

### 3.1 内部模型

新增 `edge/src/piguard_edge/models/sample.py` 与 `models/frame.py`。

`Sample[T]` 保留可空的 `value`、UTC `sample_at`、`monotonic_ns`、`source`、`status`。`status` 只有 `ok`、`timeout`、`unavailable`、`error`。单位固定为 m、km/h、°C、deg/s。读取失败时 `value` 为空，测试断言它不是 0。

`LaneResult` 保留 `valid`、`offset_ratio`、`direction`、`confidence`、`sample_at`。无车道时 `valid=false`、`direction=unknown`，偏移保持空。`Frame` 保留采集时间和图像槽；P1 允许 `image` 为空。合成像素留到 P5。

检查：构造故障样本和无效车道，序列化到测试夹具后数值仍为空。

### 3.2 五类接口与 mock

新增 `drivers/base.py`，定义 `DistanceSensor`、`CameraSource`、`GyroscopeSensor`、`TemperatureSensor`、`SpeedProvider`。mock 放在对应驱动目录。硬件库只允许未来的 HC-SR04、OV5647、MPU6050、AHT20 驱动导入。

mock 距离返回场景换算后的 `raw_value`。课设第 17 节给出的是道路距离：实现时先除以 `distance_scale` 得到 raw，处理层再乘一次得到 mapped。`distance_scale=100` 时，道路 50 m 对应 raw 0.5 m、mapped 50 m。静止速度保持 0，映射函数不对它做除法。

陀螺仪 mock 输出三轴角速度。P1 按配置的 `yaw_axis` 和 `yaw_sign` 取出 `yaw_rate`；默认 `z` 与 `1`。加速度不进入车速。

检查：mock 模块的导入图里不出现 gpiozero、smbus2、Picamera2。缩放测试只看到一次乘法。

### 3.3 六个场景

新增 `simulation/scenarios.py` 与 `simulation/engine.py`。场景名只有：

`normal_drive`、`obstacle_approach`、`lane_departure`、`sharp_turn`、`high_temperature`、`sensor_failure`。

关键帧顺序使用课设第 17 节。某个场景没有写出的量，保持 `normal_drive` 的值：速度 30 km/h，道路距离 50 m，角速度 2 deg/s，温度 25 °C，车道偏移 0。`lane_departure` 使用课设给出的正偏移，方向与符号一致；负向由测试夹具覆盖。

每个关键帧保持 1 秒场景时间。`playback_speed` 只接受 0.5、1.0、2.0，并只缩放这个间隔。距离、车速、陀螺仪、温度、车道读取同一个场景时钟，不允许各自累计时间。

`sensor_failure` 按课设分别给出距离 `timeout`、摄像头 `unavailable`、陀螺仪 `error`，对应值为空。温度和车速保持有效。摄像头不可用时车道为 invalid。

`scenario.stop` 回到 `normal_drive`，采样继续。P1 由测试直接调用这个切换；MQTT 命令留到 P4。

检查：同一 fake time 上五类读数来自同一关键帧。把时钟重新拨到同一时刻，读数相同。停止后下一拍已经是 `normal_drive`。

### 3.4 配置加载

新增 `config/loader.py`。本阶段加入 YAML 加载依赖，版本在实现时按该库的当前文档锁定，并写进 `pyproject.toml`。

加载 `edge/config/default.example.yaml`。相对路径按配置文件所在目录解析。同目录的 `local.yaml` 可以覆盖；该文件不进 Git。环境变量覆盖留到 P2 出现凭据时再做。

加载器接受并保存 `hardware` 段。未知顶层键拒绝启动。mock 驱动的构造参数里不接收 BCM、I2C 地址和 CSI 分辨率。

检查：示例配置加载成功。删掉 `hardware` 或写成未知顶层键时失败。mock 驱动对象上看不到引脚和地址。

### 3.5 LatestState 与采集步进

新增 `runtime/state.py` 与 `runtime/workers.py`。每类源只更新自己的最新样本。规则、MQTT 和图像编码不在本阶段启动。

采样周期使用总规范第 4 节：距离和车速 10 Hz，陀螺仪 50 Hz，温度 1 Hz，图像采集 10 FPS。单元测试推进 fake clock，调用 worker 使用的同一步进函数，不使用墙钟睡眠。线程循环只作为这个步进函数的外壳，放到本阶段最后；测试不依赖它的调度精度。

检查：把 fake clock 推进 1 秒后，距离约 10 个样本、陀螺仪约 50 个、温度 1 个，且时间戳来自 fake clock。某个源返回故障时，其他源继续更新。

### 3.6 P1 验收

在 `edge/tests/unit/` 覆盖：故障不变成 0、六个场景的共享时钟、播放倍率、停止后恢复、距离只缩放一次、加载器接受 `hardware` 且 mock 不读取它。

通过命令：

```sh
cd edge
python -m pytest tests/unit
PYTHONPATH=src python -c "import piguard_edge"
```

本次运行记录见 [P1 单元测试报告](../../verification/unit/p1-local-data-chain.md)。

本阶段不连接 Broker，不打开摄像头，不访问 GPIO。

## 4. P1 之后

P1 的检查通过后，按第 1 节的表进入 P2。P2 的实施文件再写 Topic、遗嘱、seq 和与 Go 的对照样例。P3 使用本节场景产生的样本做规则计时，车道结果仍由场景提供。P5 才把 `Frame.image` 填成实际画面，并进入同一条车道处理函数。
