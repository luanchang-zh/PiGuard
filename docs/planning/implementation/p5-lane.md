# P5 车道检测实施计划

状态：合成道路和本地录像已完成。OV5647 实拍未做。

契约以 [总规范](../PLAN.md) 第 6 节的 P5 门槛为准。联调结果见 [真实 Broker 报告](../../verification/integration/p5-live-stack.md)。

## 1. 本阶段产出

- `processing/lane.py`：对一张 BGR 图做 ROI、灰度、模糊、Canny、Hough，并算出偏移。
- `drivers/camera/paint.py`：按给定偏移画两条白线。检测只能从像素恢复，不能读回这个参数。
- `drivers/camera/synthetic.py`：场景决定画在哪里；检测失败时车道无效。
- `drivers/camera/video.py`：读取本地视频，结束时释放采集。
- `drivers/factory.py`：`synthetic` 与 `video`。空的 `video_path` 不能启动录像驱动。

## 2. 行为

- 画面先变成 640×480。ROI 是 y=240 到 480。灰度后用 5×5 高斯模糊，Canny 阈值 50/150，HoughLinesP 的 rho=1、theta=π/180、threshold=40、minLineLength=40、maxLineGap=30。
- 斜率绝对值小于 0.3 的线段和水平线段不用。竖线按它在画面中心的左右归边。线段延伸到 ROI 内 y=220 处取 x。
- 左右线都存在且车道宽度不小于 40 像素才有效。`offset_ratio = (320 - lane_center) / (lane_width / 2)`。绝对值小于 0.01 记为 0，避免居中道路因噪声变成左偏。正值向右，负值向左，0 为 center。缺一边或空白画面时 `valid=false`，偏移和置信度为 null，方向为 unknown。
- 置信度是左右线段数之和除以 4，最大为 1。偏移和置信度写成 Python float，避免报文里出现 numpy 数值。
- 合成摄像头在传感器故障时返回空图像和无效车道，不调用检测。录像读到结尾后状态为 unavailable，再次 release 不报错。
- 配置里的 `process_fps` 仍是 5，`capture_fps` 仍是 10。当前采样器按采集频率读摄像头，并且每次读取都做检测。OV5647 以后走同一条 `inspect_lane`。默认配置继续使用 synthetic。

## 3. 验证

单元测试用画出来的像素检查符号、处理步骤、缺线和录像释放，并用替换检测函数证明合成摄像头不抄场景里的偏移数字。默认 `python -m pytest tests/unit tests/contract` 不连接 Broker。真实 Broker 上的居中车道见联调报告。

## 4. 本阶段不做

Picamera2、CSI 实拍、梯形掩膜、`camera.snapshot`、远程配置和 GPIO。
