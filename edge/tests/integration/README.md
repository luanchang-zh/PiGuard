# 软件环境联调

`test_live_stack.py` 连接本机已启动的 Mosquitto 和独立 Go 服务，覆盖上线、遥测、事件入库、命令 Ack、命令去重、Broker 重启和正常停止。默认的 `python -m pytest` 不收集这个目录。同一 `car-001` 不要同时运行 Go 模拟器。

报告见 [真实 Broker 联调](../../../docs/verification/integration/p5-live-stack.md)。
