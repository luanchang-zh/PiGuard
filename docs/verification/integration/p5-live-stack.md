# 真实 Broker 联调报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/integration/test_live_stack.py`。默认的 `python -m pytest` 不收集这个文件。Go 模拟器没有启动。边缘端没有实现 HTTP 入库确认客户端；入库以平台的 HTTP 查询为准。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows，win32 |
| Python | 3.13.12 |
| pytest | 9.1.1 |
| OpenCV | 4.11.0 |
| websockets | 15.0.1 |
| Docker Engine | 29.7.2 |
| 后端 | 独立工作副本 `E:\StudyDir\PiGuard-go`，分支 `go后端开发`，提交 `7b39493177fca1586279707a357b6f906694053e` |
| 服务 | Compose 项目 `piguard-edge-it`：Mosquitto 1883、Go 后端 8080。设备 `car-001` 使用 MQTT 和 HTTP Basic 认证 |
| 时钟 | 边缘端使用系统时钟。命令有效期由后端按真实时间签发 |

Docker Desktop 把宿主机上的单个文件挂进容器后变成了空目录，所以 Broker 配置是先写入命名卷再挂载的。构建后端镜像时 `proxy.golang.org` 返回 unexpected EOF，构建使用了 `GOPROXY=https://goproxy.cn,direct`。这两处修改没有提交，测试结束后工作副本已还原。重跑时需要重新处理这两处。

## 命令与结果

在 `edge/` 下，环境变量指向已启动的 Compose 栈之后执行：

```text
python -m pytest tests/integration/test_live_stack.py -q --tb=short
```

1 项通过，0 项失败，耗时 15.87 秒。

## 观察到的行为

| 门槛 | 结果 |
| --- | --- |
| 设备上线，车速 30 km/h | 通过。`GET /devices/car-001/state` 的 `online` 为 true |
| 合成道路经过检测后进入平台状态 | 通过。车道 `valid=true`，`offset_ratio` 约为 0，方向 center，置信度 1 |
| `buzzer.test` 400 ms 成功，两个 WebSocket 都收到 telemetry、event、command_update | 通过。命令 `cmd-VNJARDBEUQXP2RL5LLSXRF3GSM`，结果为 `buzzer test completed`。两条 `/api/v1/ws` 各保持约 14.81 秒后正常结束 |
| 同一条已完成命令再次发布，不重做，`executed_at` 不变 | 通过。平台记录 `忽略命令回执 reason=duplicate`，`executed_at=2026-10-12T04:48:13.615Z` |
| `scenario.start obstacle_approach` 后出现障碍物事件，风险变为 danger | 通过。事件只有两条：`obstacle_warning` / `started` / warning，以及 `obstacle_warning` / `level_changed` / danger。结束后状态里距离为 5 m，车速 30 km/h |
| Broker 重启后边缘端重新连接，平台不新增事件 | 通过。后端先记录 MQTT EOF 和一次连接拒绝，随后重新订阅。重启后的 event_id 集合与重启前相同，设备重新在线 |
| 正常停止后设备离线 | 通过。停止后 `GET /devices/car-001` 的 `online` 为 false。停止前平台保存的风险仍是 danger |

## 本报告未覆盖

没有把 Go 后端进程单独重启，也没有在事件已经发出后把边缘进程崩溃再拉起。边缘端仍不会把 HTTP 查询结果写回 `platform_confirmed`。磁盘写满没有在这次实网里注入。没有运行 Go 模拟器，没有使用树莓派、GPIO、蜂鸣器、LED 或 OV5647。测试结束后 Compose 项目已用 `docker compose -p piguard-edge-it down -v` 停掉。
